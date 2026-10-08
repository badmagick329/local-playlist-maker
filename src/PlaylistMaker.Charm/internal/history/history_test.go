package history

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReadCountsSeparatePlaythroughsOfSameEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), HistoryFileName)
	for _, playID := range []string{"first", "second", "second"} {
		if err := Append(path, Event{Event: "completed", SessionID: "session", EntryID: "entry", PlayID: playID, TrackID: "track", EndReason: "eof"}); err != nil {
			t.Fatal(err)
		}
	}
	index, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := index.Tracks["track"].Played; got != 2 {
		t.Fatalf("plays = %d, want 2", got)
	}
}

func TestReadNormalizesLatestTerminalEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), HistoryFileName)
	contents := strings.Join([]string{
		`not json`,
		`{"event":"started","sessionId":"s","entryId":"e"}`,
		`{"event":"stopped","eventAtUtc":"2026-01-01T00:00:00Z","sessionId":"s","entryId":"e","trackId":"trk_one","audioPath":"C:\\Music\\One.flac","videoPath":"C:\\Video\\One.mkv","watchedPercent":95}`,
		`{"event":"skipped","eventAtUtc":"2025-01-01T00:00:00Z","sessionId":"s","entryId":"e","trackId":"trk_one","audioPath":"C:\\Music\\One.flac","videoPath":"C:\\Video\\One.mkv"}`,
		`{"event":"stopped","eventAtUtc":"2026-01-02T00:00:00Z","sessionId":"s","entryId":"two","trackId":"trk_one","audioPath":"c:/music/one.flac","videoPath":"c:/video/two.mkv","endReason":"eof","watchedPercent":10}`,
	}, "\n")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	index, err := Read(path)
	if err != nil || index.InvalidLines != 1 {
		t.Fatalf("read = %#v, %v", index, err)
	}
	summary := index.Tracks[`trk_one`]
	if summary.Played != 2 || summary.Completed != 2 || summary.Skipped != 0 || len(summary.Recent) != 2 {
		t.Fatalf("summary = %#v", summary)
	}
	if summary.Recent[0].Percent == nil || *summary.Recent[0].Percent != 100 {
		t.Fatalf("EOF normalization = %#v", summary.Recent[0])
	}
}

func TestNormalizeClampsDurationAndPercent(t *testing.T) {
	percent, seconds := 150.0, 80.0
	duration := 60.0
	item := Normalize(Event{Event: "stopped", WatchedPercent: &percent, WatchedSeconds: &seconds, DurationSeconds: &duration})
	if item.Percent == nil || *item.Percent != 100 || item.Seconds == nil || *item.Seconds != 60 || item.Outcome != "completed" {
		t.Fatalf("normalized = %#v", item)
	}
	_ = time.Now()
}

func TestLastAttemptedIgnoresNotStarted(t *testing.T) {
	path := filepath.Join(t.TempDir(), HistoryFileName)
	contents := "{\"event\":\"not_started\",\"eventAtUtc\":\"2026-01-03T00:00:00Z\",\"sessionId\":\"s\",\"entryId\":\"one\",\"videoPath\":\"video\"}\n" +
		"{\"event\":\"skipped\",\"eventAtUtc\":\"2026-01-02T00:00:00Z\",\"sessionId\":\"s\",\"entryId\":\"two\",\"videoPath\":\"video\"}\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	index, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := index.Videos["video"].LastAttempted; got == nil || got.Day() != 2 {
		t.Fatalf("last attempted = %v", got)
	}
}

func TestReadUsesLaterCompletionForRevisitedEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), HistoryFileName)
	contents := strings.Join([]string{
		`{"event":"started","eventAtUtc":"2026-01-01T00:00:00Z","sessionId":"session","entryId":"entry","videoPath":"video"}`,
		`{"event":"skipped","eventAtUtc":"2026-01-01T00:01:00Z","sessionId":"session","entryId":"entry","videoPath":"video"}`,
		`{"event":"started","eventAtUtc":"2026-01-01T00:02:00Z","sessionId":"session","entryId":"entry","videoPath":"video"}`,
		`{"event":"completed","eventAtUtc":"2026-01-01T00:03:00Z","sessionId":"session","entryId":"entry","videoPath":"video"}`,
	}, "\n")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	index, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	summary := index.Videos["video"]
	if summary.Completed != 1 || summary.Skipped != 0 || summary.Played != 1 {
		t.Fatalf("revisited summary = %#v", summary)
	}
}

func writeSkipThenBack(t *testing.T, betweenWatched string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), HistoryFileName)
	contents := strings.Join([]string{
		`{"event":"started","eventAtUtc":"2026-01-01T00:00:00Z","sessionId":"s","entryId":"a","playId":"s:1","videoPath":"a.mkv"}`,
		`{"event":"skipped","eventAtUtc":"2026-01-01T00:01:00Z","sessionId":"s","entryId":"a","playId":"s:1","videoPath":"a.mkv","watchedSeconds":60}`,
		`{"event":"started","eventAtUtc":"2026-01-01T00:01:00Z","sessionId":"s","entryId":"b","playId":"s:4","videoPath":"b.mkv"}`,
		`{"event":"skipped","eventAtUtc":"2026-01-01T00:01:00Z","sessionId":"s","entryId":"b","playId":"s:4","videoPath":"b.mkv","watchedSeconds":` + betweenWatched + `}`,
		`{"event":"started","eventAtUtc":"2026-01-01T00:01:00Z","sessionId":"s","entryId":"a","playId":"s:7","videoPath":"a.mkv"}`,
	}, "\n")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadIgnoresSkipReversedByGoingBack(t *testing.T) {
	index, err := Read(writeSkipThenBack(t, "2"))
	if err != nil {
		t.Fatal(err)
	}
	a, b := index.Videos["a.mkv"], index.Videos["b.mkv"]
	if a.Skipped != 0 || b.Skipped != 0 {
		t.Fatalf("skips = %d, %d, want 0, 0", a.Skipped, b.Skipped)
	}
	if b.LastAttempted != nil || len(b.Recent) != 1 || b.Recent[0].Outcome != "reversed" {
		t.Fatalf("passed-over video = %#v", b)
	}
}

func TestReadKeepsSkipsWhenInBetweenVideoWasWatched(t *testing.T) {
	index, err := Read(writeSkipThenBack(t, "30"))
	if err != nil {
		t.Fatal(err)
	}
	if a, b := index.Videos["a.mkv"], index.Videos["b.mkv"]; a.Skipped != 1 || b.Skipped != 1 || b.LastAttempted == nil {
		t.Fatalf("summaries = %#v, %#v", a, b)
	}
}
