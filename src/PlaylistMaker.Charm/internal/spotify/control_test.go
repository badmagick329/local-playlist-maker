package spotify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type controlFixture struct {
	player   *Player
	now      time.Time
	state    PlaybackState
	commands []string
	limited  bool
}

func newControlFixture(t *testing.T) *controlFixture {
	t.Helper()
	f := &controlFixture{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	f.state = PlaybackState{IsPlaying: true, ProgressMS: 45000, RepeatState: "off", Item: &Track{URI: "spotify:track:fixture", DurationMS: 180000}}
	f.state.Device.ID = "device"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if f.limited {
			w.Header().Set("Retry-After", "3600")
			w.WriteHeader(429)
			return
		}
		if r.Method == "PUT" {
			body, _ := io.ReadAll(r.Body)
			f.commands = append(f.commands, r.URL.Path+":"+string(body))
			return
		}
		json.NewEncoder(w).Encode(f.state)
	}))
	t.Cleanup(server.Close)
	f.player = &Player{Client: &Client{Auth: validAuth(t, server), HTTP: server.Client(), APIBase: server.URL}, deviceID: "device", trackURI: f.state.Item.URI, observed: true, repeatConfirmed: true, startedAt: f.now.Add(-time.Minute), completionDeadline: f.now.Add(195 * time.Second), playDurationMS: 180000, lastProgress: 45000, maxProgress: 45000, Now: func() time.Time { return f.now }, control: playbackControl{lastAdvance: f.now}}
	return f
}
func (f *controlFixture) check(t *testing.T, after time.Duration) bool {
	t.Helper()
	f.now = f.now.Add(after)
	f.player.nextCheck = time.Time{}
	done, err := f.player.Finished(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return done
}

func TestPauseCeilingAndLongResumePreservePosition(t *testing.T) {
	f := newControlFixture(t)
	ceiling := 90000
	f.player.Intent(true, &ceiling)
	f.check(t, 0)
	if len(f.commands) != 0 || f.player.control.phase != "catch-up" {
		t.Fatal("catch-up should play naturally")
	}
	if delay := f.player.nextCheck.Sub(f.now); delay > time.Second {
		t.Fatalf("ceiling poll=%s", delay)
	}
	f.state.ProgressMS = 90000
	f.check(t, 45*time.Second)
	if len(f.commands) != 1 || f.commands[0] != "/me/player/pause:" {
		t.Fatalf("commands=%v", f.commands)
	}
	f.state.IsPlaying = false
	f.check(t, time.Second)
	if f.player.control.phase != "paused" {
		t.Fatal("pause unconfirmed")
	}
	deadline := f.player.completionDeadline
	f.check(t, 8*time.Hour)
	if f.player.completionDeadline.Sub(deadline) != 8*time.Hour {
		t.Fatal("held time consumed completion budget")
	}
	f.player.Intent(false, nil)
	f.check(t, 0)
	if f.commands[len(f.commands)-1] != "/me/player/play:" {
		t.Fatalf("resume must omit URI/seek: %v", f.commands)
	}
	if f.player.control.phase != "recovering" {
		t.Fatal("accepted command released hold")
	}
	f.state.IsPlaying = true
	f.state.ProgressMS = 91000
	f.check(t, time.Second)
	if f.player.control.phase != "playing" {
		t.Fatal("advancing resume not verified")
	}
}

func TestPausedCeilingAheadUnknownAndSeeks(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(map[bool]string{false: "ahead", true: "unknown"}[unknown], func(t *testing.T) {
			f := newControlFixture(t)
			target := 30000
			var ceiling *int = &target
			if unknown {
				ceiling = nil
			}
			f.player.Intent(true, ceiling)
			f.check(t, 0)
			if len(f.commands) != 1 || !strings.Contains(f.commands[0], "pause") {
				t.Fatal("must pause immediately")
			}
			f.state.IsPlaying = false
			f.check(t, time.Second)
			target = 10000
			f.player.Intent(true, &target)
			f.check(t, time.Second)
			if len(f.commands) != 1 {
				t.Fatal("backward seek issued command")
			}
			forward := 60000
			f.player.Intent(true, &forward)
			f.check(t, time.Second)
			if f.commands[len(f.commands)-1] != "/me/player/play:" {
				t.Fatal("forward seek did not allow natural catch-up")
			}
		})
	}
}

func TestShorterSongCompletesBeforeCeiling(t *testing.T) {
	f := newControlFixture(t)
	target := 200000
	f.player.Intent(true, &target)
	f.state.ProgressMS = 179000
	f.check(t, 0)
	f.state.ProgressMS = 180000
	f.state.IsPlaying = false
	if !f.check(t, time.Second) {
		t.Fatal("normal completion not retained")
	}
	if len(f.commands) != 0 {
		t.Fatal("completed occurrence resumed")
	}
}

func TestUnexpectedPauseRecoveryAndLatestUserIntent(t *testing.T) {
	f := newControlFixture(t)
	f.state.IsPlaying = false
	f.check(t, 0)
	if f.player.control.phase != "recovering" || len(f.commands) != 1 {
		t.Fatal("pause not recovered promptly")
	}
	target := 45000
	f.player.Intent(true, &target)
	f.check(t, time.Second)
	if f.player.control.phase != "paused" {
		t.Fatal("new user pause ignored")
	}
	if len(f.commands) != 1 {
		t.Fatal("recovery resumed against latest intent")
	}
}

func TestStallAndRateLimitRecoveryRemainBounded(t *testing.T) {
	for _, limited := range []bool{false, true} {
		t.Run(map[bool]string{false: "stall", true: "rate-limit"}[limited], func(t *testing.T) {
			f := newControlFixture(t)
			f.limited = limited
			f.check(t, stallLimit)
			if f.player.control.phase != "recovering" {
				t.Fatal("stalled progress not detected")
			}
			f.now = f.now.Add(recoveryLimit)
			if _, err := f.player.Finished(context.Background()); err == nil {
				t.Fatal("backoff extended recovery forever")
			}
			if len(f.commands) > 3 {
				t.Fatal("unbounded recovery commands")
			}
		})
	}
}

func TestTakeoverBlocksWithoutOverwriting(t *testing.T) {
	for _, device := range []bool{false, true} {
		f := newControlFixture(t)
		if device {
			f.state.Device.ID = "other"
		} else {
			f.state.Item.URI = "spotify:track:unrelated"
		}
		if _, err := f.player.Finished(context.Background()); err == nil {
			t.Fatal("takeover not blocked")
		}
		if len(f.commands) != 0 {
			t.Fatalf("overwrote takeover: %v", f.commands)
		}
	}
}

func TestDeliberateNearEndPauseIsNotCompletion(t *testing.T) {
	f := newControlFixture(t)
	target := 175000
	f.player.Intent(true, &target)
	f.state.ProgressMS = target
	f.check(t, 0)
	f.state.IsPlaying = false
	if f.check(t, time.Second) {
		t.Fatal("our own pause was mistaken for natural completion")
	}
	f.now = f.now.Add(8 * time.Hour)
	f.player.Intent(false, nil)
	if f.check(t, 0) {
		t.Fatal("resumed occurrence completed without progress")
	}
	if f.player.control.phase != "recovering" {
		t.Fatal("parked hold consumed completion budget")
	}
}

func TestRestartAdoptsOnlyUnambiguousPausedOwnedOccurrence(t *testing.T) {
	for _, scenario := range []string{"paused", "regressed", "finished", "playing", "other-owner"} {
		t.Run(scenario, func(t *testing.T) {
			f := newControlFixture(t)
			f.player.StatePath = filepath.Join(t.TempDir(), "state.json")
			f.player.SessionID = "session"
			owner := ActiveState{SessionID: "session", DeviceID: "device", TrackURI: f.state.Item.URI}
			evidence := f.player.Checkpoint()
			f.state.IsPlaying = false
			switch scenario {
			case "regressed":
				f.state.ProgressMS = 0
			case "finished":
				f.state.ProgressMS = 180000
			case "playing":
				f.state.IsPlaying = true
			case "other-owner":
				owner.SessionID = "other"
			}
			if err := f.player.writeState(owner); err != nil {
				t.Fatal(err)
			}
			recovered := &Player{Client: f.player.Client, StatePath: f.player.StatePath, SessionID: "session", Now: func() time.Time { return f.now }}
			err := recovered.Restore(context.Background(), evidence)
			if scenario == "paused" {
				if err != nil || recovered.control.phase != "recovering" || recovered.lastProgress != 45000 {
					t.Fatalf("restore=%v state=%+v", err, recovered.control)
				}
			} else if err == nil {
				t.Fatal("ambiguous playback accepted")
			}
			if len(f.commands) != 0 {
				t.Fatalf("restoration issued playback commands: %v", f.commands)
			}
		})
	}
}

func TestMissingAndStaleObservationsFailWithinRecoveryBudget(t *testing.T) {
	for _, stale := range []bool{false, true} {
		f := newControlFixture(t)
		if stale {
			f.state.Timestamp = f.player.startedAt.Add(-time.Hour).UnixMilli()
		} else {
			f.state = PlaybackState{}
		}
		f.check(t, stallLimit)
		f.now = f.now.Add(recoveryLimit)
		if _, err := f.player.Finished(context.Background()); err == nil {
			t.Fatal("missing/stale telemetry kept session alive")
		}
		if len(f.commands) != 0 {
			t.Fatal("missing/stale state authorized a command")
		}
	}
}

func TestShortTrackPauseStillRequiresNearEndEvidence(t *testing.T) {
	f := newControlFixture(t)
	f.player.playDurationMS = 5000
	f.player.maxProgress = 500
	f.player.lastProgress = 500
	f.state.Item.DurationMS = 5000
	f.state.ProgressMS = 500
	f.state.IsPlaying = false
	if f.check(t, 0) {
		t.Fatal("early short-track interruption counted as completion")
	}
	if f.player.control.phase != "recovering" {
		t.Fatal("short track was not protected")
	}
}

func TestFailedCleanupRetainsOwnershipEvidence(t *testing.T) {
	f := newControlFixture(t)
	f.player.StatePath = filepath.Join(t.TempDir(), "owner.json")
	f.player.SessionID = "owner"
	f.player.stateTrackURI = f.player.trackURI
	f.player.prepared, f.player.attempted = true, true
	if err := f.player.writeState(ActiveState{SessionID: "owner", DeviceID: "device", TrackURI: f.player.trackURI}); err != nil {
		t.Fatal(err)
	}
	f.limited = true
	if err := f.player.Close(context.Background()); err == nil {
		t.Fatal("failed stop reported success")
	}
	if _, err := os.Stat(f.player.StatePath); err != nil {
		t.Fatal("uncertain playback lost ownership fence", err)
	}
}
