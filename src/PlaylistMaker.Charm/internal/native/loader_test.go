package native

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"playlistmaker/charm/internal/catalog"
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
	if len(snapshot.Tracks) != 2 {
		t.Fatalf("unexpected snapshot: %#v", snapshot.Tracks)
	}
	var aurora, nayeon library.Track
	for _, track := range snapshot.Tracks {
		if track.Artist == "AURORA" {
			aurora = track
		} else if track.Artist == "나연" {
			nayeon = track
		}
	}
	if len(aurora.Variants) != 3 {
		t.Fatalf("AURORA variants = %#v", aurora.Variants)
	}
	if got := aurora.Variants[2]; got.ID != filepath.Join(root, "VIDEOS", "240101 AURORA - Northern Lights.mkv") || got.Category != "Music Video" {
		t.Fatalf("default video identity/category = %#v", got)
	}
	if nayeon.Title != "Pop!" || nayeon.Variants[0].Filename != "240301 나연 - Pop!.mkv" {
		t.Fatalf("unicode track = %#v", nayeon)
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
	mappingContents, err := os.ReadFile(filepath.Join(destination, "data", "video-audio-map.json"))
	if err != nil {
		t.Fatal(err)
	}
	var mappings map[string]string
	if err := json.Unmarshal(mappingContents, &mappings); err != nil {
		t.Fatal(err)
	}
	media := catalog.New()
	trackIDs := map[string]string{}
	for _, audio := range mappings {
		if _, ok := trackIDs[strings.ToLower(audio)]; ok {
			continue
		}
		id := "trk_" + strings.Repeat("0", 23) + string(rune('1'+len(trackIDs)))
		trackIDs[strings.ToLower(audio)] = id
		artist, title, date := "AURORA", "Northern Lights", "2024-01-15"
		if strings.Contains(audio, "Pop.flac") {
			artist, title, date = "나연", "Pop!", "2024-05"
		}
		media.Tracks = append(media.Tracks, catalog.Track{ID: id, Artist: artist, Title: title, ReleaseDate: date, LocalAudioPath: audio})
	}
	for video, audio := range mappings {
		media.Videos = append(media.Videos, catalog.Video{Path: video, TrackID: trackIDs[strings.ToLower(audio)]})
	}
	if err := catalog.Write(filepath.Join(destination, "data", "media-catalog.json"), media); err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join(destination, "fixture.yml")
	fixtureContents, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(fixtureContents), "mappingFile: data/video-audio-map.json", "mediaCatalogFile: data/media-catalog.json", 1)
	updated = strings.Replace(updated, "audioPlaylistCommand: [\"foobar2000.exe\", \"{playlistPath}\"]", "localTrackingStartCommand: [\"foobar2000.exe\", \"{audioPath}\"]", 1)
	updated = strings.Replace(updated, "audioPlaylistSuffix: \"_audios.m3u8\"", "localTrackingStopCommand: [\"foobar2000.exe\", \"/stop\"]", 1)
	updated = strings.Replace(updated, "audioSingleFileCommand: [\"foobar2000.exe\"]", "", 1)
	if err := os.WriteFile(fixture, []byte(updated), 0o600); err != nil {
		t.Fatal(err)
	}
	return destination
}
