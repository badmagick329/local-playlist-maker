package native

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"playlistmaker/charm/internal/config"
	"playlistmaker/charm/internal/library"
	"playlistmaker/charm/internal/metadata"
)

func TestLoaderBuildsThePortableLibraryFixture(t *testing.T) {
	root := copyFixture(t)
	loaded, err := config.Load(filepath.Join(root, "fixture.yml"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := (Loader{Config: loaded}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Tracks) != 2 || len(snapshot.Tracks[0].Variants) != 3 || snapshot.Tracks[0].Artist != "AURORA" {
		t.Fatalf("unexpected snapshot: %#v", snapshot.Tracks)
	}
	if got := snapshot.Tracks[0].Variants[2]; got.ID != filepath.Join(root, "VIDEOS", "240101 AURORA - Northern Lights.mkv") || got.Category != "Music Video" {
		t.Fatalf("default video identity/category = %#v", got)
	}
	if snapshot.Tracks[1].Title != "Pop!" || snapshot.Tracks[1].Variants[0].Filename != "240301 나연 - Pop!.mkv" {
		t.Fatalf("unicode track = %#v", snapshot.Tracks[1])
	}
}

func TestLoaderCompletesMappedAudioMissingFromCache(t *testing.T) {
	root := copyFixture(t)
	cache := filepath.Join(root, "data", "flac_cache.json")
	if err := os.WriteFile(cache, []byte(`[]`), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load(filepath.Join(root, "fixture.yml"))
	if err != nil {
		t.Fatal(err)
	}
	reader := fakeTagReader{}
	snapshot, err := (Loader{Config: loaded, TagReader: reader}).Load(context.Background())
	if err != nil || len(snapshot.Tracks) != 2 {
		t.Fatalf("cache completion = %#v, %v", snapshot, err)
	}
}

func TestClassifyRecognizesNumberedPerformances(t *testing.T) {
	for _, test := range []struct {
		name string
		path string
		want string
	}{
		{"exact", "Artist - Track Performance.mkv", "Performance"},
		{"numbered", "Artist - Track Performance 2.mkv", "Performance"},
		{"case insensitive", "Artist - Track pErFoRmAnCe 3.mkv", "Performance"},
		{"zero is not numbered", "Artist - Track Performance 0.mkv", "Music Video"},
		{"trailing words do not match", "Artist - Track Performance 2 final.mkv", "Music Video"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := string(classify(test.path)); got != test.want {
				t.Fatalf("classify(%q) = %q, want %q", test.path, got, test.want)
			}
		})
	}
}

func TestClassifyUsesSharedRecognizedParenthesizedVariants(t *testing.T) {
	for _, test := range []struct {
		path string
		want library.Category
	}{
		{"Artist - Track (Band Live).mkv", library.BandLive},
		{"Artist - Track (Jihyo Fancam).mkv", library.Fancam},
		{"Artist - Track (Japan Concert).mkv", library.Concert},
		{"Artist - Track (Live Audio).mkv", library.LiveAudio},
		{"Artist - Track (Whole Different Animal).mkv", library.MusicVideo},
	} {
		if got := classify(test.path); got != test.want {
			t.Fatalf("classify(%q) = %q, want %q", test.path, got, test.want)
		}
	}
}

func TestVideoDateAcceptsSixAndEightDigitPrefixes(t *testing.T) {
	for _, path := range []string{"241029 ITZY - Imaginary Friend.webm", "20241029 ITZY - Imaginary Friend.webm"} {
		date, label, ok := videoDate(path)
		if !ok || label != "2024-10-29" || date.IsZero() {
			t.Fatalf("videoDate(%q) = %v, %q, %t", path, date, label, ok)
		}
	}
}

func TestLoaderDoesNotScanConfiguredAudioDirectoriesAtStartup(t *testing.T) {
	root := copyFixture(t)
	loaded, err := config.Load(filepath.Join(root, "fixture.yml"))
	if err != nil {
		t.Fatal(err)
	}
	loaded.AudioDirectories = []string{filepath.Join(root, "missing-audio-library")}
	if _, err := (Loader{Config: loaded, ReadOnly: true}).Load(context.Background()); err != nil {
		t.Fatalf("startup scanned configured audio directories: %v", err)
	}
}

type fakeTagReader struct{}

func (fakeTagReader) Read(_ context.Context, path string) (metadata.Entry, error) {
	if strings.Contains(path, "Northern") {
		return metadata.Entry{Artist: "AURORA", Title: "Northern Lights", Date: "2024-01-15", TrackNumber: 1}, nil
	}
	return metadata.Entry{Artist: "나연", Title: "Pop!", Date: "2024-05", TrackNumber: 2}, nil
}

func copyFixture(t *testing.T) string {
	t.Helper()
	source, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "testdata", "charm-backend", "library-basic"))
	if err != nil {
		t.Fatal(err)
	}
	destination := t.TempDir()
	if err := filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if filepath.Ext(path) == ".json" {
			contents = []byte(strings.ReplaceAll(string(contents), "@ROOT@", strings.ReplaceAll(destination, `\`, `\\`)))
		} else {
			contents = []byte(strings.ReplaceAll(string(contents), "@ROOT@", destination))
		}
		return os.WriteFile(target, contents, 0o600)
	}); err != nil {
		t.Fatal(err)
	}
	manifestContents, err := os.ReadFile(filepath.Join(destination, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		VideoModificationTimesUTC map[string]time.Time `json:"videoModificationTimesUtc"`
	}
	if err := json.Unmarshal(manifestContents, &manifest); err != nil {
		t.Fatal(err)
	}
	for relative, modified := range manifest.VideoModificationTimesUTC {
		path := filepath.Join(destination, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, modified, modified); err != nil {
			t.Fatal(err)
		}
	}
	return destination
}
