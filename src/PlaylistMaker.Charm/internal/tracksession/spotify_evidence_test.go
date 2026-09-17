package tracksession

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

	"playlistmaker/charm/internal/spotify"
	"playlistmaker/charm/internal/tracking"
)

// Exercise the real queue and Spotify controller together: a completion result
// can issue the next Start immediately, before the runner publishes status.
type spotifyQueueFixture struct {
	queue    *playQueue
	player   *spotify.Player
	now      time.Time
	state    spotify.PlaybackState
	starts   []string
	resumes  int
	commands int
}

func newSpotifyQueueFixture(t *testing.T) *spotifyQueueFixture {
	t.Helper()
	f := &spotifyQueueFixture{now: time.Now()}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/me/player/devices":
			json.NewEncoder(w).Encode(map[string]any{"devices": []spotify.Device{{ID: "device", Name: "fixture"}}})
		case strings.HasPrefix(r.URL.Path, "/tracks/"):
			json.NewEncoder(w).Encode(spotify.Track{URI: "spotify:track:" + strings.TrimPrefix(r.URL.Path, "/tracks/"), DurationMS: 180000})
		case r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(f.state)
		default:
			f.commands++
			if r.URL.Path == "/me/player/play" {
				body, _ := io.ReadAll(r.Body)
				if len(body) == 0 {
					f.resumes++
				} else {
					var request struct {
						URIs []string `json:"uris"`
					}
					if err := json.Unmarshal(body, &request); err != nil {
						t.Error(err)
						return
					}
					f.starts = append(f.starts, request.URIs[0])
					f.state = spotify.PlaybackState{IsPlaying: true, RepeatState: "off", Timestamp: f.now.UnixMilli(), Item: &spotify.Track{URI: request.URIs[0], DurationMS: 180000}}
					f.state.Device.ID = "device"
				}
			}
			if r.URL.Path == "/me/player/pause" {
				f.state.IsPlaying = false
				f.state.Timestamp = f.now.UnixMilli()
			}
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	t.Cleanup(server.Close)
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")
	data, _ := json.Marshal(spotify.Token{AccessToken: "synthetic-token", ExpiresAtUTC: time.Now().Add(time.Hour)})
	if err := os.WriteFile(authPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	f.player = &spotify.Player{Client: &spotify.Client{Auth: &spotify.Auth{TokenPath: authPath}, HTTP: server.Client(), APIBase: server.URL}, StatePath: filepath.Join(dir, "owner.json"), SessionID: "queue-test", Now: func() time.Time { return f.now }}
	runtime := &Runtime{Spotify: f.player}
	if err := runtime.Prepare(context.Background(), "fixture", []Entry{{Track: tracking.Track{SpotifyURI: "spotify:track:first"}}}); err != nil {
		t.Fatal(err)
	}
	f.queue = &playQueue{runtime: runtime}
	if err := f.queue.load(context.Background(), "first", 0, tracking.Track{SpotifyURI: "spotify:track:first"}); err != nil {
		t.Fatal(err)
	}
	f.state.ProgressMS = 500
	f.tick(t, time.Second)
	return f
}

func (f *spotifyQueueFixture) tick(t *testing.T, elapsed time.Duration) {
	t.Helper()
	f.now = f.now.Add(elapsed)
	if err := f.queue.tick(context.Background()); err != nil {
		t.Fatal(err)
	}
}
func (f *spotifyQueueFixture) enqueueNext(t *testing.T) {
	t.Helper()
	f.queue.end(context.Background(), "eof")
	if err := f.queue.load(context.Background(), "second", 1, tracking.Track{SpotifyURI: "spotify:track:second"}); err != nil {
		t.Fatal(err)
	}
}

func TestQueueCannotAdvanceAfterNearEndDeviceTakeover(t *testing.T) {
	f := newSpotifyQueueFixture(t)
	f.enqueueNext(t)
	f.state.ProgressMS = 175000
	f.tick(t, 175*time.Second)
	commands := f.commands
	f.state.Device.ID = "other-device"
	f.state.Item.URI = "spotify:track:unrelated"
	f.now = f.now.Add(5 * time.Second)
	if err := f.queue.tick(context.Background()); err == nil {
		t.Fatal("takeover did not block queue")
	}
	if !f.queue.status("queue-test", 1).Hold {
		t.Fatal("takeover did not retain video hold")
	}
	f.player.Retry()
	f.now = f.now.Add(time.Second)
	if err := f.queue.tick(context.Background()); err == nil {
		t.Fatal("retry bypassed device takeover")
	}
	if len(f.starts) != 1 || f.resumes != 0 || f.commands != commands || f.queue.active.id != "first" || len(f.queue.pending) != 1 {
		t.Fatalf("takeover advanced queue: starts=%v resumes=%d commands=%d", f.starts, f.resumes, f.commands)
	}
}

func TestQueueAdvancesExactlyOnceAfterShortTailResume(t *testing.T) {
	f := newSpotifyQueueFixture(t)
	ceiling := 179500
	f.queue.paused = true
	f.queue.target = &ceiling
	f.state.ProgressMS = ceiling
	f.tick(t, 179*time.Second)
	f.tick(t, time.Second)
	f.enqueueNext(t)
	f.queue.paused = false
	f.tick(t, time.Second)
	if f.resumes != 1 || !f.queue.status("queue-test", 1).Hold {
		t.Fatal("accepted resume released hold or did not resume")
	}
	f.state.ProgressMS = 180000
	f.state.Timestamp = f.now.Add(time.Second).UnixMilli()
	f.tick(t, time.Second)
	if len(f.starts) != 2 || f.starts[1] != "spotify:track:second" || f.resumes != 1 || f.queue.active.id != "second" {
		t.Fatalf("short tail did not advance exactly once: starts=%v resumes=%d", f.starts, f.resumes)
	}
	f.state.ProgressMS = 500
	f.tick(t, time.Second)
	f.tick(t, time.Second)
	if len(f.starts) != 2 || f.resumes != 1 {
		t.Fatal("completed occurrence replayed after queue advancement")
	}
	if f.queue.status("queue-test", 2).Hold {
		t.Fatal("verified next occurrence did not release hold")
	}
}

func TestQueueStillAdvancesAfterNaturalCompletion(t *testing.T) {
	f := newSpotifyQueueFixture(t)
	f.enqueueNext(t)
	f.state.ProgressMS = 179000
	f.tick(t, 179*time.Second)
	f.state.IsPlaying = false
	f.state.ProgressMS = 180000
	f.tick(t, 2*time.Second)
	if len(f.starts) != 2 || f.resumes != 0 || f.queue.active.id != "second" {
		t.Fatal("natural completion no longer advances queue")
	}
}
