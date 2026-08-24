package tracksession

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"playlistmaker/charm/internal/history"
	"playlistmaker/charm/internal/tracking"
)

func recoverySession(t *testing.T, historyEnabled bool) (string, Manifest) {
	t.Helper()
	directory := t.TempDir()
	historyPath := filepath.Join(directory, history.HistoryFileName)
	entries := []Entry{
		{VideoPath: "one.mkv", Track: tracking.Track{TrackID: "track-one", Artist: "Artist", Title: "One"}},
		{VideoPath: "two.mkv", Track: tracking.Track{TrackID: "track-two", Artist: "Artist", Title: "Two"}},
	}
	path, manifest, err := Create(directory, entries, false, historyEnabled, historyPath, 50)
	if err != nil {
		t.Fatal(err)
	}
	manifest.HelperProcessID, manifest.MPVProcessID = 11, 22
	if err := WriteManifest(path, manifest); err != nil {
		t.Fatal(err)
	}
	return path, manifest
}

func appendSessionEvents(t *testing.T, manifest Manifest, lines string) {
	t.Helper()
	if err := os.WriteFile(manifest.EventPath, []byte(lines), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestNormalShutdownCleansManifestWithoutRecoveryHistory(t *testing.T) {
	path, manifest := recoverySession(t, true)
	appendSessionEvents(t, manifest, `{"eventId":"1","event":"shutdown"}`+"\n")
	if err := RecoverStale(context.Background(), filepath.Dir(filepath.Dir(path)), func(int) bool { return false }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("completed manifest remained")
	}
	if _, err := os.Stat(manifest.HistoryPath); !os.IsNotExist(err) {
		t.Fatal("normal shutdown wrote recovery history")
	}
}

func TestMPVCrashRecoversLoadedAndNotStartedWithoutDuplicates(t *testing.T) {
	path, manifest := recoverySession(t, true)
	appendSessionEvents(t, manifest, `{"eventId":"1","event":"file-loaded","playlistPosition":0}`+"\n"+`{"eventId":"1","event":"file-loaded","playlistPosition":0}`+"\n")
	if err := RecoverStale(context.Background(), filepath.Dir(filepath.Dir(path)), func(int) bool { return false }); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(manifest.HistoryPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(contents), `"event":"abandoned"`) != 1 || strings.Count(string(contents), `"event":"not_started"`) != 1 {
		t.Fatalf("history = %s", contents)
	}
}

func TestHelperCrashStartupRecoverySkipsLiveSession(t *testing.T) {
	path, manifest := recoverySession(t, true)
	if err := RecoverStale(context.Background(), filepath.Dir(filepath.Dir(path)), func(pid int) bool { return pid == manifest.MPVProcessID }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("live session was touched")
	}
	if err := RecoverStale(context.Background(), filepath.Dir(filepath.Dir(path)), func(int) bool { return false }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("stale helper session remained")
	}
}

func TestHistoryDisabledCrashWritesNothing(t *testing.T) {
	path, manifest := recoverySession(t, false)
	appendSessionEvents(t, manifest, `{"eventId":"1","event":"file-loaded","playlistPosition":0}`+"\n")
	if err := RecoverStale(context.Background(), filepath.Dir(filepath.Dir(path)), func(int) bool { return false }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(manifest.HistoryPath); !os.IsNotExist(err) {
		t.Fatal("history-disabled recovery wrote history")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("history-disabled manifest remained")
	}
}

func TestRecoveryDoesNotDuplicateExistingTerminalEvent(t *testing.T) {
	path, manifest := recoverySession(t, true)
	counted := false
	if err := history.Append(manifest.HistoryPath, history.Event{Event: "completed", SessionID: manifest.SessionID, EntryID: manifest.Entries[0].EntryID, CountedAsPlayed: &counted}); err != nil {
		t.Fatal(err)
	}
	if err := RecoverStale(context.Background(), filepath.Dir(filepath.Dir(path)), func(int) bool { return false }); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(manifest.HistoryPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(contents), manifest.Entries[0].EntryID) != 1 {
		t.Fatalf("duplicate terminal history = %s", contents)
	}
}

func TestRecoveryFailureKeepsManifest(t *testing.T) {
	path, manifest := recoverySession(t, true)
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	manifest.HistoryPath = filepath.Join(blocker, "history.jsonl")
	if err := WriteManifest(path, manifest); err != nil {
		t.Fatal(err)
	}
	if err := RecoverStale(context.Background(), filepath.Dir(filepath.Dir(path)), func(int) bool { return false }); err == nil {
		t.Fatal("recovery failure was ignored")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("failed recovery removed its manifest")
	}
}
