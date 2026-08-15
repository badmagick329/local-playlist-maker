package mapping

import (
	"os"
	"path/filepath"
	"strconv"
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

func TestWriteDeduplicatesLinearlyWithLastValueWinsAndStableOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "map.json")
	entries := []Entry{
		{VideoPath: `C:\Videos\B.mkv`, AudioPath: `C:\Audio\B.flac`},
		{VideoPath: `C:\Videos\A.mkv`, AudioPath: `C:\Audio\Old.flac`},
		{VideoPath: `c:/videos/a.mkv`, AudioPath: `C:\Audio\New.flac`},
	}
	if err := Write(path, entries); err != nil {
		t.Fatal(err)
	}
	loaded, err := Read(path)
	if err != nil || len(loaded) != 2 || loaded[0].VideoPath != `c:\videos\a.mkv` || loaded[0].AudioPath != `C:\Audio\New.flac` || loaded[1].VideoPath != `C:\Videos\B.mkv` {
		t.Fatalf("deduplicated mapping = %#v, %v", loaded, err)
	}
}

func BenchmarkDeduplicate6500(b *testing.B) {
	entries := syntheticEntries(6500)
	b.ResetTimer()
	for range b.N {
		if _, err := deduplicate(entries); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWrite6500(b *testing.B) {
	entries := syntheticEntries(6500)
	path := filepath.Join(b.TempDir(), "map.json")
	b.ResetTimer()
	for range b.N {
		if err := Write(path, entries); err != nil {
			b.Fatal(err)
		}
	}
}

func syntheticEntries(count int) []Entry {
	entries := make([]Entry, 0, count)
	for index := range count {
		entries = append(entries, Entry{VideoPath: filepath.Join(`C:\Videos`, "Artist", "video-"+strconv.Itoa(index)+".mkv"), AudioPath: filepath.Join(`C:\Audio`, "Artist", "track-"+strconv.Itoa(index)+".flac")})
	}
	return entries
}
