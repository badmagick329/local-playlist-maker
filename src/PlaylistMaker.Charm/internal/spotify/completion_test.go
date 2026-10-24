package spotify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"playlistmaker/charm/internal/tracking"
	"strings"
	"testing"
	"time"
)

func TestPlaybackDeadlineCannotBeExtendedByPollingBackoff(t *testing.T) {
	for _, observed := range []bool{false, true} {
		p := &Player{observed: observed, startedAt: time.Now().Add(-31 * time.Second), completionDeadline: time.Now().Add(-time.Second), nextCheck: time.Now().Add(time.Hour), lastPlayback: "Skibidi (Performance Video)"}
		done, err := p.Finished(context.Background())
		var failure *tracking.PlaybackFailure
		if done || !errors.As(err, &failure) {
			t.Fatalf("deadline retained session: %v %v", done, err)
		}
	}
}

func TestCloseDoesNotPauseUnrelatedPlayback(t *testing.T) {
	pauses := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			pauses++
			return
		}
		_, _ = w.Write([]byte(`{"is_playing":true,"device":{"id":"device"},"item":{"uri":"spotify:track:unrelated"}}`))
	}))
	defer server.Close()
	p := &Player{Client: &Client{Auth: validAuth(t, server), HTTP: server.Client(), APIBase: server.URL}, prepared: true, attempted: true, deviceID: "device", trackURI: "spotify:track:requested", StatePath: t.TempDir() + "/state.json"}
	if err := p.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if pauses != 0 {
		t.Fatal("cleanup paused unrelated playback")
	}
}

func TestFinishedRecognizesSaucinReleaseAndReleasesQueue(t *testing.T) {
	var expected Track
	_ = json.Unmarshal([]byte(`{"uri":"spotify:track:12bPNXcJVrUyLSrGRxHQW2","name":"SAUCIN′","duration_ms":163717,"artists":[{"name":"NEXZ"}]}`), &expected)
	response := `{"is_playing":true,"repeat_state":"off","device":{"id":"device"},"item":{"uri":"spotify:track:0HTI5hPxJ9NCEEUADrNBk8","name":"SAUCIN'","duration_ms":182366,"artists":[{"name":"NEXZ"}]},"progress_ms":1000}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()
	player := &Player{Client: &Client{Auth: validAuth(t, server), HTTP: server.Client(), APIBase: server.URL}, deviceID: "device", trackURI: expected.URI, requestedTrack: expected, startedAt: time.Now()}
	if done, err := player.Finished(context.Background()); done || err != nil || !player.observed {
		t.Fatalf("SAUCIN release not acknowledged: done=%v observed=%v err=%v", done, player.observed, err)
	}
	response = `{"is_playing":false,"repeat_state":"off","device":{"id":"device"},"item":{"uri":"spotify:track:0HTI5hPxJ9NCEEUADrNBk8","name":"SAUCIN'","duration_ms":182366,"artists":[{"name":"NEXZ"}]},"progress_ms":182366}`
	player.nextCheck = time.Time{}
	if done, err := player.Finished(context.Background()); !done || err != nil {
		t.Fatalf("completed play blocks queue: done=%v err=%v", done, err)
	}
	player.observed = false
	other := expected
	other.URI = "spotify:track:other"
	other.Name = "SAUCIN' (Remix)"
	if player.matchesTrack(other) {
		t.Fatal("different version accepted")
	}
}

func TestFinishedRecognizesAlternateReleaseAndUsesItsDuration(t *testing.T) {
	var expected Track
	if err := json.Unmarshal([]byte(`{"uri":"spotify:track:requested","name":"RUN IT","duration_ms":209000,"artists":[{"name":"Stray Kids"}]}`), &expected); err != nil {
		t.Fatal(err)
	}
	response := `{"is_playing":true,"repeat_state":"off","device":{"id":"device"},"item":{"uri":"spotify:track:actual","name":"RUN IT","duration_ms":228681,"artists":[{"name":"Stray Kids"}]},"progress_ms":0}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()
	player := &Player{Client: &Client{Auth: validAuth(t, server), HTTP: server.Client(), APIBase: server.URL}, deviceID: "device", trackURI: expected.URI, requestedTrack: expected, startedAt: time.Now()}
	if done, err := player.Finished(context.Background()); done || err != nil || !player.observed {
		t.Fatalf("alternate release not recognized: %v, %v", done, err)
	}
	response = `{"is_playing":true,"repeat_state":"off","device":{"id":"device"},"item":{"uri":"spotify:track:actual","name":"RUN IT","duration_ms":228681,"artists":[{"name":"Stray Kids"}]},"progress_ms":210000}`
	player.nextCheck = time.Time{}
	if done, err := player.Finished(context.Background()); done || err != nil {
		t.Fatalf("longer release ended at requested duration: %v, %v", done, err)
	}
	actual := expected
	player.observed = false
	actual.URI = "spotify:track:other"
	actual.Artists = append(actual.Artists[:0:0], actual.Artists...)
	actual.Artists[0].Name = "Another artist"
	if player.matchesTrack(actual) {
		t.Fatal("accepted same title by another artist")
	}
}

func TestFinishedWaitsForNewPlayAndNaturalCompletion(t *testing.T) {
	response := `{"is_playing":false,"repeat_state":"off","device":{"id":"device"},"item":{"uri":"spotify:track:song","duration_ms":240000},"progress_ms":240000}`
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodGet || r.URL.Path != "/me/player" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()
	player := &Player{Client: &Client{Auth: validAuth(t, server), HTTP: server.Client(), APIBase: server.URL}, deviceID: "device", trackURI: "spotify:track:song", startedAt: time.Now()}
	check := func(want bool) {
		t.Helper()
		player.nextCheck = time.Time{}
		got, err := player.Finished(context.Background())
		if err != nil || got != want {
			t.Fatalf("finished = %v, %v; want %v", got, err, want)
		}
	}
	check(false) // The previous repeat's stopped state is not the new play ending.
	response = `{"is_playing":true,"repeat_state":"off","device":{"id":"device"},"item":{"uri":"spotify:track:song","duration_ms":240000},"progress_ms":239000}`
	check(false)
	if player.observed {
		t.Fatal("previous repeat was mistaken for the new play")
	}
	response = `{"is_playing":true,"repeat_state":"off","device":{"id":"device"},"item":{"uri":"spotify:track:song","duration_ms":240000},"progress_ms":0}`
	check(false)
	before := requests
	if _, err := player.Finished(context.Background()); err != nil {
		t.Fatal(err)
	}
	if requests != before {
		t.Fatal("polled before the next check was due")
	}
	response = `{"is_playing":true,"repeat_state":"off","device":{"id":"device"},"item":{"uri":"spotify:track:song","duration_ms":240000},"progress_ms":180000}`
	check(false)
	response = `{"is_playing":false,"repeat_state":"off","device":{"id":"device"},"item":{"uri":"spotify:track:song","duration_ms":240000},"progress_ms":240000}`
	check(true)
}

func TestFinishedHonorsRateLimitWithoutEndingSong(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	player := &Player{Client: &Client{Auth: validAuth(t, server), HTTP: server.Client(), APIBase: server.URL}}
	for range 2 {
		done, err := player.Finished(context.Background())
		if err != nil || done {
			t.Fatalf("rate limit ended play: %v, %v", done, err)
		}
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
}

func TestFinishedAcceptsPlaybackThatStartsAfterTimeout(t *testing.T) {
	response := `{"is_playing":false}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()
	player := &Player{Client: &Client{Auth: validAuth(t, server), HTTP: server.Client(), APIBase: server.URL}, deviceID: "device", trackURI: "spotify:track:song", startedAt: time.Now().Add(-20 * time.Second)}
	done, err := player.Finished(context.Background())
	if done || err == nil {
		t.Fatalf("expected pending playback diagnostic, got %v, %v", done, err)
	}
	response = `{"is_playing":true,"repeat_state":"off","device":{"id":"device"},"item":{"uri":"spotify:track:song","duration_ms":240000},"progress_ms":1000}`
	player.nextCheck = time.Time{}
	done, err = player.Finished(context.Background())
	if done || err != nil || !player.observed {
		t.Fatalf("late start was not accepted: %v, %v", done, err)
	}
}

func TestFinishedDoesNotAcceptUnrelatedOrIdleStateWithoutNearEndEvidence(t *testing.T) {
	response := `{"is_playing":true,"repeat_state":"off","device":{"id":"device"},"item":{"uri":"spotify:track:song","duration_ms":240000},"progress_ms":1000}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()
	player := &Player{Client: &Client{Auth: validAuth(t, server), HTTP: server.Client(), APIBase: server.URL}, deviceID: "device", trackURI: "spotify:track:song", startedAt: time.Now()}
	checkPending := func() {
		t.Helper()
		player.nextCheck = time.Time{}
		if done, err := player.Finished(context.Background()); done || err != nil {
			t.Fatalf("unproven completion accepted: done=%v err=%v", done, err)
		}
	}
	checkPending()
	response = `{"is_playing":false,"repeat_state":"off","device":{"id":"device"},"item":{"uri":"spotify:track:song","duration_ms":240000},"progress_ms":1000}`
	checkPending()
	response = `{"is_playing":true,"repeat_state":"off","device":{"id":"device"},"item":{"uri":"spotify:track:other","duration_ms":200000},"progress_ms":5000}`
	player.nextCheck = time.Time{}
	if done, err := player.Finished(context.Background()); done || err == nil {
		t.Fatalf("takeover not blocked: %v %v", done, err)
	}
}

func TestFinishedDoesNotTreatBackwardProgressAsCompletion(t *testing.T) {
	response := `{"is_playing":true,"repeat_state":"off","device":{"id":"device"},"item":{"uri":"spotify:track:song","duration_ms":240000},"progress_ms":0}`
	diagnostics := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()
	player := &Player{Client: &Client{Auth: validAuth(t, server), HTTP: server.Client(), APIBase: server.URL}, deviceID: "device", trackURI: "spotify:track:song", startedAt: time.Now()}
	player.SetDiagnostic(func(message string) { diagnostics = append(diagnostics, message) })
	check := func() bool {
		player.nextCheck = time.Time{}
		done, err := player.Finished(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		return done
	}
	if check() {
		t.Fatal("initial state completed")
	}
	response = `{"is_playing":true,"repeat_state":"off","device":{"id":"device"},"item":{"uri":"spotify:track:song","duration_ms":240000},"progress_ms":239000}`
	if check() {
		t.Fatal("near-end playback completed while playing")
	}
	response = `{"is_playing":true,"repeat_state":"off","device":{"id":"device"},"item":{"uri":"spotify:track:song","duration_ms":240000},"progress_ms":0}`
	if check() {
		t.Fatal("backward jump completed the occurrence")
	}
	response = `{"is_playing":false,"repeat_state":"off","device":{"id":"device"},"item":{"uri":"spotify:track:song","duration_ms":240000},"progress_ms":0}`
	if check() {
		t.Fatal("stop after backward jump used stale near-end evidence")
	}
	response = `{"is_playing":true,"repeat_state":"off","device":{"id":"device"},"item":{"uri":"spotify:track:other","duration_ms":200000},"progress_ms":5000}`
	player.nextCheck = time.Time{}
	if done, err := player.Finished(context.Background()); done || err == nil {
		t.Fatalf("takeover not blocked: %v %v", done, err)
	}
	if !strings.Contains(strings.Join(diagnostics, "\n"), "backward") {
		t.Fatalf("missing backward-progress diagnostic: %#v", diagnostics)
	}
}

func TestFinishedAcceptsStoppedPositionResetAfterNearEnd(t *testing.T) {
	states := []string{
		`{"is_playing":true,"repeat_state":"off","device":{"id":"device"},"item":{"uri":"spotify:track:song","duration_ms":240000},"progress_ms":0}`,
		`{"is_playing":true,"repeat_state":"off","device":{"id":"device"},"item":{"uri":"spotify:track:song","duration_ms":240000},"progress_ms":239000}`,
		`{"is_playing":false,"repeat_state":"off","device":{"id":"device"},"item":{"uri":"spotify:track:song","duration_ms":240000},"progress_ms":0}`,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(states[0]))
		states = states[1:]
	}))
	defer server.Close()
	player := &Player{Client: &Client{Auth: validAuth(t, server), HTTP: server.Client(), APIBase: server.URL}, deviceID: "device", trackURI: "spotify:track:song", startedAt: time.Now()}
	for index, want := range []bool{false, false, true} {
		player.nextCheck = time.Time{}
		if done, err := player.Finished(context.Background()); done != want || err != nil {
			t.Fatalf("state %d: finished = %v, %v; want %v", index, done, err, want)
		}
	}
}

func TestFinishedAcceptsMissedStopAfterObservedNearEndProgress(t *testing.T) {
	response := `{"is_playing":true,"repeat_state":"off","device":{"id":"device"},"item":{"uri":"spotify:track:song","duration_ms":240000},"progress_ms":0}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()
	player := &Player{Client: &Client{Auth: validAuth(t, server), HTTP: server.Client(), APIBase: server.URL}, deviceID: "device", trackURI: "spotify:track:song", startedAt: time.Now()}
	check := func() (bool, error) {
		player.nextCheck = time.Time{}
		return player.Finished(context.Background())
	}
	if done, err := check(); done || err != nil {
		t.Fatalf("initial state = %v, %v", done, err)
	}
	response = `{"is_playing":true,"repeat_state":"off","device":{"id":"device"},"item":{"uri":"spotify:track:song","duration_ms":240000},"progress_ms":239000}`
	if done, err := check(); done || err != nil {
		t.Fatalf("near-end state = %v, %v", done, err)
	}
	response = `{"is_playing":true,"repeat_state":"off","device":{"id":"device"},"item":{"uri":"spotify:track:other","duration_ms":200000},"progress_ms":1000}`
	if done, err := check(); !done || err != nil {
		t.Fatalf("missed stop transition = %v, %v", done, err)
	}
}

func TestFinishedRetriesAndBoundsUnconfirmedRepeatState(t *testing.T) {
	now := time.Unix(1000, 0)
	repeatRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/me/player/repeat" {
			repeatRequests++
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"is_playing":true,"repeat_state":"context","device":{"id":"device"},"item":{"uri":"spotify:track:song","duration_ms":240000},"progress_ms":1000}`))
	}))
	defer server.Close()
	player := &Player{Client: &Client{Auth: validAuth(t, server), HTTP: server.Client(), APIBase: server.URL}, deviceID: "device", trackURI: "spotify:track:song", startedAt: now, Now: func() time.Time { return now }}
	if done, err := player.Finished(context.Background()); done || err != nil {
		t.Fatalf("first repeat check = %v, %v", done, err)
	}
	now = now.Add(5 * time.Second)
	player.nextCheck = time.Time{}
	if done, err := player.Finished(context.Background()); done || err != nil {
		t.Fatalf("repeat retry = %v, %v", done, err)
	}
	if repeatRequests != 2 {
		t.Fatalf("repeat disable requests = %d, want 2", repeatRequests)
	}
	now = now.Add(26 * time.Second)
	player.nextCheck = now.Add(time.Hour)
	if done, err := player.Finished(context.Background()); done || err == nil {
		t.Fatalf("unconfirmed repeat was not bounded: done=%v err=%v", done, err)
	}
}

func TestFinishedIgnoresPlaybackResponseOlderThanStart(t *testing.T) {
	now := time.Unix(1000, 0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"is_playing":false,"repeat_state":"off","timestamp":990000,"device":{"id":"device"},"item":{"uri":"spotify:track:song","duration_ms":240000},"progress_ms":240000}`))
	}))
	defer server.Close()
	player := &Player{Client: &Client{Auth: validAuth(t, server), HTTP: server.Client(), APIBase: server.URL}, deviceID: "device", trackURI: "spotify:track:song", startedAt: now, Now: func() time.Time { return now }}
	if done, err := player.Finished(context.Background()); done || err != nil || player.observed {
		t.Fatalf("stale response changed completion state: done=%v observed=%v err=%v", done, player.observed, err)
	}
}

func (f *controlFixture) sample(t *testing.T, playing bool, progress int, after time.Duration) bool {
	t.Helper()
	f.now = f.now.Add(after)
	f.state.IsPlaying, f.state.ProgressMS, f.state.Timestamp = playing, progress, f.now.UnixMilli()
	return f.check(t, 0)
}

// librespot's end of "Looks Can Kill": near the end, then rewound to the start
// while still reported as playing.
func rewoundAtEnd(t *testing.T) (*controlFixture, *[]string) {
	t.Helper()
	f := newControlFixture(t)
	diagnostics := []string{}
	f.player.SetDiagnostic(func(message string) { diagnostics = append(diagnostics, message) })
	f.state.Item.DurationMS, f.player.playDurationMS = 195559, 195559
	if f.sample(t, true, 192729, 0) || f.sample(t, true, 647, 3*time.Second) {
		t.Fatal("playing sample completed the occurrence")
	}
	return f, &diagnostics
}

func TestFinishedAcceptsRewindToStartThenPauseAfterNearEnd(t *testing.T) {
	f, diagnostics := rewoundAtEnd(t)
	if !f.sample(t, false, 211, 5*time.Second) {
		t.Fatal("rewound and paused ending was not accepted")
	}
	if len(f.commands) != 0 {
		t.Fatalf("finished song was resumed: %v", f.commands)
	}
	if !strings.Contains(strings.Join(*diagnostics, "\n"), "completion decision: accepted rewind-to-start endpoint after reaching 192729ms of 195559ms") {
		t.Fatalf("missing rewind completion diagnostic: %v", *diagnostics)
	}
}

func TestRewindFollowedByReplayIsNotCompletion(t *testing.T) {
	for _, stopped := range []int{3200, 200} {
		t.Run(fmt.Sprint(stopped), func(t *testing.T) {
			f, _ := rewoundAtEnd(t)
			if f.sample(t, true, 3000, 3*time.Second) || f.sample(t, false, stopped, time.Second) {
				t.Fatal("replayed song completed the occurrence")
			}
			if f.player.control.phase != "recovering" || len(f.commands) != 1 || f.commands[0] != "/me/player/play:" {
				t.Fatalf("interrupted replay was not recovered in place: phase=%s commands=%v", f.player.control.phase, f.commands)
			}
		})
	}
}

func TestMidSongRewindThenPauseIsRecovered(t *testing.T) {
	f := newControlFixture(t)
	if f.sample(t, true, 50000, 0) || f.sample(t, true, 819, 3*time.Second) || f.sample(t, false, 211, 5*time.Second) {
		t.Fatal("mid-song rewind completed the occurrence")
	}
	if len(f.commands) != 1 || f.commands[0] != "/me/player/play:" {
		t.Fatalf("mid-song stop was not recovered in place: %v", f.commands)
	}
}

func TestDeliberatePauseAfterRewindIsNotCompletion(t *testing.T) {
	f, _ := rewoundAtEnd(t)
	f.player.Intent(true, nil)
	if f.sample(t, true, 700, time.Second) || f.sample(t, false, 211, time.Second) {
		t.Fatal("our own pause after a rewind was mistaken for completion")
	}
	if len(f.commands) != 1 || f.commands[0] != "/me/player/pause:" || f.player.control.phase != "paused" {
		t.Fatalf("deliberate hold not kept: phase=%s commands=%v", f.player.control.phase, f.commands)
	}
}

// After a near-end sample, Spotify moves on to the next item in context.
func nextAfterNearEnd(t *testing.T, contextURI string, failPause bool) (bool, *controlFixture, *[]string) {
	t.Helper()
	f := newControlFixture(t)
	f.failPause = failPause
	diagnostics := []string{}
	f.player.SetDiagnostic(func(message string) { diagnostics = append(diagnostics, message) })
	f.player.albumURI = f.state.Item.Album.URI
	if f.sample(t, true, 175000, 0) {
		t.Fatal("near-end playing sample completed the occurrence")
	}
	f.state.Item = &Track{URI: "spotify:track:next", Name: "Next", DurationMS: 200000}
	if contextURI != "" {
		f.state.Context = &struct {
			URI string `json:"uri"`
		}{contextURI}
	}
	return f.sample(t, true, 1000, time.Second), f, &diagnostics
}

func TestCompletionPausesNextTrackInStartedAlbum(t *testing.T) {
	done, f, diagnostics := nextAfterNearEnd(t, "spotify:album:fixture", false)
	if !done {
		t.Fatal("transition with near-end evidence was not accepted")
	}
	if len(f.commands) != 1 || f.commands[0] != "/me/player/pause:" {
		t.Fatalf("album follow-on was not paused: %v", f.commands)
	}
	if !strings.Contains(strings.Join(*diagnostics, "\n"), "completion follow-on pause: paused next album track") {
		t.Fatalf("missing follow-on pause diagnostic: %v", *diagnostics)
	}
}

func TestCompletionLeavesNextTrackOutsideStartedAlbum(t *testing.T) {
	for name, contextURI := range map[string]string{"other-context": "spotify:playlist:other", "no-context": ""} {
		t.Run(name, func(t *testing.T) {
			done, f, _ := nextAfterNearEnd(t, contextURI, false)
			if !done {
				t.Fatal("transition with near-end evidence was not accepted")
			}
			if len(f.commands) != 0 {
				t.Fatalf("unrelated playback was mutated: %v", f.commands)
			}
		})
	}
}

func TestCompletionStandsWhenFollowOnPauseFails(t *testing.T) {
	done, f, diagnostics := nextAfterNearEnd(t, "spotify:album:fixture", true)
	if !done {
		t.Fatal("failed follow-on pause undid completion")
	}
	if len(f.commands) != 1 || f.commands[0] != "/me/player/pause:" {
		t.Fatalf("follow-on pause was not attempted: %v", f.commands)
	}
	if !strings.Contains(strings.Join(*diagnostics, "\n"), "completion follow-on pause failed") {
		t.Fatalf("missing follow-on pause failure diagnostic: %v", *diagnostics)
	}
}

func TestNewOccurrenceDoesNotInheritRewindEvidence(t *testing.T) {
	f, _ := rewoundAtEnd(t)
	p := f.player
	p.prepared, p.SessionID, p.StatePath = true, "rewind", filepath.Join(t.TempDir(), "owner.json")
	p.requestedTrack = *f.state.Item
	if err := p.Start(context.Background(), tracking.Track{SpotifyURI: f.state.Item.URI}); err != nil {
		t.Fatal(err)
	}
	f.commands = nil
	if f.sample(t, true, 1000, time.Second) || !p.observed {
		t.Fatal("new occurrence was not observed")
	}
	if f.sample(t, false, 211, time.Second) {
		t.Fatal("previous occurrence's rewind completed the new one")
	}
	if len(f.commands) != 1 || f.commands[0] != "/me/player/play:" {
		t.Fatalf("early stop was not recovered in place: %v", f.commands)
	}
}
