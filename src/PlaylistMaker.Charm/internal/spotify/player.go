package spotify

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"playlistmaker/charm/internal/tracking"
)

type ActiveState struct {
	SessionID      string `json:"sessionId"`
	HelperPID      int    `json:"helperPid"`
	DeviceID       string `json:"deviceId"`
	OriginalVolume int    `json:"originalVolume"`
}

type Player struct {
	Client         *Client
	StatePath      string
	SessionID      string
	HelperPID      int
	deviceID       string
	originalVolume int
	prepared       bool
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
		return fmt.Errorf("Spotify device name %q matched %d devices", deviceName, len(matches))
	}
	device := matches[0]
	if device.ID == "" || device.Restricted || !device.SupportsVolume || device.VolumePercent == nil {
		return fmt.Errorf("Spotify device %q does not support volume control", deviceName)
	}
	p.deviceID, p.originalVolume, p.prepared = device.ID, *device.VolumePercent, true
	if err := p.writeState(ActiveState{SessionID: p.SessionID, HelperPID: p.HelperPID, DeviceID: p.deviceID, OriginalVolume: p.originalVolume}); err != nil {
		return err
	}
	if err := p.Client.SetVolume(ctx, p.deviceID, 0); err != nil {
		_ = os.Remove(p.StatePath)
		return err
	}
	return nil
}

func (p *Player) Start(ctx context.Context, track tracking.Track) error {
	if !p.prepared {
		return fmt.Errorf("Spotify player was not preflighted")
	}
	if err := p.Client.Pause(ctx, p.deviceID); err != nil {
		return err
	}
	return p.Client.Play(ctx, p.deviceID, track.SpotifyURI)
}

func (p *Player) Stop(ctx context.Context) error {
	if !p.prepared {
		return nil
	}
	return p.Client.Pause(ctx, p.deviceID)
}

func (p *Player) Close(ctx context.Context) error {
	if !p.prepared {
		return nil
	}
	first := p.Client.Pause(ctx, p.deviceID)
	if err := p.Client.SetVolume(ctx, p.deviceID, p.originalVolume); first == nil {
		first = err
	}
	if first == nil {
		_ = os.Remove(p.StatePath)
		p.prepared = false
	}
	return first
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
	if err := client.Pause(ctx, state.DeviceID); err != nil {
		return err
	}
	if err := client.SetVolume(ctx, state.DeviceID, state.OriginalVolume); err != nil {
		return err
	}
	return os.Remove(statePath)
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
