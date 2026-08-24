package migration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"playlistmaker/charm/internal/catalog"
	"playlistmaker/charm/internal/history"
	"playlistmaker/charm/internal/metadata"
)

func TestRunBacksUpFilesAndAttachesTrackIDs(t *testing.T) {
	root := t.TempDir()
	mapping := filepath.Join(root, "mapping.json")
	historyPath := filepath.Join(root, history.HistoryFileName)
	cache := filepath.Join(root, "cache.json")
	catalogue := filepath.Join(root, "catalogue.json")
	if err := os.WriteFile(mapping, []byte(`{"video.mkv":"audio.flac"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(historyPath, []byte("{\"event\":\"completed\",\"sessionId\":\"s\",\"entryId\":\"e\",\"audioPath\":\"audio.flac\"}\n{\"event\":\"completed\",\"sessionId\":\"s\",\"entryId\":\"x\",\"audioPath\":\"missing.flac\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := metadata.WriteCache(cache, map[string]metadata.Entry{"audio": {FilePath: "audio.flac", Artist: "Artist", Title: "Title", Date: "2024"}}); err != nil {
		t.Fatal(err)
	}
	report, err := Run(mapping, catalogue, historyPath, cache)
	if err != nil || report.Tracks != 1 || report.Videos != 1 || report.HistoryEventsUpdated != 1 || report.UnresolvedHistoryEntries != 1 {
		t.Fatalf("migration report = %#v, %v", report, err)
	}
	media, err := catalog.Read(catalogue)
	if err != nil || len(media.Tracks) != 1 || media.Tracks[0].ID == "" || media.Videos[0].TrackID != media.Tracks[0].ID {
		t.Fatalf("catalogue = %#v, %v", media, err)
	}
	updated, _ := os.ReadFile(historyPath)
	if !strings.Contains(string(updated), `"trackId":"`+media.Tracks[0].ID+`"`) {
		t.Fatalf("history was not attached: %s", updated)
	}
	backupBefore, _ := os.ReadFile(report.MappingBackupPath)
	if string(backupBefore) != `{"video.mkv":"audio.flac"}` {
		t.Fatalf("mapping backup changed: %s", backupBefore)
	}
	if _, err := Run(mapping, catalogue, historyPath, cache); err == nil {
		t.Fatal("second migration should not overwrite catalogue or backups")
	}
}
