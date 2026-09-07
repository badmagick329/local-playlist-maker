package spotify

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"playlistmaker/charm/internal/tracking"
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
	response := `{"is_playing":true,"device":{"id":"device"},"item":{"uri":"spotify:track:0HTI5hPxJ9NCEEUADrNBk8","name":"SAUCIN'","duration_ms":182366,"artists":[{"name":"NEXZ"}]},"progress_ms":1000}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()
	player := &Player{Client: &Client{Auth: validAuth(t, server), HTTP: server.Client(), APIBase: server.URL}, deviceID: "device", trackURI: expected.URI, requestedTrack: expected, startedAt: time.Now()}
	if done, err := player.Finished(context.Background()); done || err != nil || !player.observed {
		t.Fatalf("SAUCIN release not acknowledged: done=%v observed=%v err=%v", done, player.observed, err)
	}
	response = `{"is_playing":false,"device":{"id":"device"},"item":{"uri":"spotify:track:0HTI5hPxJ9NCEEUADrNBk8","name":"SAUCIN'","duration_ms":182366,"artists":[{"name":"NEXZ"}]},"progress_ms":0}`
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
	response := `{"is_playing":true,"device":{"id":"device"},"item":{"uri":"spotify:track:actual","name":"RUN IT","duration_ms":228681,"artists":[{"name":"Stray Kids"}]},"progress_ms":0}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()
	player := &Player{Client: &Client{Auth: validAuth(t, server), HTTP: server.Client(), APIBase: server.URL}, deviceID: "device", trackURI: expected.URI, requestedTrack: expected, startedAt: time.Now()}
	if done, err := player.Finished(context.Background()); done || err != nil || !player.observed {
		t.Fatalf("alternate release not recognized: %v, %v", done, err)
	}
	response = `{"is_playing":true,"device":{"id":"device"},"item":{"uri":"spotify:track:actual","name":"RUN IT","duration_ms":228681,"artists":[{"name":"Stray Kids"}]},"progress_ms":210000}`
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
	response := `{"is_playing":false,"device":{"id":"device"},"item":{"uri":"spotify:track:song","duration_ms":240000},"progress_ms":240000}`
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
	response = `{"is_playing":true,"device":{"id":"device"},"item":{"uri":"spotify:track:song","duration_ms":240000},"progress_ms":239000}`
	check(false)
	if player.observed {
		t.Fatal("previous repeat was mistaken for the new play")
	}
	response = `{"is_playing":true,"device":{"id":"device"},"item":{"uri":"spotify:track:song","duration_ms":240000},"progress_ms":0}`
	check(false)
	before := requests
	if _, err := player.Finished(context.Background()); err != nil {
		t.Fatal(err)
	}
	if requests != before {
		t.Fatal("polled before the next check was due")
	}
	response = `{"is_playing":true,"device":{"id":"device"},"item":{"uri":"spotify:track:song","duration_ms":240000},"progress_ms":180000}`
	check(false)
	response = `{"is_playing":false,"device":{"id":"device"},"item":{"uri":"spotify:track:song","duration_ms":240000},"progress_ms":240000}`
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
	response = `{"is_playing":true,"device":{"id":"device"},"item":{"uri":"spotify:track:song","duration_ms":240000},"progress_ms":1000}`
	player.nextCheck = time.Time{}
	done, err = player.Finished(context.Background())
	if done || err != nil || !player.observed {
		t.Fatalf("late start was not accepted: %v, %v", done, err)
	}
}
