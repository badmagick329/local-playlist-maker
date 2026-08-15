package mapping

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadWriteAndUpsertAreCaseInsensitiveAndUnicodeSafe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "map.json")
	entries := Upsert(nil, `C:\Videos\나연.mkv`, `C:\Audio\Pop.flac`)
	entries = Upsert(entries, `c:/videos/나연.mkv`, `C:\Audio\Updated.flac`)
	if len(entries) != 1 {
		t.Fatalf("entries = %#v", entries)
	}
	if err := Write(path, entries); err != nil {
		t.Fatal(err)
	}
	loaded, err := Read(path)
	if err != nil || len(loaded) != 1 || loaded[0].AudioPath != `C:\Audio\Updated.flac` {
		t.Fatalf("round trip = %#v, %v", loaded, err)
	}
	if _, err := os.ReadFile(path); err != nil {
		t.Fatalf("atomic output missing: %v", err)
	}
}
