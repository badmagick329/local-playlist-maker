package tracksession

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"playlistmaker/charm/internal/tracking"
	"testing"
	"time"
)

func TestPausedQueueDrainsOnlyEarlierCompletedOccurrences(t *testing.T) {
	ctx := context.Background()
	p := &controlledSpotify{}
	q := playQueue{runtime: &Runtime{Spotify: p, spotifyAvailable: true}}
	track := tracking.Track{SpotifyURI: "spotify:track:fixture"}
	q.load(ctx, "earlier", 0, track)
	q.end(ctx, "eof")
	q.load(ctx, "also-completed", 1, track)
	q.end(ctx, "eof")
	q.load(ctx, "current", 2, track)
	zero := 0
	if !q.intent(Event{OccurrenceID: "current", IntentSequence: 1, Paused: true, PositionMS: &zero}) {
		t.Fatal("intent rejected")
	}
	p.finished = true
	if err := q.tick(ctx); err != nil {
		t.Fatal(err)
	}
	if q.active.id != "also-completed" {
		t.Fatal("earlier completed occurrence not drained")
	}
	if err := q.tick(ctx); err != nil {
		t.Fatal(err)
	}
	if q.active != nil || len(p.Started) != 2 || len(q.pending) != 1 {
		t.Fatal("zero ceiling started current occurrence")
	}
	q.target = nil
	if err := q.tick(ctx); err != nil {
		t.Fatal(err)
	}
	if len(p.Started) != 2 {
		t.Fatal("unknown ceiling started playback")
	}
	q.intent(Event{OccurrenceID: "current", IntentSequence: 2, Paused: false})
	if err := q.tick(ctx); err != nil {
		t.Fatal(err)
	}
	if len(p.Started) != 3 {
		t.Fatal("resume lost queued occurrence")
	}
	if err := q.tick(ctx); err != nil {
		t.Fatal(err)
	}
	q.intent(Event{OccurrenceID: "current", IntentSequence: 3, Paused: true})
	q.intent(Event{OccurrenceID: "current", IntentSequence: 4, Paused: false})
	if err := q.tick(ctx); err != nil {
		t.Fatal(err)
	}
	if len(p.Started) != 3 {
		t.Fatal("completed occurrence replayed")
	}
}

type retrySpotify struct {
	controlledSpotify
	retries int
	failed  bool
}

func (p *retrySpotify) Intent(bool, *int) {}
func (p *retrySpotify) TrackingStatus() (string, string) {
	if p.failed {
		return "blocked", p.statusErr.Error()
	}
	return "playing", "Verified"
}
func (p *retrySpotify) Retry() { p.retries++; p.statusErr = nil; p.failed = false }
func (p *retrySpotify) Finished(context.Context) (bool, error) {
	p.failed = p.statusErr != nil
	return false, p.statusErr
}

func TestRunnerRetryPreservesEarlierOccurrenceAndLatestPause(t *testing.T) {
	path, m, err := Create(t.TempDir(), []Entry{{Track: tracking.Track{SpotifyURI: "spotify:track:first"}}, {Track: tracking.Track{SpotifyURI: "spotify:track:second"}}}, false, false, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	m.MPVProcessID = 123
	if err = WriteManifest(path, m); err != nil {
		t.Fatal(err)
	}
	appendEvents := func(events ...Event) {
		t.Helper()
		f, e := os.OpenFile(m.EventPath, os.O_APPEND|os.O_WRONLY, 0600)
		if e != nil {
			t.Fatal(e)
		}
		defer f.Close()
		for _, event := range events {
			event.SessionID = m.SessionID
			if e = json.NewEncoder(f).Encode(event); e != nil {
				t.Fatal(e)
			}
		}
	}
	appendEvents(Event{EventID: "first", Event: "file-loaded", PlaylistPosition: 0}, Event{EventID: "end", Event: "end-file", Completed: true}, Event{EventID: "second", Event: "file-loaded", PlaylistPosition: 1})
	p := &retrySpotify{controlledSpotify: controlledSpotify{statusErr: &tracking.PlaybackFailure{Message: "test interruption"}}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- (Runner{Runtime: &Runtime{Spotify: p}, IsAlive: func(int) bool { return true }, Poll: time.Millisecond}).Run(ctx, path)
	}()
	waitStatus := func(state string) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			select {
			case e := <-done:
				t.Fatalf("helper exited: %v", e)
			default:
			}
			data, _ := os.ReadFile(m.StatusPath)
			var s Status
			if json.Unmarshal(data, &s) == nil && s.State == state {
				return
			}
			time.Sleep(time.Millisecond)
		}
		data, _ := os.ReadFile(m.StatusPath)
		t.Fatalf("never reached %s: %s", state, data)
	}
	waitStatus("blocked")
	target := 90000
	appendEvents(Event{EventID: "resume", Event: "intent", OccurrenceID: "second", IntentSequence: 1}, Event{EventID: "pause", Event: "intent", OccurrenceID: "second", IntentSequence: 2, Paused: true, PositionMS: &target})
	waitStatus("user-paused")
	cancel()
	<-done
	if len(p.Started) != 1 || p.retries != 1 {
		t.Fatalf("retry restarted or lost occurrence: starts=%d retries=%d", len(p.Started), p.retries)
	}
	data, err := os.ReadFile(m.CheckpointPath)
	if err != nil {
		t.Fatal(err)
	}
	var c checkpoint
	if err = json.Unmarshal(data, &c); err != nil {
		t.Fatal(err)
	}
	if c.Active != "first" || len(c.Pending) != 1 || c.Pending[0] != "second" || !c.Paused {
		t.Fatalf("lost queue/intent: %+v", c)
	}
}

func TestLatestIntentRejectsEarlierOccurrenceAndSequence(t *testing.T) {
	q := playQueue{video: &queuedPlay{id: "second"}}
	target := 90000
	q.intent(Event{OccurrenceID: "second", IntentSequence: 5, Paused: true, PositionMS: &target})
	for _, e := range []Event{{OccurrenceID: "first", IntentSequence: 6}, {OccurrenceID: "second", IntentSequence: 4}, {OccurrenceID: "second", IntentSequence: 5}} {
		if q.intent(e) {
			t.Fatal("stale event accepted")
		}
	}
	if !q.paused || *q.target != 90000 {
		t.Fatal("stale intent changed ceiling")
	}
	q.intent(Event{OccurrenceID: "second", IntentSequence: 6, Paused: false})
	if q.paused {
		t.Fatal("latest intent ignored")
	}
}

func TestCheckpointRetainsCompletedVideoAndQueueWithoutReplaying(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "checkpoint.json")
	track := tracking.Track{SpotifyURI: "spotify:track:fixture"}
	first := &queuedPlay{id: "first", position: 0, completed: true, track: track, videoStartedAt: time.Now()}
	second := &queuedPlay{id: "second", position: 1, track: track}
	q := playQueue{runtime: &Runtime{}, active: first, video: second, pending: []*queuedPlay{second}, paused: true, intentSequence: 4}
	if err := q.save(path, 500, false); err != nil {
		t.Fatal(err)
	}
	restored := playQueue{runtime: &Runtime{}}
	offset, ended, err := restored.restore(path, Manifest{Entries: []Entry{{Track: track}, {Track: track}}})
	if err != nil || offset != 500 || ended {
		t.Fatalf("restore=%d %t %v", offset, ended, err)
	}
	if restored.active.id != "first" || !restored.active.completed || restored.video != restored.pending[0] || !restored.paused || restored.blocked == "" {
		t.Fatal("lost occurrence state or allowed ambiguous replay")
	}
}
