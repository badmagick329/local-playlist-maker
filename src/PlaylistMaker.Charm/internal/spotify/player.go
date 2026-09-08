package spotify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"playlistmaker/charm/internal/tracking"
)

type ActiveState struct {
	SessionID string `json:"sessionId"`
	HelperPID int    `json:"helperPid"`
	DeviceID  string `json:"deviceId"`
	TrackURI  string `json:"trackUri"`
}

type Player struct {
	Client             *Client
	StatePath          string
	SessionID          string
	HelperPID          int
	deviceID           string
	prepared           bool
	attempted          bool
	trackURI           string
	stateTrackURI      string
	requestedTrack     Track
	startedAt          time.Time
	nextCheck          time.Time
	observed           bool
	completionDeadline time.Time
	lastPlayback       string
	repeatConfirmed    bool
	repeatRetryAt      time.Time
	repeatAttempts     int
	lastRepeatState    string
	lastStateTimestamp int64
	lastProgress       int
	maxProgress        int
	lastProgressLogged int
	playDurationMS     int
	staleLogged        bool
	candidateLogged    bool
	diagnostic         func(string)
	Now                func() time.Time
}

func (p *Player) SetDiagnostic(write func(string)) { p.diagnostic = write }

func (p *Player) Preflight(ctx context.Context, deviceName string) error {
	if p.Client == nil || strings.TrimSpace(deviceName) == "" {
		return fmt.Errorf("Spotify client ID and device name are not configured")
	}
	devices, err := p.Client.Devices(ctx)
	if err != nil {
		return err
	}
	matches := []Device{}
	for _, device := range devices {
		if strings.EqualFold(device.Name, deviceName) {
			matches = append(matches, device)
		}
	}
	if len(matches) != 1 {
		return fmt.Errorf("Spotify device name %q matched %d devices; available devices: %s", deviceName, len(matches), deviceSummary(devices))
	}
	device := matches[0]
	if device.ID == "" {
		return fmt.Errorf("Spotify device %q has no device ID", deviceName)
	}
	if device.Restricted {
		return fmt.Errorf("Spotify device %q is restricted", deviceName)
	}
	p.deviceID, p.prepared = device.ID, true
	return nil
}

func (p *Player) Start(ctx context.Context, track tracking.Track) error {
	if !p.prepared {
		return fmt.Errorf("Spotify player was not preflighted")
	}
	if p.requestedTrack.URI != track.SpotifyURI {
		requested, err := p.Client.Track(ctx, track.SpotifyURI)
		if err != nil {
			return err
		}
		p.requestedTrack = requested
	}
	// The session queue owns repetitions; inherited Spotify repeat would prevent
	// a play from finishing and hold every later video in the tracking queue.
	if err := p.Client.DisableRepeat(ctx, p.deviceID); err != nil {
		return err
	}
	p.trackURI, p.stateTrackURI = track.SpotifyURI, track.SpotifyURI
	if err := p.writeState(ActiveState{SessionID: p.SessionID, HelperPID: p.HelperPID, DeviceID: p.deviceID, TrackURI: p.stateTrackURI}); err != nil {
		return err
	}
	p.attempted = true
	if err := p.Client.Play(ctx, p.deviceID, track.SpotifyURI); err != nil {
		return err
	}
	p.startedAt, p.nextCheck, p.observed = p.now(), time.Time{}, false
	p.completionDeadline, p.lastPlayback = time.Time{}, "no playback response"
	p.repeatConfirmed, p.repeatRetryAt, p.repeatAttempts = false, time.Time{}, 1
	p.lastRepeatState, p.lastStateTimestamp = "", 0
	p.lastProgress, p.maxProgress, p.lastProgressLogged, p.playDurationMS = 0, 0, 0, 0
	p.staleLogged, p.candidateLogged = false, false
	p.report("repeat disable requested before playback; awaiting observed repeat state")
	return nil
}

// Finished uses Spotify's actual state so a short video cannot truncate its
// tracking song. The first matching playback must be observed before accepting
// an idle response, since Connect can briefly return the previous play's state.
func (p *Player) Finished(ctx context.Context) (bool, error) {
	now := p.now()
	// Check deadlines before polling backoff: a rate limit or offline device must
	// not keep a detached session alive forever.
	if !p.startedAt.IsZero() && (((!p.observed || !p.repeatConfirmed) && now.Sub(p.startedAt) >= 30*time.Second) ||
		(p.observed && !p.completionDeadline.IsZero() && now.After(p.completionDeadline))) {
		message := fmt.Sprintf("Spotify tracking timed out: requested %q (%s); observed=%t, repeatConfirmed=%t, repeatDisableAttempts=%d, maximumProgress=%dms; last response: %s", p.requestedTrack.Name, p.trackURI, p.observed, p.repeatConfirmed, p.repeatAttempts, p.maxProgress, p.lastPlayback)
		p.report("completion decision: tracking failed at bounded deadline; " + message)
		return false, &tracking.PlaybackFailure{Message: message}
	}
	if now.Before(p.nextCheck) {
		return false, nil
	}
	p.nextCheck = now.Add(time.Second)
	state, err := p.Client.CurrentPlayback(ctx)
	if err != nil {
		p.nextCheck = now.Add(5 * time.Second)
		var limited *RateLimitError
		if errors.As(err, &limited) && limited.Valid {
			p.nextCheck = now.Add(limited.RetryAfter)
			return false, nil
		}
		return false, err
	}
	if state.Timestamp > 0 && (state.Timestamp < p.startedAt.Add(-2*time.Second).UnixMilli() || p.lastStateTimestamp > 0 && state.Timestamp < p.lastStateTimestamp) {
		if !p.staleLogged {
			p.report(fmt.Sprintf("ignored stale playback response: timestamp=%d, startedAt=%d", state.Timestamp, p.startedAt.UnixMilli()))
			p.staleLogged = true
		}
		return false, nil
	}
	if state.Timestamp > p.lastStateTimestamp {
		p.lastStateTimestamp = state.Timestamp
	}
	matching := state.Device.ID == p.deviceID && state.Item != nil && p.matchesTrack(*state.Item)
	p.lastPlayback = fmt.Sprintf("playing=%t, device=%s, position=%dms, repeat=%q, timestamp=%d", state.IsPlaying, state.Device.ID, state.ProgressMS, state.RepeatState, state.Timestamp)
	if state.Item != nil {
		p.lastPlayback += fmt.Sprintf(", track=%q (%s)", state.Item.Name, state.Item.URI)
	}
	if state.Device.ID == p.deviceID {
		if state.RepeatState == "off" {
			if !p.repeatConfirmed {
				p.report("observed repeat state off")
			}
			p.repeatConfirmed = true
		} else if state.RepeatState != p.lastRepeatState {
			if state.RepeatState == "" {
				p.report("repeat state was absent from playback response; awaiting confirmation")
			} else {
				p.report(fmt.Sprintf("observed repeat state %q; requesting off again", state.RepeatState))
			}
		}
		p.lastRepeatState = state.RepeatState
		if state.RepeatState != "off" && !now.Before(p.repeatRetryAt) {
			p.repeatAttempts++
			p.repeatRetryAt = now.Add(5 * time.Second)
			if err := p.Client.DisableRepeat(ctx, p.deviceID); err != nil {
				return false, fmt.Errorf("verify Spotify repeat is off: %w", err)
			}
		}
	}
	if !p.observed {
		if !matching || !state.IsPlaying || time.Duration(state.ProgressMS)*time.Millisecond > now.Sub(p.startedAt)+2*time.Second {
			if now.Sub(p.startedAt) > 15*time.Second {
				p.nextCheck = now.Add(5 * time.Second)
				return false, fmt.Errorf("waiting for Spotify to confirm the requested tracking song; keep Spotify open")
			}
			return false, nil
		}
		p.observed = true
		p.trackURI = state.Item.URI
		if p.attempted && p.stateTrackURI != p.trackURI {
			if err := p.writeState(ActiveState{SessionID: p.SessionID, HelperPID: p.HelperPID, DeviceID: p.deviceID, TrackURI: p.trackURI}); err != nil {
				return false, fmt.Errorf("record observed Spotify release: %w", err)
			}
			p.stateTrackURI = p.trackURI
		}
		p.playDurationMS = state.Item.DurationMS
		p.lastProgress, p.maxProgress = state.ProgressMS, state.ProgressMS
		p.completionDeadline = now.Add(time.Duration(max(0, state.Item.DurationMS-state.ProgressMS))*time.Millisecond + time.Minute)
		p.report(fmt.Sprintf("playback observed: position=%dms, duration=%dms, completionDeadline=%s", state.ProgressMS, state.Item.DurationMS, p.completionDeadline.UTC().Format(time.RFC3339)))
	} else if matching && (state.IsPlaying || state.ProgressMS >= p.lastProgress) {
		// A stopped response can reset position to zero. Only a playing
		// backward jump invalidates the prior play's near-end evidence.
		p.recordProgress(state.ProgressMS)
	}
	if !matching || !state.IsPlaying {
		if p.repeatConfirmed && p.nearEnd() {
			p.report(fmt.Sprintf("completion decision: accepted observed transition after reaching %dms of %dms", p.maxProgress, p.playDurationMS))
			return true, nil
		}
		if !p.candidateLogged {
			p.report(fmt.Sprintf("completion decision: ignored transition without near-end evidence; maximumProgress=%dms, duration=%dms", p.maxProgress, p.playDurationMS))
			p.candidateLogged = true
		}
		return false, nil
	}
	remaining := time.Duration(state.Item.DurationMS-state.ProgressMS) * time.Millisecond
	if remaining > time.Second {
		p.nextCheck = now.Add(min(remaining+250*time.Millisecond, 5*time.Second))
	}
	return false, nil
}

func (p *Player) recordProgress(progress int) {
	if progress+2_000 < p.lastProgress {
		p.report(fmt.Sprintf("significant progress change: backward from %dms to %dms; not treated as completion", p.lastProgress, progress))
		p.maxProgress = progress
		p.lastProgressLogged = progress
		p.candidateLogged = false
	} else {
		p.maxProgress = max(p.maxProgress, progress)
		if progress-p.lastProgressLogged >= 30_000 || p.nearEnd() && p.lastProgressLogged+2_000 < progress {
			p.report(fmt.Sprintf("significant progress change: position=%dms, duration=%dms", progress, p.playDurationMS))
			p.lastProgressLogged = progress
		}
	}
	p.lastProgress = progress
}

func (p *Player) nearEnd() bool {
	return p.playDurationMS > 0 && p.maxProgress >= max(0, p.playDurationMS-7_000)
}

func (p *Player) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

func (p *Player) report(message string) {
	if p.diagnostic != nil {
		p.diagnostic(message)
	}
}

// Connect can play another release of the requested song without linked_from
// metadata. Match Spotify's canonical title and full artist credits in that case;
// use the playing release's duration, never the requested release's duration.
func (p *Player) matchesTrack(actual Track) bool {
	if actual.URI == p.trackURI {
		return true
	}
	if p.observed {
		return false
	}
	expected := p.requestedTrack
	if expected.Name == "" || len(expected.Artists) == 0 || !strings.EqualFold(normalizeReleaseTitle(expected.Name), normalizeReleaseTitle(actual.Name)) || len(expected.Artists) != len(actual.Artists) {
		return false
	}
	for i, artist := range expected.Artists {
		if !strings.EqualFold(artist.Name, actual.Artists[i].Name) {
			return false
		}
	}
	return true
}

// Releases can use typographic apostrophes (including a prime) for the same
// title. Preserve words and version labels so different mixes remain distinct.
func normalizeReleaseTitle(title string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '‘', '’', '′', 'ʼ', '＇':
			return '\''
		}
		return r
	}, title)
}

func (p *Player) Stop(ctx context.Context) error {
	if !p.prepared || !p.attempted {
		return nil
	}
	state, err := p.Client.CurrentPlayback(ctx)
	if err != nil {
		return err
	}
	if !state.IsPlaying || state.Device.ID != p.deviceID || state.Item == nil || !p.matchesTrack(*state.Item) {
		return nil
	}
	return p.Client.Pause(ctx, p.deviceID)
}

func (p *Player) Close(ctx context.Context) error {
	if !p.prepared || !p.attempted {
		return nil
	}
	stopErr := p.Stop(ctx)
	if err := os.Remove(p.StatePath); err == nil || os.IsNotExist(err) {
		p.prepared = false
		p.attempted = false
		return stopErr
	} else {
		return fmt.Errorf("remove Spotify active state: %w", err)
	}
}

func Recover(ctx context.Context, client *Client, statePath string, alive ...func(int) bool) error {
	contents, err := os.ReadFile(statePath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var state ActiveState
	if err := json.Unmarshal(contents, &state); err != nil {
		return err
	}
	isAlive := processAlive
	if len(alive) > 0 && alive[0] != nil {
		isAlive = alive[0]
	}
	if state.HelperPID > 0 && isAlive(state.HelperPID) {
		return nil
	}
	if client == nil {
		return fmt.Errorf("Spotify client is unavailable")
	}
	playback, err := client.CurrentPlayback(ctx)
	if err != nil {
		var responseErr *ResponseError
		if errors.As(err, &responseErr) && responseErr.StatusCode == http.StatusNotFound {
			return os.Remove(statePath)
		}
		return err
	}
	if !playback.IsPlaying || playback.Device.ID == "" || playback.Device.ID != state.DeviceID || playback.Item == nil || playback.Item.URI != state.TrackURI {
		return os.Remove(statePath)
	}
	if err := client.Pause(ctx, state.DeviceID); err != nil {
		return err
	}
	return os.Remove(statePath)
}

func deviceSummary(devices []Device) string {
	if len(devices) == 0 {
		return "none"
	}
	values := make([]string, 0, len(devices))
	for _, device := range devices {
		name := strings.TrimSpace(device.Name)
		if name == "" {
			name = "(unnamed)"
		}
		deviceType := strings.TrimSpace(device.Type)
		if deviceType == "" {
			values = append(values, name)
			continue
		}
		values = append(values, fmt.Sprintf("%s (%s)", name, deviceType))
	}
	return strings.Join(values, ", ")
}

func (p *Player) writeState(state ActiveState) error {
	contents, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p.StatePath), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(p.StatePath), ".spotify-state-*.tmp")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err = temporary.Write(append(contents, '\n')); err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := secureFile(name); err != nil {
		return err
	}
	return os.Rename(name, p.StatePath)
}
