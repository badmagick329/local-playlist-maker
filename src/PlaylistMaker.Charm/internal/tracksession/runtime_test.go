package tracksession

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"playlistmaker/charm/internal/tracking"
)

type fakeSpotify struct {
	tracking.Fake
	preflight error
}

func (f *fakeSpotify) Preflight(context.Context, string) error { return f.preflight }

func TestRuntimePrefersSpotifyAndDisablesItAfterFailure(t *testing.T) {
	spotify := &fakeSpotify{}
	local := &tracking.Fake{}
	runtime := &Runtime{Spotify: spotify, Local: local, AllowUntracked: true, DiagnosticsPath: filepath.Join(t.TempDir(), "diagnostics.jsonl")}
	entries := []Entry{{Track: tracking.Track{TrackID: "one", SpotifyURI: "spotify:track:one", LocalAudioPath: "one.flac"}}, {Track: tracking.Track{TrackID: "two", SpotifyURI: "spotify:track:two", LocalAudioPath: "two.flac"}}}
	if err := runtime.Prepare(context.Background(), "device", entries); err != nil {
		t.Fatal(err)
	}
	spotify.Err = errors.New("network")
	runtime.Load(context.Background(), 0, entries[0].Track)
	runtime.Load(context.Background(), 1, entries[1].Track)
	if len(spotify.Started) != 1 || len(local.Started) != 2 {
		t.Fatalf("Spotify starts = %d, local fallbacks = %d", len(spotify.Started), len(local.Started))
	}
	contents, _ := os.ReadFile(runtime.DiagnosticsPath)
	if !strings.Contains(string(contents), "spotify unavailable") || !strings.Contains(string(contents), "foobar") {
		t.Fatalf("diagnostics = %s", contents)
	}
}

func TestPrepareRejectsSpotifyOnlyTrackWhenPreflightFails(t *testing.T) {
	runtime := &Runtime{Spotify: &fakeSpotify{preflight: errors.New("missing device")}, Local: &tracking.Fake{}}
	err := runtime.Prepare(context.Background(), "device", []Entry{{Track: tracking.Track{TrackID: "one", Artist: "Artist", Title: "Title", SpotifyURI: "spotify:track:one"}}})
	if err == nil || !strings.Contains(err.Error(), "no tracking route") {
		t.Fatalf("preflight error = %v", err)
	}
}

func TestReadEventsReturnsCompleteLinesOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(path, []byte("{\"eventId\":\"one\",\"event\":\"file-loaded\"}\n{\"eventId\":\"two\""), 0o600); err != nil {
		t.Fatal(err)
	}
	events, offset, err := readEvents(path, 0)
	if err != nil || len(events) != 1 || events[0].EventID != "one" {
		t.Fatalf("events = %#v, offset %d, %v", events, offset, err)
	}
	file, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	_, _ = file.WriteString("}\n")
	_ = file.Close()
	events, _, err = readEvents(path, offset)
	if err != nil || len(events) != 1 || events[0].EventID != "two" {
		t.Fatalf("resumed events = %#v, %v", events, err)
	}
}
