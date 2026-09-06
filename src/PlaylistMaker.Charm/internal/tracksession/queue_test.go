package tracksession

import (
	"context"
	"errors"
	"testing"
	"time"

	"playlistmaker/charm/internal/tracking"
)

type controlledSpotify struct {
	fakeSpotify
	finished  bool
	statusErr error
}

func (p *controlledSpotify) Finished(context.Context) (bool, error) { return p.finished, p.statusErr }

func TestQueueKeepsPlayOnStatusFailureAndResumesAfterRecovery(t *testing.T) {
	ctx := context.Background()
	spotify := &controlledSpotify{statusErr: errors.New("Spotify start is delayed")}
	runtime := &Runtime{Spotify: spotify}
	track := tracking.Track{SpotifyURI: "spotify:track:song"}
	if err := runtime.Prepare(ctx, "device", []Entry{{Track: track}}); err != nil {
		t.Fatal(err)
	}
	queue := playQueue{runtime: runtime}
	if err := queue.load(ctx, "first", 0, track); err != nil {
		t.Fatal(err)
	}
	queue.end(ctx, "eof")
	if err := queue.load(ctx, "second", 0, track); err != nil {
		t.Fatal(err)
	}
	if err := queue.tick(ctx); err != nil {
		t.Fatalf("status failure escaped to video termination: %v", err)
	}
	if spotify.Stops != 0 || len(spotify.Started) != 1 || queue.active.id != "first" {
		t.Fatal("unconfirmed play was interrupted or lost")
	}
	spotify.statusErr, spotify.finished = nil, true
	if err := queue.tick(ctx); err != nil {
		t.Fatal(err)
	}
	if len(spotify.Started) != 2 || queue.active.id != "second" {
		t.Fatal("queue did not recover")
	}
}

func TestQueueDoesNotImmediatelyStopDeferredLocalFallback(t *testing.T) {
	ctx := context.Background()
	spotify := &controlledSpotify{}
	local := &tracking.Fake{}
	runtime := &Runtime{Spotify: spotify, Local: local}
	first := tracking.Track{SpotifyURI: "spotify:track:first"}
	second := tracking.Track{LocalAudioPath: "second.flac"}
	if err := runtime.Prepare(ctx, "device", []Entry{{Track: first}, {Track: second}}); err != nil {
		t.Fatal(err)
	}
	queue := playQueue{runtime: runtime}
	if err := queue.load(ctx, "first", 0, first); err != nil {
		t.Fatal(err)
	}
	queue.end(ctx, "eof")
	if err := queue.load(ctx, "second", 1, second); err != nil {
		t.Fatal(err)
	}
	queue.video.videoStartedAt = time.Now().Add(-time.Minute)
	queue.end(ctx, "eof")
	spotify.finished = true
	if err := queue.tick(ctx); err != nil {
		t.Fatal(err)
	}
	if err := queue.tick(ctx); err != nil {
		t.Fatal(err)
	}
	if len(local.Started) != 1 || local.Stops != 0 {
		t.Fatal("deferred local fallback was cut off")
	}
	queue.active.trackingStartedAt = time.Now().Add(-2 * time.Minute)
	if err := queue.tick(ctx); err != nil {
		t.Fatal(err)
	}
	if local.Stops != 1 || !queue.idle() {
		t.Fatal("local fallback did not finish")
	}
}

func TestQueueSkipRemovesOnlyMatchingRepeatedPlay(t *testing.T) {
	ctx := context.Background()
	spotify := &controlledSpotify{}
	runtime := &Runtime{Spotify: spotify}
	track := tracking.Track{TrackID: "song", SpotifyURI: "spotify:track:song"}
	if err := runtime.Prepare(ctx, "device", []Entry{{Track: track}}); err != nil {
		t.Fatal(err)
	}
	queue := playQueue{runtime: runtime}
	for _, id := range []string{"first", "second", "third"} {
		if err := queue.load(ctx, id, 0, track); err != nil {
			t.Fatal(err)
		}
		if id != "third" {
			queue.end(ctx, "eof")
		}
	}
	queue.end(ctx, "stop")
	if len(spotify.Started) != 1 || spotify.Stops != 0 {
		t.Fatal("later videos interrupted the first song")
	}
	if len(queue.pending) != 1 || queue.pending[0].id != "second" {
		t.Fatalf("wrong repeat removed: %#v", queue.pending)
	}
	spotify.finished = true
	if err := queue.tick(ctx); err != nil {
		t.Fatal(err)
	}
	if len(spotify.Started) != 2 || queue.active.id != "second" {
		t.Fatal("second play was lost")
	}
	if err := queue.tick(ctx); err != nil {
		t.Fatal(err)
	}
	if !queue.idle() || len(spotify.Started) != 2 {
		t.Fatal("skipped third play was started")
	}
}

func TestQueueSkipStopsCurrentSpotifyPlay(t *testing.T) {
	ctx := context.Background()
	spotify := &controlledSpotify{}
	runtime := &Runtime{Spotify: spotify}
	track := tracking.Track{SpotifyURI: "spotify:track:song"}
	if err := runtime.Prepare(ctx, "device", []Entry{{Track: track}}); err != nil {
		t.Fatal(err)
	}
	queue := playQueue{runtime: runtime}
	if err := queue.load(ctx, "play", 0, track); err != nil {
		t.Fatal(err)
	}
	queue.end(ctx, "stop")
	if spotify.Stops != 1 || !queue.idle() {
		t.Fatal("skipped active play kept running")
	}
}

func TestQueueAdvancesAfterSpotifyFinishesShortVideoTail(t *testing.T) {
	ctx := context.Background()
	spotify := &controlledSpotify{}
	runtime := &Runtime{Spotify: spotify}
	first := tracking.Track{SpotifyURI: "spotify:track:first"}
	second := tracking.Track{SpotifyURI: "spotify:track:second"}
	if err := runtime.Prepare(ctx, "device", []Entry{{Track: first}, {Track: second}}); err != nil {
		t.Fatal(err)
	}
	queue := playQueue{runtime: runtime}
	if err := queue.load(ctx, "first", 0, first); err != nil {
		t.Fatal(err)
	}
	queue.end(ctx, "eof")
	if err := queue.load(ctx, "second", 1, second); err != nil {
		t.Fatal(err)
	}
	if err := queue.tick(ctx); err != nil {
		t.Fatal(err)
	}
	if len(spotify.Started) != 1 || spotify.Stops != 0 {
		t.Fatal("short video cut off its Spotify song")
	}
	spotify.finished = true
	if err := queue.tick(ctx); err != nil {
		t.Fatal(err)
	}
	if len(spotify.Started) != 2 || spotify.Started[1].SpotifyURI != second.SpotifyURI {
		t.Fatal("next song was not started after completion")
	}
	queue.end(ctx, "stop")
	if spotify.Stops != 2 || !queue.idle() {
		t.Fatal("skip after catching up stopped the wrong play")
	}
}
