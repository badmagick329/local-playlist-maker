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

	"playlistmaker/charm/internal/tracking"
)

type ActiveState struct {
	SessionID string `json:"sessionId"`
	HelperPID int    `json:"helperPid"`
	DeviceID  string `json:"deviceId"`
}

type Player struct {
	Client    *Client
	StatePath string
	SessionID string
	HelperPID int
	deviceID  string
	prepared  bool
	attempted bool
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
	if err := p.writeState(ActiveState{SessionID: p.SessionID, HelperPID: p.HelperPID, DeviceID: p.deviceID}); err != nil {
		return err
	}
	p.attempted = true
	return p.Client.Play(ctx, p.deviceID, track.SpotifyURI)
}

func (p *Player) Stop(ctx context.Context) error {
	if !p.prepared || !p.attempted {
		return nil
	}
	return p.Client.Pause(ctx, p.deviceID)
}

func (p *Player) Close(ctx context.Context) error {
	if !p.prepared || !p.attempted {
		return nil
	}
	if err := p.Client.Pause(ctx, p.deviceID); err != nil {
		return err
	}
	if err := os.Remove(p.StatePath); err == nil || os.IsNotExist(err) {
		p.prepared = false
		p.attempted = false
		return nil
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
