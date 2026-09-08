package tracksession

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"playlistmaker/charm/internal/tracking"
)

func TestFailedDetachedSessionReleasesLockForNextSession(t *testing.T) {
	dir := t.TempDir()
	failure := &tracking.PlaybackFailure{Message: "Spotify returned an unexpected song"}
	for _, fail := range []bool{true, false} {
		path, manifest, err := Create(dir, []Entry{{VideoPath: "one.mkv", Track: tracking.Track{SpotifyURI: "spotify:track:one"}}}, false, false, "", 50)
		if err != nil {
			t.Fatal(err)
		}
		manifest.MPVProcessID = 123
		if err := WriteManifest(path, manifest); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(manifest.EventPath, []byte("{\"eventId\":\"1\",\"event\":\"file-loaded\",\"playlistPosition\":0}\n{\"eventId\":\"2\",\"event\":\"end-file\",\"playlistPosition\":0,\"completed\":true}\n{\"eventId\":\"3\",\"event\":\"shutdown\"}\n"), 0600); err != nil {
			t.Fatal(err)
		}
		p := &controlledSpotify{finished: true}
		if fail {
			p.statusErr = failure
		}
		r := Runner{Runtime: &Runtime{Spotify: p}, Poll: time.Millisecond, IsAlive: func(int) bool { return true }}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		err = r.Run(ctx, path)
		cancel()
		if fail && !errors.Is(err, failure) || !fail && err != nil {
			t.Fatalf("session result: %v", err)
		}
		if _, err := os.Stat(manifest.LockPath); !os.IsNotExist(err) {
			t.Fatalf("lock retained: %v", err)
		}
		if len(p.Started) != 1 {
			t.Fatal("session never started")
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "tracking-error.txt")); err != nil {
		t.Fatal("failure not exposed to UI", err)
	}
}

type sessionSpotify struct {
	fakeSpotify
	started  chan struct{}
	finished atomic.Bool
	stops    atomic.Int32
}

func (s *sessionSpotify) Start(context.Context, tracking.Track) error {
	s.started <- struct{}{}
	return nil
}
func (s *sessionSpotify) Stop(context.Context) error             { s.stops.Add(1); return nil }
func (s *sessionSpotify) Close(context.Context) error            { return nil }
func (s *sessionSpotify) Finished(context.Context) (bool, error) { return s.finished.Load(), nil }

func TestSeparateLaunchWaitsForPreviousCompletedQuitToFinishSpotify(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := func() (*sessionSpotify, Manifest, <-chan error) {
		t.Helper()
		path, manifest, err := Create(dir, []Entry{{VideoPath: "one.mkv", Track: tracking.Track{SpotifyURI: "spotify:track:one"}}}, false, false, "", 50)
		if err != nil {
			t.Fatal(err)
		}
		manifest.MPVProcessID = 123
		if err := WriteManifest(path, manifest); err != nil {
			t.Fatal(err)
		}
		events := "{\"eventId\":\"1\",\"event\":\"file-loaded\",\"playlistPosition\":0}\n" +
			"{\"eventId\":\"2\",\"event\":\"end-file\",\"playlistPosition\":0,\"endReason\":\"quit\",\"completed\":true}\n" +
			"{\"eventId\":\"3\",\"event\":\"shutdown\"}\n"
		if err := os.WriteFile(manifest.EventPath, []byte(events), 0600); err != nil {
			t.Fatal(err)
		}
		spotify := &sessionSpotify{started: make(chan struct{}, 1)}
		runner := Runner{Runtime: &Runtime{Spotify: spotify}, Poll: time.Millisecond, IsAlive: func(int) bool { return true }}
		done := make(chan error, 1)
		go func() { done <- runner.Run(ctx, path) }()
		return spotify, manifest, done
	}
	first, _, firstDone := start()
	select {
	case <-first.started:
	case <-ctx.Done():
		t.Fatal("first play did not start")
	}
	second, secondManifest, secondDone := start()
	for {
		if _, err := os.Stat(secondManifest.ReadyPath); err == nil {
			break
		}
		select {
		case err := <-secondDone:
			t.Fatalf("second launch rejected: %v", err)
		case <-ctx.Done():
			t.Fatal("second helper not ready")
		case <-time.After(time.Millisecond):
		}
	}
	select {
	case <-second.started:
		t.Fatal("second launch interrupted first song")
	default:
	}
	if first.stops.Load() != 0 {
		t.Fatal("near-complete quit stopped Spotify")
	}
	first.finished.Store(true)
	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("first did not drain")
	}
	select {
	case <-second.started:
	case err := <-secondDone:
		t.Fatalf("second exited before playback: %v", err)
	case <-ctx.Done():
		t.Fatal("second did not start after first finished")
	}
	second.finished.Store(true)
	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("second did not drain")
	}
}
