package tracksession

import (
	"context"
	"errors"
	"os"
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
