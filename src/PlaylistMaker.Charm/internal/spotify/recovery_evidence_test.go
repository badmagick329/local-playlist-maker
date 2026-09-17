package spotify

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"playlistmaker/charm/internal/tracking"
)

func requestedPausedStart(t *testing.T) *controlFixture {
	t.Helper()
	f := newControlFixture(t)
	p := f.player
	p.prepared = true
	p.SessionID = "early-pause"
	p.StatePath = filepath.Join(t.TempDir(), "owner.json")
	p.requestedTrack = *f.state.Item
	if err := p.Start(context.Background(), tracking.Track{SpotifyURI: f.state.Item.URI}); err != nil {
		t.Fatal(err)
	}
	f.commands = nil
	f.state.IsPlaying = false
	f.state.ProgressMS = 500
	f.now = f.now.Add(time.Second)
	f.state.Timestamp = f.now.UnixMilli()
	return f
}

func TestNewOccurrencePausedBeforeConfirmationRecoversInPlace(t *testing.T) {
	for _, retry := range []bool{false, true} {
		t.Run(map[bool]string{false: "automatic", true: "explicit-retry"}[retry], func(t *testing.T) {
			f := requestedPausedStart(t)
			if retry {
				f.now = f.now.Add(30 * time.Second)
				if _, err := f.player.Finished(context.Background()); err == nil {
					t.Fatal("initial confirmation did not time out")
				}
				f.player.Retry()
			}
			if f.check(t, 0) {
				t.Fatal("paused start completed")
			}
			if len(f.commands) != 1 || f.commands[0] != "/me/player/play:" {
				t.Fatalf("expected in-place resume only: %v", f.commands)
			}
			if f.player.observed || f.player.control.phase != "recovering" {
				t.Fatal("accepted resume released unconfirmed start")
			}
			f.state.IsPlaying = true
			f.check(t, time.Second)
			if f.player.observed || f.player.control.phase != "recovering" {
				t.Fatal("stationary playing response released hold")
			}
			f.state.ProgressMS = 1500
			f.state.Timestamp = f.now.Add(time.Second).UnixMilli()
			f.check(t, time.Second)
			if !f.player.observed || f.player.control.phase != "playing" {
				t.Fatal("advancing occurrence was not confirmed")
			}
			if len(f.commands) != 1 {
				t.Fatalf("restarted or redundantly resumed occurrence: %v", f.commands)
			}
		})
	}
}

func TestEarlyPauseStillHonoursLongUserHoldAndRetry(t *testing.T) {
	f := requestedPausedStart(t)
	ceiling := 500
	f.player.Intent(true, &ceiling)
	f.check(t, 0)
	if len(f.commands) != 0 || f.player.control.phase != "paused" {
		t.Fatal("deliberate early pause was resumed")
	}
	f.check(t, 8*time.Hour)
	f.player.Intent(false, nil)
	f.check(t, 0)
	if len(f.commands) != 1 {
		t.Fatal("long pause prevented in-place resume")
	}
	f.now = f.now.Add(recoveryLimit)
	if _, err := f.player.Finished(context.Background()); err == nil {
		t.Fatal("unchanged early pause recovered without progress")
	}
	f.player.Retry()
	f.check(t, 0)
	if len(f.commands) != 2 || f.player.observed {
		t.Fatal("explicit retry restarted or confirmed stationary playback")
	}
	f.state.IsPlaying = true
	f.state.ProgressMS = 1500
	f.state.Timestamp = f.now.Add(time.Second).UnixMilli()
	f.check(t, time.Second)
	if !f.player.observed || f.player.control.phase != "playing" {
		t.Fatal("retry did not verify advancing playback")
	}
}

func TestUnconfirmedPauseRejectsPreviousOccurrenceAndUncertainOwnership(t *testing.T) {
	for _, scenario := range []string{"previous-timestamp", "previous-position", "missing-timestamp", "unaccepted-start", "different-owner"} {
		t.Run(scenario, func(t *testing.T) {
			f := requestedPausedStart(t)
			switch scenario {
			case "previous-timestamp":
				f.state.Timestamp = f.player.startedAt.Add(-time.Second).UnixMilli()
			case "previous-position":
				f.state.ProgressMS = 175000
			case "missing-timestamp":
				f.state.Timestamp = 0
			case "unaccepted-start":
				f.player.startAccepted = false
			case "different-owner":
				if err := f.player.writeState(ActiveState{SessionID: "other", DeviceID: "device", TrackURI: f.state.Item.URI}); err != nil {
					t.Fatal(err)
				}
			}
			f.now = f.now.Add(4 * time.Minute)
			f.player.block("confirmation timed out")
			f.player.Retry()
			done, _ := f.player.Finished(context.Background())
			if done || f.player.observed || len(f.commands) != 0 {
				t.Fatalf("ambiguous occurrence authorised playback: done=%v observed=%v commands=%v", done, f.player.observed, f.commands)
			}
		})
	}
}

func resumedShortTail(t *testing.T) *controlFixture {
	t.Helper()
	f := newControlFixture(t)
	target := 179500
	f.state.ProgressMS = target
	f.state.Timestamp = f.now.UnixMilli()
	f.player.Intent(true, &target)
	f.check(t, 0)
	f.state.IsPlaying = false
	f.state.Timestamp = f.now.Add(time.Second).UnixMilli()
	f.check(t, time.Second)
	if !f.player.control.deliberateStop {
		t.Fatal("ceiling pause not retained")
	}
	f.player.Intent(false, nil)
	f.check(t, 0)
	if len(f.commands) != 2 || f.commands[1] != "/me/player/play:" {
		t.Fatalf("resume commands: %v", f.commands)
	}
	return f
}

func TestResumedShortTailCompletesBetweenObservations(t *testing.T) {
	f := resumedShortTail(t)
	f.state.ProgressMS = 180000
	f.state.Timestamp = f.now.Add(time.Second).UnixMilli()
	if !f.check(t, time.Second) {
		t.Fatal("fresh stopped endpoint rejected")
	}
	if len(f.commands) != 2 {
		t.Fatalf("completed song resumed again: %v", f.commands)
	}
}

func TestShortTailRequiresFreshProgressAfterResume(t *testing.T) {
	for _, scenario := range []string{"stationary", "stale", "same-timestamp", "missing-timestamp", "too-soon"} {
		t.Run(scenario, func(t *testing.T) {
			f := resumedShortTail(t)
			elapsed := time.Second
			f.state.ProgressMS = 180000
			switch scenario {
			case "stationary":
				f.state.ProgressMS = 179500
				f.state.Timestamp = f.now.Add(time.Second).UnixMilli()
			case "stale":
				f.state.Timestamp = f.now.Add(-time.Second).UnixMilli()
			case "missing-timestamp":
				f.state.Timestamp = 0
			case "too-soon":
				elapsed = 100 * time.Millisecond
				f.state.Timestamp = f.now.Add(elapsed).UnixMilli()
			}
			if f.check(t, elapsed) {
				t.Fatal("stationary/stale/impossible endpoint accepted")
			}
			if !f.player.control.deliberateStop || f.player.control.phase != "recovering" {
				t.Fatal("unverified response released hold")
			}
			if len(f.commands) != 2 {
				t.Fatalf("unproven endpoint issued additional resume: %v", f.commands)
			}
			f.state.ProgressMS = 180000
			f.state.Timestamp = f.now.Add(time.Second).UnixMilli()
			if !f.check(t, time.Second) {
				t.Fatal("subsequent fresh completion rejected")
			}
			if len(f.commands) != 2 {
				t.Fatal("completion required another resume")
			}
		})
	}
}

func TestNearEndDeviceTakeoverRetainsEvidenceButBlocksControl(t *testing.T) {
	f := newControlFixture(t)
	f.state.ProgressMS = 175000
	f.check(t, 0)
	f.state.Device.ID = "other-device"
	f.state.Item.URI = "spotify:track:other"
	f.now = f.now.Add(5 * time.Second)
	done, err := f.player.Finished(context.Background())
	if done || err == nil || f.player.control.phase != "blocked" {
		t.Fatalf("takeover authorised completion: %v %v", done, err)
	}
	if f.player.maxProgress != 175000 {
		t.Fatal("takeover discarded near-end evidence")
	}
	f.player.Retry()
	f.now = f.now.Add(time.Second)
	if done, err = f.player.Finished(context.Background()); done || err == nil {
		t.Fatal("retry bypassed takeover")
	}
	if len(f.commands) != 0 {
		t.Fatalf("takeover was overwritten: %v", f.commands)
	}
}
