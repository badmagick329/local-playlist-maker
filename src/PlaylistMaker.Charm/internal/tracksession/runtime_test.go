package tracksession

import (
	"context"
	"encoding/json"
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
	if err := runtime.Load(context.Background(), 0, entries[0].Track); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Load(context.Background(), 1, entries[1].Track); err != nil {
		t.Fatal(err)
	}
	if len(spotify.Started) != 1 || len(local.Started) != 2 {
		t.Fatalf("Spotify starts = %d, local fallbacks = %d", len(spotify.Started), len(local.Started))
	}
	contents, _ := os.ReadFile(runtime.DiagnosticsPath)
	if !strings.Contains(string(contents), "Spotify: network") || !strings.Contains(string(contents), "foobar") {
		t.Fatalf("diagnostics = %s", contents)
	}
	later := diagnosticAt(t, runtime.DiagnosticsPath, 1, "foobar")
	if !strings.Contains(later.FallbackReason, "network") {
		t.Fatalf("later foobar diagnostic = %#v", later)
	}
}

func TestLaterSpotifyOnlyTrackReturnsOriginalRuntimeFailure(t *testing.T) {
	spotify := &fakeSpotify{Fake: tracking.Fake{Err: errors.New("first Spotify start failed")}}
	local := &tracking.Fake{}
	runtime := &Runtime{Spotify: spotify, Local: local, DiagnosticsPath: filepath.Join(t.TempDir(), "diagnostics.jsonl")}
	first := tracking.Track{TrackID: "one", SpotifyURI: "spotify:track:one", LocalAudioPath: "one.flac"}
	later := tracking.Track{TrackID: "two", Artist: "Artist", Title: "Two", SpotifyURI: "spotify:track:two"}
	if err := runtime.Prepare(context.Background(), "device", []Entry{{Track: first}, {Track: later}}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Load(context.Background(), 0, first); err != nil {
		t.Fatal(err)
	}
	err := runtime.Load(context.Background(), 1, later)
	if err == nil || !strings.Contains(err.Error(), "first Spotify start failed") {
		t.Fatalf("later load error = %v", err)
	}
	if len(spotify.Started) != 1 {
		t.Fatalf("Spotify starts = %d", len(spotify.Started))
	}
	final := diagnosticAt(t, runtime.DiagnosticsPath, 1, "untracked")
	if !strings.Contains(final.FallbackReason, "first Spotify start failed") {
		t.Fatalf("later diagnostic = %#v", final)
	}
}

func TestLaterSpotifyOnlyTrackUsesNoopWithOriginalRuntimeFailure(t *testing.T) {
	spotify := &fakeSpotify{Fake: tracking.Fake{Err: errors.New("first Spotify start failed")}}
	local := &tracking.Fake{}
	runtime := &Runtime{Spotify: spotify, Local: local, AllowUntracked: true, DiagnosticsPath: filepath.Join(t.TempDir(), "diagnostics.jsonl")}
	first := tracking.Track{TrackID: "one", SpotifyURI: "spotify:track:one", LocalAudioPath: "one.flac"}
	later := tracking.Track{TrackID: "two", SpotifyURI: "spotify:track:two"}
	if err := runtime.Prepare(context.Background(), "device", []Entry{{Track: first}, {Track: later}}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Load(context.Background(), 0, first); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Load(context.Background(), 1, later); err != nil {
		t.Fatal(err)
	}
	if runtime.activeProvider != "untracked" || len(spotify.Started) != 1 {
		t.Fatalf("provider = %q, Spotify starts = %d", runtime.activeProvider, len(spotify.Started))
	}
	final := diagnosticAt(t, runtime.DiagnosticsPath, 1, "untracked")
	if !strings.Contains(final.FallbackReason, "first Spotify start failed") {
		t.Fatalf("later diagnostic = %#v", final)
	}
}

func TestPrepareRejectsSpotifyOnlyTrackWhenPreflightFails(t *testing.T) {
	preflightErr := errors.New("missing device")
	runtime := &Runtime{Spotify: &fakeSpotify{preflight: preflightErr}, Local: &tracking.Fake{}}
	err := runtime.Prepare(context.Background(), "device", []Entry{{Track: tracking.Track{TrackID: "one", Artist: "Artist", Title: "Title", SpotifyURI: "spotify:track:one"}}})
	if !errors.Is(err, preflightErr) {
		t.Fatalf("preflight error = %v", err)
	}
}

func TestRuntimeRejectsProviderFailureWhenUntrackedIsDisabled(t *testing.T) {
	spotify := &fakeSpotify{Fake: tracking.Fake{Err: errors.New("Spotify failed")}}
	local := &tracking.Fake{Err: errors.New("foobar failed")}
	runtime := &Runtime{Spotify: spotify, Local: local, DiagnosticsPath: filepath.Join(t.TempDir(), "diagnostics.jsonl")}
	track := tracking.Track{TrackID: "one", Artist: "Artist", Title: "Title", SpotifyURI: "spotify:track:one", LocalAudioPath: "one.flac"}
	if err := runtime.Prepare(context.Background(), "device", []Entry{{Track: track}}); err != nil {
		t.Fatal(err)
	}
	err := runtime.Load(context.Background(), 0, track)
	if err == nil || !strings.Contains(err.Error(), "Spotify failed") || !strings.Contains(err.Error(), "foobar failed") {
		t.Fatalf("load error = %v", err)
	}
	if runtime.active != nil {
		t.Fatal("disallowed fallback selected an active provider")
	}
	contents, _ := os.ReadFile(runtime.DiagnosticsPath)
	if !strings.Contains(string(contents), "untracked playback is disabled") {
		t.Fatalf("diagnostics = %s", contents)
	}
}

func TestRuntimeUsesNoopOnlyWhenUntrackedIsEnabled(t *testing.T) {
	spotify := &fakeSpotify{Fake: tracking.Fake{Err: errors.New("Spotify failed")}}
	local := &tracking.Fake{Err: errors.New("foobar failed")}
	runtime := &Runtime{Spotify: spotify, Local: local, AllowUntracked: true}
	track := tracking.Track{TrackID: "one", SpotifyURI: "spotify:track:one", LocalAudioPath: "one.flac"}
	if err := runtime.Prepare(context.Background(), "device", []Entry{{Track: track}}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Load(context.Background(), 0, track); err != nil {
		t.Fatal(err)
	}
	if runtime.activeProvider != "untracked" {
		t.Fatalf("active provider = %q", runtime.activeProvider)
	}
}

func TestLocalFallbackUsesSpotifyPreflightError(t *testing.T) {
	local := &tracking.Fake{}
	runtime := &Runtime{Spotify: &fakeSpotify{preflight: errors.New("configured device missing")}, Local: local, DiagnosticsPath: filepath.Join(t.TempDir(), "diagnostics.jsonl")}
	track := tracking.Track{TrackID: "one", SpotifyURI: "spotify:track:one", LocalAudioPath: "one.flac"}
	if err := runtime.Prepare(context.Background(), "device", []Entry{{Track: track}}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Load(context.Background(), 0, track); err != nil {
		t.Fatal(err)
	}
	if len(local.Started) != 1 {
		t.Fatalf("local starts = %d", len(local.Started))
	}
	contents, _ := os.ReadFile(runtime.DiagnosticsPath)
	if !strings.Contains(string(contents), "configured device missing") {
		t.Fatalf("diagnostics = %s", contents)
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

func diagnosticAt(t *testing.T, path string, position int, provider string) Diagnostic {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(contents)), "\n") {
		var diagnostic Diagnostic
		if json.Unmarshal([]byte(line), &diagnostic) == nil && diagnostic.PlaylistPosition == position && diagnostic.Provider == provider {
			return diagnostic
		}
	}
	t.Fatalf("diagnostic position %d provider %q not found in %s", position, provider, contents)
	return Diagnostic{}
}
