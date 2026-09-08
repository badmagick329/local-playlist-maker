package tracksession

import (
	"context"
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

func TestRunnerTerminatesMPVAfterDisallowedUntrackedFallback(t *testing.T) {
	directory := t.TempDir()
	track := tracking.Track{TrackID: "one", Artist: "Artist", Title: "Title", SpotifyURI: "spotify:track:one", LocalAudioPath: "one.flac"}
	manifestPath, manifest, err := Create(directory, []Entry{{VideoPath: "one.mkv", Track: track}}, false, false, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	manifest.MPVProcessID = 123
	if err := WriteManifest(manifestPath, manifest); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest.EventPath, []byte(`{"eventId":"loaded","event":"file-loaded","playlistPosition":0}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	spotify := &fakeSpotify{Fake: tracking.Fake{Err: errors.New("Spotify failed")}}
	local := &tracking.Fake{Err: errors.New("foobar failed")}
	terminated := 0
	runner := Runner{Runtime: &Runtime{Spotify: spotify, Local: local}, Poll: time.Millisecond, IsAlive: func(int) bool { return true }, Terminate: func(pid int) error { terminated = pid; return nil }}
	err = runner.Run(context.Background(), manifestPath)
	if err == nil || terminated != 123 {
		t.Fatalf("run error = %v, terminated pid = %d", err, terminated)
	}
	if spotify.Closed != 1 {
		t.Fatalf("Spotify close calls = %d", spotify.Closed)
	}
	if _, statErr := os.Stat(manifestPath); statErr != nil {
		t.Fatal("failed session manifest was removed")
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

func TestRunnerRequestsMPVPauseAndStopsFailedTrackingSession(t *testing.T) {
	for _, scenario := range []string{"status failure", "later load failure", "deferred start failure"} {
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
			terminated := 0
			runner := Runner{Runtime: &Runtime{Spotify: player}, Poll: time.Millisecond, IsAlive: func(int) bool { return true }, Terminate: func(pid int) error { terminated = pid; return nil }}
			err = runner.Run(context.Background(), manifestPath)
			if scenario == "status failure" && !errors.Is(err, failure) || scenario != "status failure" && (err == nil || !strings.Contains(err.Error(), "later Spotify start failed")) {
				t.Fatalf("run error = %v", err)
			}
			if later != nil && (len(later.attempts) != 2 || later.attempts[1] != "spotify:track:advice") {
				t.Fatalf("unexpected start attempts: %v", later.attempts)
			}
			if terminated != 0 {
				t.Fatalf("mpv was terminated after tracking startup: %d", terminated)
			}
			pause, pauseErr := os.ReadFile(manifest.PausePath)
			if pauseErr != nil || !strings.Contains(string(pause), err.Error()) {
				t.Fatalf("mpv pause was not requested: %q, %v", pause, pauseErr)
			}
			if len(spotify.Started) != 1 || spotify.Started[0].SpotifyURI != "spotify:track:bingle" {
				t.Fatalf("queued play started after failure: %#v", spotify.Started)
			}
			if spotify.Closed != 1 {
				t.Fatalf("tracking runtime close calls = %d", spotify.Closed)
			}
			if _, err := os.Stat(manifest.LockPath); !os.IsNotExist(err) {
				t.Fatalf("tracking ownership retained: %v", err)
			}
			contents, err := os.ReadFile(filepath.Join(directory, "tracking-error.txt"))
			if err != nil || !strings.Contains(string(contents), "Tracking stopped:") {
				t.Fatalf("tracking failure not exposed: %q, %v", contents, err)
			}
		})
	}
}
