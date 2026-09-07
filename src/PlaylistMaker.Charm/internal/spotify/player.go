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
	requestedTrack     Track
	startedAt          time.Time
	nextCheck          time.Time
	observed           bool
	completionDeadline time.Time
	lastPlayback       string
}

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
	if err := p.writeState(ActiveState{SessionID: p.SessionID, HelperPID: p.HelperPID, DeviceID: p.deviceID}); err != nil {
		return err
	}
	p.attempted = true
	// The session queue owns repetitions; inherited Spotify repeat would prevent
	// a play from finishing and hold every later video in the tracking queue.
	if err := p.Client.DisableRepeat(ctx, p.deviceID); err != nil {
		return err
	}
	if err := p.Client.Play(ctx, p.deviceID, track.SpotifyURI); err != nil {
		return err
	}
	p.trackURI, p.startedAt, p.nextCheck, p.observed = track.SpotifyURI, time.Now(), time.Time{}, false
	p.completionDeadline, p.lastPlayback = time.Time{}, "no playback response"
	return nil
}

// Finished uses Spotify's actual state so a short video cannot truncate its
// tracking song. The first matching playback must be observed before accepting
// an idle response, since Connect can briefly return the previous play's state.
func (p *Player) Finished(ctx context.Context) (bool, error) {
	now := time.Now()
	// Check deadlines before polling backoff: a rate limit or offline device must
	// not keep a detached session alive forever.
	if !p.startedAt.IsZero() && ((!p.observed && now.Sub(p.startedAt) >= 30*time.Second) ||
		(p.observed && !p.completionDeadline.IsZero() && now.After(p.completionDeadline))) {
		return false, &tracking.PlaybackFailure{Message: fmt.Sprintf("Spotify tracking timed out: requested %q (%s); last response: %s", p.requestedTrack.Name, p.trackURI, p.lastPlayback)}
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
	matching := state.Device.ID == p.deviceID && state.Item != nil && p.matchesTrack(*state.Item)
	p.lastPlayback = fmt.Sprintf("playing=%t, device=%s, position=%dms", state.IsPlaying, state.Device.ID, state.ProgressMS)
	if state.Item != nil {
		p.lastPlayback += fmt.Sprintf(", track=%q (%s)", state.Item.Name, state.Item.URI)
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
		p.completionDeadline = now.Add(time.Duration(max(0, state.Item.DurationMS-state.ProgressMS))*time.Millisecond + time.Minute)
	}
	if !matching || !state.IsPlaying {
		return true, nil
	}
	remaining := time.Duration(state.Item.DurationMS-state.ProgressMS) * time.Millisecond
	if remaining > time.Second {
		p.nextCheck = now.Add(min(remaining+250*time.Millisecond, 5*time.Second))
	}
	return false, nil
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
	if !playback.IsPlaying || playback.Device.ID == "" || playback.Device.ID != state.DeviceID {
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
