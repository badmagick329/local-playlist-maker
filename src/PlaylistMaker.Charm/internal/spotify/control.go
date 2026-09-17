package spotify

import (
	"context"
	"playlistmaker/charm/internal/tracking"
	"time"
)

// Five-second normal polls tolerate two unchanged readings. Recovery has an
// independent wall-clock budget, including failed requests and rate limiting.
const stallLimit = 12 * time.Second
const recoveryLimit = 30 * time.Second

type playbackControl struct {
	retry                                       bool
	paused                                      bool
	deliberateStop                              bool
	target                                      *int
	phase, message                              string
	updated, lastAdvance, recoveryAt, commandAt time.Time
	attempts                                    int
	baseline                                    int
	resume                                      *resumeEvidence
}

// A stopped endpoint can be the only observation of a resumed short tail.
// Keep the pre-command position and timestamp so an old pause cannot prove it.
type resumeEvidence struct {
	requestedAt time.Time
	progress    int
	timestamp   int64
}

func (p *Player) completedResume(state PlaybackState) bool {
	r := p.control.resume
	return p.control.deliberateStop && r != nil && !state.IsPlaying &&
		p.playDurationMS > 0 && state.Item.DurationMS == p.playDurationMS &&
		state.ProgressMS >= p.playDurationMS && state.ProgressMS > r.progress &&
		state.Timestamp > r.timestamp && state.Timestamp >= r.requestedAt.UnixMilli() &&
		p.now().Sub(r.requestedAt) >= time.Duration(p.playDurationMS-r.progress)*time.Millisecond
}

func (p *Player) SetProtection(protect func() error) { p.protect = protect }

func (p *Player) Intent(paused bool, target *int) {
	p.accountHold(p.now())
	if p.control.paused && !paused && p.control.phase != "blocked" {
		p.recovering(p.now())
	}
	if p.control.paused != paused || !sameTarget(p.control.target, target) {
		p.nextCheck = time.Time{}
	}
	p.control.paused, p.control.target = paused, target
}

// A safely identified early pause may be intentionally held before the first
// advancing sample. Neither confirmation nor completion spends that held time.
func (p *Player) accountHold(now time.Time) {
	if p.control.phase == "paused" && !p.control.updated.IsZero() {
		held := now.Sub(p.control.updated)
		p.confirmationHeld += held
		if !p.completionDeadline.IsZero() {
			p.completionDeadline = p.completionDeadline.Add(held)
		}
		p.control.updated = now
	}
}

func sameTarget(a, b *int) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }

func (p *Player) TrackingStatus() (string, string) { return p.control.phase, p.control.message }

func (p *Player) Retry() {
	if p.control.phase != "blocked" {
		return
	}
	p.control.phase, p.control.message = "recovering", "Verifying Spotify playback"
	p.control.recoveryAt, p.control.commandAt, p.nextCheck = p.now(), time.Time{}, time.Time{}
	p.control.attempts = 0
	p.control.retry = true
	// Retain progress/completion evidence, but grant a new bounded recovery attempt.
	p.completionDeadline = p.now().Add(time.Duration(max(0, p.playDurationMS-p.maxProgress))*time.Millisecond + time.Minute)
}

func (p *Player) block(message string) error {
	p.control.phase, p.control.message = "blocked", message
	p.report(message)
	return &tracking.PlaybackFailure{Message: message}
}

func (p *Player) recovering(now time.Time) {
	p.control.phase, p.control.message = "recovering", "Recovering Spotify; video held"
	p.control.recoveryAt, p.control.commandAt = now, time.Time{}
	p.control.attempts, p.control.baseline = 0, p.lastProgress
	p.control.resume = nil
	p.report(p.control.message + "; " + p.lastPlayback)
}

// Commands are only issued against a freshly observed owned song and device.
// Deliberate holds do not spend completion time, but catch-up still must advance.
func (p *Player) controlPlayback(ctx context.Context, state PlaybackState, previous int) error {
	now := p.now()
	c := &p.control
	if state.ProgressMS > previous && state.IsPlaying {
		c.lastAdvance = now
	}
	hold := c.paused && (c.target == nil || state.ProgressMS >= *c.target)
	if hold {
		if state.IsPlaying {
			if c.phase != "pausing" {
				p.recovering(now)
				c.phase = "pausing"
			}
			if c.commandAt.IsZero() || now.Sub(c.commandAt) >= 5*time.Second {
				c.commandAt = now
				c.deliberateStop = true
				c.resume = nil
				if p.protect != nil {
					if err := p.protect(); err != nil {
						return p.block("Cannot publish tracking hold: " + err.Error())
					}
				}
				if err := p.Client.Pause(ctx, p.deviceID); err != nil {
					return err
				}
			}
			return nil
		}
		c.phase, c.message = "paused", "Paused at video catch-up ceiling"
		c.lastAdvance = now
		return nil
	}
	if c.phase == "paused" || c.phase == "pausing" {
		p.recovering(now)
	}
	if !state.IsPlaying && c.phase != "recovering" {
		p.recovering(now)
	}
	if c.phase == "recovering" {
		if state.IsPlaying && state.ProgressMS > c.baseline && state.ProgressMS > previous {
			c.deliberateStop = false
			c.phase, c.message = "playing", "Spotify playback verified"
			c.lastAdvance = now
			p.report(c.message)
		} else if c.attempts < 3 && (c.commandAt.IsZero() || now.Sub(c.commandAt) >= 8*time.Second) {
			c.commandAt = now
			c.attempts++
			c.baseline = state.ProgressMS
			p.report("requesting in-place Spotify resume")
			if p.protect != nil {
				if err := p.protect(); err != nil {
					return p.block("Cannot publish tracking hold: " + err.Error())
				}
			}
			if err := p.Client.Resume(ctx, p.deviceID); err != nil {
				return err
			}
			c.resume = &resumeEvidence{requestedAt: now, progress: state.ProgressMS, timestamp: state.Timestamp}
		}
	} else {
		c.phase, c.message = "playing", "Tracking playback"
	}
	if c.paused && c.phase == "playing" {
		c.phase, c.message = "catch-up", "Spotify catching up to paused video"
	}
	return nil
}
