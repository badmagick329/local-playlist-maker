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
	f.queue = &playQueue{runtime: runtime, now: func() time.Time { return f.now }}
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

func (f *spotifyQueueFixture) finishSong(t *testing.T) {
	t.Helper()
	f.state.ProgressMS = 179000
	f.tick(t, 179*time.Second)
	f.state.IsPlaying = false
	f.state.ProgressMS = 180000
	f.tick(t, 2*time.Second)
}

func (f *spotifyQueueFixture) loadVideo(t *testing.T, id string, position int) {
	t.Helper()
	f.queue.end(context.Background(), "eof")
	if err := f.queue.load(context.Background(), id, position, tracking.Track{SpotifyURI: "spotify:track:" + id}); err != nil {
		t.Fatal(err)
	}
}

func TestLateSongStartDoesNotHoldPlayingVideoWhileConfirming(t *testing.T) {
	f := newSpotifyQueueFixture(t)
	f.enqueueNext(t)
	f.finishSong(t)
	if f.queue.active == nil || f.queue.active.id != "second" {
		t.Fatal("queued song did not start after the previous song finished")
	}
	if status := f.queue.status("queue-test", 1); status.Hold || status.State != "starting" {
		t.Fatalf("late start held the playing video: %+v", status)
	}
	f.tick(t, time.Second)
	if status := f.queue.status("queue-test", 2); status.Hold || status.State != "playing" {
		t.Fatalf("confirmed late start did not continue playing: %+v", status)
	}
}

func TestLateSongStartHoldsVideoWhenUnconfirmedAfterGrace(t *testing.T) {
	f := newSpotifyQueueFixture(t)
	f.enqueueNext(t)
	f.finishSong(t)
	// Connect keeps reporting the finished song instead of the requested one.
	f.state = spotify.PlaybackState{RepeatState: "off", Timestamp: f.now.UnixMilli(), ProgressMS: 180000, Item: &spotify.Track{URI: "spotify:track:first", DurationMS: 180000}}
	f.state.Device.ID = "device"
	f.tick(t, time.Second)
	if f.queue.status("queue-test", 1).Hold {
		t.Fatal("late start held before its grace period expired")
	}
	f.tick(t, 4*time.Second)
	if status := f.queue.status("queue-test", 2); !status.Hold || status.State != "starting" {
		t.Fatalf("unconfirmed late start did not hold the video: %+v", status)
	}
}

func TestSongStartingWithItsVideoHoldsUntilConfirmed(t *testing.T) {
	f := newSpotifyQueueFixture(t)
	f.queue.end(context.Background(), "eof")
	f.finishSong(t)
	if !f.queue.idle() {
		t.Fatal("finished song left the queue busy")
	}
	f.loadVideo(t, "second", 1)
	if status := f.queue.status("queue-test", 1); !status.Hold || status.State != "starting" {
		t.Fatalf("song starting with its video did not hold until confirmed: %+v", status)
	}
	f.tick(t, time.Second)
	if f.queue.status("queue-test", 2).Hold {
		t.Fatal("confirmed start did not release the hold")
	}
}

// A paused video several songs ahead must not pause the backlog. Earlier songs
// play out, and the paused video's own song catches up to its position.
func TestPausedVideoLetsBacklogPlayAndCatchUp(t *testing.T) {
	f := newSpotifyQueueFixture(t)
	f.enqueueNext(t)
	f.loadVideo(t, "third", 2)
	ceiling := 60000
	f.queue.paused, f.queue.target = true, &ceiling
	f.tick(t, time.Second)
	if !f.state.IsPlaying || f.queue.active.id != "first" {
		t.Fatal("paused later video paused an earlier song")
	}
	f.finishSong(t)
	f.tick(t, time.Second)
	if f.queue.active.id != "second" || !f.state.IsPlaying || f.queue.status("queue-test", 1).Hold {
		t.Fatal("backlog song did not play behind the paused video")
	}
	f.finishSong(t)
	f.tick(t, time.Second)
	if f.queue.active.id != "third" || !f.state.IsPlaying {
		t.Fatal("paused video's song did not start catching up")
	}
	if phase, _ := f.player.TrackingStatus(); phase != "catch-up" {
		t.Fatalf("paused video's song phase = %q, want catch-up", phase)
	}
	f.state.ProgressMS = ceiling
	f.tick(t, time.Second)
	f.tick(t, time.Second)
	if phase, _ := f.player.TrackingStatus(); f.state.IsPlaying || phase != "paused" {
		t.Fatalf("song did not pause at the video position: playing=%t phase=%q", f.state.IsPlaying, phase)
	}
	if len(f.starts) != 3 {
		t.Fatalf("starts = %v", f.starts)
	}
}
