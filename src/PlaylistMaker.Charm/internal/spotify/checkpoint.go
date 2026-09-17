package spotify

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"playlistmaker/charm/internal/tracking"
	"time"
)

type evidence struct {
	Device, URI                 string
	Requested                   Track
	Observed, Repeat            bool
	Progress, Maximum, Duration int
	Started                     time.Time
}

func (p *Player) Checkpoint() json.RawMessage {
	data, _ := json.Marshal(evidence{p.deviceID, p.trackURI, p.requestedTrack, p.observed, p.repeatConfirmed, p.lastProgress, p.maxProgress, p.playDurationMS, p.startedAt})
	return data
}

func (p *Player) Reacquire() error {
	data, err := os.ReadFile(p.StatePath)
	if err != nil {
		return p.block("Spotify ownership evidence is unavailable; automatic resume is unsafe")
	}
	var owner ActiveState
	if json.Unmarshal(data, &owner) != nil || owner.SessionID != p.SessionID || owner.TrackURI != p.trackURI || owner.DeviceID != p.deviceID {
		return p.block("Another session replaced this Spotify occurrence; automatic resume is unsafe")
	}
	return nil
}

// A failed suspension may not have reached Spotify. A later session must not
// overwrite that possibly-live occurrence just because its helper released a lock.
func (p *Player) checkPreviousOwner(ctx context.Context) error {
	data, err := os.ReadFile(p.StatePath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var owner ActiveState
	if err = json.Unmarshal(data, &owner); err != nil {
		return err
	}
	if owner.SessionID == p.SessionID {
		return nil
	}
	state, err := p.Client.CurrentPlayback(ctx)
	if err != nil {
		return &tracking.StartBlocked{Message: "Cannot verify that the previous tracking occurrence stopped: " + err.Error()}
	}
	if state.Item == nil || state.Device.ID == "" {
		return &tracking.StartBlocked{Message: "Previous Spotify ownership is ambiguous; open Spotify before retrying"}
	}
	if state.IsPlaying && state.Device.ID == owner.DeviceID && state.Item.URI == owner.TrackURI {
		return &tracking.StartBlocked{Message: "A suspended session's Spotify occurrence is still playing; pause it before retrying"}
	}
	return nil
}

// A restart can adopt a confirmed, still-paused occurrence only while its durable
// ownership record and position agree. Missing/end/reset states remain ambiguous.
func (p *Player) Restore(ctx context.Context, data json.RawMessage) error {
	var e evidence
	if err := json.Unmarshal(data, &e); err != nil {
		return err
	}
	contents, err := os.ReadFile(p.StatePath)
	if err != nil {
		return err
	}
	var owner ActiveState
	if err = json.Unmarshal(contents, &owner); err != nil {
		return err
	}
	if !e.Observed || !e.Repeat || owner.SessionID != p.SessionID || owner.DeviceID != e.Device || owner.TrackURI != e.URI {
		return fmt.Errorf("saved Spotify occurrence cannot be identified safely")
	}
	state, err := p.Client.CurrentPlayback(ctx)
	if err != nil {
		return err
	}
	if state.Item == nil || state.Item.URI != e.URI || state.Device.ID != e.Device || state.IsPlaying || state.ProgressMS < e.Progress || state.ProgressMS >= e.Duration-7000 {
		return fmt.Errorf("Spotify playback is ambiguous; restore the paused owned occurrence before retry")
	}
	p.deviceID, p.trackURI, p.stateTrackURI = e.Device, e.URI, e.URI
	p.requestedTrack, p.observed, p.repeatConfirmed = e.Requested, true, true
	p.prepared, p.attempted = true, true
	p.startedAt = e.Started
	p.lastProgress, p.maxProgress, p.playDurationMS = state.ProgressMS, max(e.Maximum, state.ProgressMS), e.Duration
	p.control = playbackControl{phase: "blocked"}
	p.Retry()
	return p.writeState(ActiveState{SessionID: p.SessionID, HelperPID: p.HelperPID, DeviceID: e.Device, TrackURI: e.URI})
}

func (p *Player) ownsState() (bool, error) {
	data, err := os.ReadFile(p.StatePath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var owner ActiveState
	if err = json.Unmarshal(data, &owner); err != nil {
		return false, err
	}
	return owner.SessionID == p.SessionID && owner.DeviceID == p.deviceID && owner.TrackURI == p.stateTrackURI, nil
}
