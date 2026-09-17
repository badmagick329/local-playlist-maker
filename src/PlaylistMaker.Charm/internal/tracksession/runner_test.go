package tracksession

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"playlistmaker/charm/internal/tracking"
)

func TestRunnerRestartsRepeatedTrackAndDeduplicatesEvents(t *testing.T) {
	manifestPath, manifest, err := Create(t.TempDir(), []Entry{{VideoPath: "one.mkv", Track: tracking.Track{TrackID: "one", SpotifyURI: "spotify:track:one"}}}, false, false, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	manifest.MPVProcessID = 123
	if err := WriteManifest(manifestPath, manifest); err != nil {
		t.Fatal(err)
	}
	events := strings.Join([]string{
		`{"eventId":"1","event":"file-loaded","playlistPosition":0}`,
		`{"eventId":"2","event":"playback-repeat","playlistPosition":0}`,
		`{"eventId":"2","event":"playback-repeat","playlistPosition":0}`,
		`{"eventId":"3","event":"end-file","playlistPosition":0,"endReason":"eof"}`,
		`{"eventId":"4","event":"file-loaded","playlistPosition":0}`,
		`{"eventId":"5","event":"end-file","playlistPosition":0,"endReason":"eof"}`,
		`{"eventId":"6","event":"shutdown"}`,
	}, "\n") + "\n"
	if err := os.WriteFile(manifest.EventPath, []byte(events), 0o600); err != nil {
		t.Fatal(err)
	}
	spotify := &fakeSpotify{}
	runner := Runner{Runtime: &Runtime{Spotify: spotify}, Poll: time.Millisecond, IsAlive: func(int) bool { return true }}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := runner.Run(ctx, manifestPath); err != nil {
		t.Fatal(err)
	}
	if len(spotify.Started) != 3 {
		t.Fatalf("Spotify starts = %d, want 3", len(spotify.Started))
	}
}

type laterStartFailureSpotify struct {
	controlledSpotify
	attempts []string
}

func (s *laterStartFailureSpotify) Start(ctx context.Context, track tracking.Track) error {
	s.attempts = append(s.attempts, track.SpotifyURI)
	if len(s.attempts) > 1 {
		return errors.New("later Spotify start failed")
	}
	return s.controlledSpotify.Start(ctx, track)
}

func TestRunnerPreservesQueueAndPersistentHoldAfterFailure(t *testing.T) {
	for _, scenario := range []string{"status failure", "deferred start failure"} {
		t.Run(scenario, func(t *testing.T) {
			directory := t.TempDir()
			entries := []Entry{
				{VideoPath: "bingle-bangle.mkv", Track: tracking.Track{TrackID: "bingle", SpotifyURI: "spotify:track:bingle"}},
				{VideoPath: "advice.mkv", Track: tracking.Track{TrackID: "advice", SpotifyURI: "spotify:track:advice"}},
				{VideoPath: "third.mkv", Track: tracking.Track{TrackID: "third", SpotifyURI: "spotify:track:third"}},
			}
			manifestPath, manifest, err := Create(directory, entries, false, false, "", 50)
			if err != nil {
				t.Fatal(err)
			}
			manifest.MPVProcessID = 123
			if err := WriteManifest(manifestPath, manifest); err != nil {
				t.Fatal(err)
			}
			events := strings.Join([]string{
				`{"eventId":"1","event":"file-loaded","playlistPosition":0}`,
				`{"eventId":"2","event":"end-file","playlistPosition":0,"endReason":"eof","completed":true}`,
				`{"eventId":"3","event":"file-loaded","playlistPosition":1}`,
			}, "\n") + "\n"
			if scenario == "later load failure" {
				events = strings.Replace(events, `"endReason":"eof","completed":true`, `"endReason":"stop"`, 1)
			}
			events += `{"eventId":"4","event":"end-file","playlistPosition":1,"endReason":"eof","completed":true}` + "\n" +
				`{"eventId":"5","event":"file-loaded","playlistPosition":2}` + "\n"
			if err := os.WriteFile(manifest.EventPath, []byte(events), 0o600); err != nil {
				t.Fatal(err)
			}
			failure := &tracking.PlaybackFailure{Message: "Spotify tracking timed out at 56797ms"}
			spotify := &controlledSpotify{statusErr: failure}
			var player SpotifyPlayer = spotify
			var later *laterStartFailureSpotify
			if scenario != "status failure" {
				later = &laterStartFailureSpotify{controlledSpotify: controlledSpotify{finished: true}}
				spotify = &later.controlledSpotify
				player = later
			}
			runner := Runner{Runtime: &Runtime{Spotify: player}, Poll: time.Millisecond, IsAlive: func(int) bool { return true }}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			result := make(chan error, 1)
			go func() { result <- runner.Run(ctx, manifestPath) }()
			for ctx.Err() == nil {
				data, _ := os.ReadFile(manifest.StatusPath)
				var status Status
				if json.Unmarshal(data, &status) == nil && status.State == "blocked" {
					cancel()
					break
				}
				time.Sleep(time.Millisecond)
			}
			err = <-result
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("helper did not stay recoverable: %v", err)
			}

			if later != nil && (len(later.attempts) != 2 || later.attempts[1] != "spotify:track:advice") {
				t.Fatalf("unexpected start attempts: %v", later.attempts)
			}
			data, readErr := os.ReadFile(manifest.StatusPath)
			var status Status
			if readErr != nil || json.Unmarshal(data, &status) != nil || !status.Hold || status.State != "blocked" {
				t.Fatalf("persistent hold missing: %s %v", data, readErr)
			}
			if _, e := os.Stat(manifest.CheckpointPath); e != nil {
				t.Fatal("queue checkpoint missing", e)
			}

			if len(spotify.Started) != 1 || spotify.Started[0].SpotifyURI != "spotify:track:bingle" {
				t.Fatalf("queued play started after failure: %#v", spotify.Started)
			}
			if spotify.Closed != 0 {
				t.Fatalf("tracking runtime close calls = %d", spotify.Closed)
			}
			if _, err := os.Stat(manifest.LockPath); !os.IsNotExist(err) {
				t.Fatalf("tracking ownership retained: %v", err)
			}
			contents, err := os.ReadFile(filepath.Join(directory, "tracking-error.txt"))
			if err != nil || len(contents) == 0 {
				t.Fatalf("tracking failure not exposed: %q, %v", contents, err)
			}
		})
	}
}
