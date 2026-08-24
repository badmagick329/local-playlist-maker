package history

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
