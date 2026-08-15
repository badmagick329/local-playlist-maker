package updater

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"playlistmaker/charm/internal/config"
	"playlistmaker/charm/internal/mapping"
	"playlistmaker/charm/internal/metadata"
	"playlistmaker/charm/internal/pathid"
)

func TestScanOrdersUnmappedVideosAndSuggestsExactMatches(t *testing.T) {
	root := t.TempDir()
	videos := filepath.Join(root, "videos")
	if err := os.MkdirAll(filepath.Join(videos, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	mapped := filepath.Join(videos, "250101 Artist - Song.mkv")
	used := filepath.Join(videos, "nested", "250102 Artist - Song Performance 2.mkv")
	cacheOnly := filepath.Join(videos, "250103 나연 - Pop!.mp4")
	for _, path := range []string{mapped, used, cacheOnly, filepath.Join(videos, "ignore.txt")} {
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	mapPath, cachePath := filepath.Join(root, "data", "map.json"), filepath.Join(root, "data", "cache.json")
	if err := mapping.Write(mapPath, []mapping.Entry{{VideoPath: mapped, AudioPath: "audio/song.flac"}}); err != nil {
		t.Fatal(err)
	}
	if err := metadata.WriteCache(cachePath, map[string]metadata.Entry{
		"song": {FilePath: "audio/song.flac", Artist: "Artist", Title: "Song"},
		"pop":  {FilePath: "audio/pop.flac", Artist: "나연", Title: "Pop!"},
	}); err != nil {
		t.Fatal(err)
	}
	items, err := (Service{Config: config.Config{DataDirectory: filepath.Join(root, "data"), MappingFile: mapPath, FlacCacheFile: cachePath, VideoDirectories: []string{videos}}}).Scan(context.Background())
	if err != nil || len(items) != 2 {
		t.Fatalf("scan = %#v, %v", items, err)
	}
	byFilename := map[string]Item{}
	for _, item := range items {
		byFilename[item.Filename] = item
	}
	if item := byFilename[filepath.Base(used)]; item.Reason != "Exact match" || item.AudioPath != "audio\\song.flac" {
		t.Fatalf("used suggestion = %#v", item)
	}
	if item := byFilename[filepath.Base(cacheOnly)]; item.Reason != "Exact match" || item.AudioPath != "audio\\pop.flac" {
		t.Fatalf("cache suggestion = %#v", item)
	}
}

func TestScanSuggestionOrderAndFuzzyRules(t *testing.T) {
	for _, test := range []struct {
		name       string
		filename   string
		cache      map[string]metadata.Entry
		mapped     []mapping.Entry
		wantAudio  string
		wantReason string
	}{
		{
			name:       "normalization supports case punctuation and whitespace",
			filename:   "260724 BILLlie - Work!!!.mkv",
			cache:      map[string]metadata.Entry{"work": {FilePath: "audio/work.flac", Artist: "billlie", Title: "Work"}},
			wantAudio:  "audio\\work.flac",
			wantReason: "Exact match",
		},
		{
			name:     "mapped evidence takes priority",
			filename: "260724 Billlie - Work.mkv",
			cache: map[string]metadata.Entry{
				"evidence": {FilePath: "audio/evidence.flac", Artist: "Billlie", Title: "Work"},
				"other":    {FilePath: "audio/other.flac", Artist: "Billlie", Title: "Work"},
			},
			mapped:     []mapping.Entry{{VideoPath: "mapped.mkv", AudioPath: "audio/evidence.flac"}},
			wantAudio:  "audio\\evidence.flac",
			wantReason: "Exact match",
		},
		{
			name:       "unique cross artist exact title is possible",
			filename:   "260724 Brave Girls - Body Wave.mkv",
			cache:      map[string]metadata.Entry{"body": {FilePath: "audio/body.flac", Artist: "BBGIRLS", Title: "BODY WAVE"}},
			wantAudio:  "audio\\body.flac",
			wantReason: "Possible match",
		},
		{
			name:     "ambiguous cross artist exact title is withheld",
			filename: "260724 Brave Girls - Body Wave.mkv",
			cache: map[string]metadata.Entry{
				"one": {FilePath: "audio/one.flac", Artist: "BBGIRLS", Title: "Body Wave"},
				"two": {FilePath: "audio/two.flac", Artist: "Other", Title: "Body Wave"},
			},
		},
		{
			name:       "unique same artist fuzzy match",
			filename:   "260724 Billlie - Work Special Stage.mkv",
			cache:      map[string]metadata.Entry{"work": {FilePath: "audio/work.flac", Artist: "Billlie", Title: "Work"}},
			wantAudio:  "audio\\work.flac",
			wantReason: "Possible match",
		},
		{
			name:     "fuzzy ties are withheld",
			filename: "260724 Billlie - Work Special Stage.mkv",
			cache: map[string]metadata.Entry{
				"one": {FilePath: "audio/one.flac", Artist: "Billlie", Title: "Work"},
				"two": {FilePath: "audio/two.flac", Artist: "Billlie", Title: "Work!"},
			},
		},
		{
			name:     "different artists are not fuzzy matched",
			filename: "260724 Billlie - Work Special Stage.mkv",
			cache:    map[string]metadata.Entry{"work": {FilePath: "audio/work.flac", Artist: "Someone Else", Title: "Work"}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			videos := filepath.Join(root, "videos")
			video := filepath.Join(videos, test.filename)
			if err := os.MkdirAll(videos, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(video, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			mapPath, cachePath := filepath.Join(root, "data", "map.json"), filepath.Join(root, "data", "cache.json")
			if err := mapping.Write(mapPath, test.mapped); err != nil {
				t.Fatal(err)
			}
			if err := metadata.WriteCache(cachePath, test.cache); err != nil {
				t.Fatal(err)
			}
			items, err := (Service{Config: config.Config{DataDirectory: filepath.Join(root, "data"), MappingFile: mapPath, FlacCacheFile: cachePath, VideoDirectories: []string{videos}}}).Scan(context.Background())
			if err != nil || len(items) != 1 || items[0].AudioPath != test.wantAudio || items[0].Reason != test.wantReason {
				t.Fatalf("suggestion = %#v, %v", items, err)
			}
			entries, err := mapping.Read(mapPath)
			if err != nil || len(entries) != len(test.mapped) {
				t.Fatalf("scan saved a suggestion: %#v, %v", entries, err)
			}
		})
	}
}

type audioReader struct {
	entries map[string]metadata.Entry
	reads   []string
}

func (r *audioReader) Read(_ context.Context, path string) (metadata.Entry, error) {
	r.reads = append(r.reads, path)
	return r.entries[pathid.ComparisonKey(path)], nil
}

func TestScanDiscoversRecursiveAudioAndRefreshesManualSearch(t *testing.T) {
	root := t.TempDir()
	videos, audio := filepath.Join(root, "videos"), filepath.Join(root, "audio")
	video := filepath.Join(videos, "260724 Brave Girls - Body Wave.mkv")
	newAudio := filepath.Join(audio, "artists", "BBGIRLS", "body-wave.flac")
	for _, path := range []string{video, newAudio, filepath.Join(audio, "artists", "BBGIRLS", "ignore.mp3")} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	mapPath, cachePath := writeScanData(t, root)
	reader := &audioReader{entries: map[string]metadata.Entry{pathid.ComparisonKey(newAudio): {Artist: "BBGIRLS", Title: "BODY WAVE"}}}
	service := Service{Config: config.Config{DataDirectory: filepath.Join(root, "data"), MappingFile: mapPath, FlacCacheFile: cachePath, VideoDirectories: []string{videos}, AudioDirectories: []string{audio, strings.ToLower(audio)}}, Reader: reader}
	items, err := service.Scan(context.Background())
	if err != nil || len(items) != 1 || items[0].AudioPath != pathid.Normalize(newAudio) || items[0].Reason != "Possible match" || len(reader.reads) != 1 {
		t.Fatalf("audio discovery = %#v, %v, reads=%#v", items, err, reader.reads)
	}
	results, err := service.Search(context.Background(), "body wave")
	if err != nil || len(results) != 1 || results[0].Artist != "BBGIRLS" || results[0].Title != "BODY WAVE" {
		t.Fatalf("refreshed search = %#v, %v", results, err)
	}
	if _, err := service.Scan(context.Background()); err != nil || len(reader.reads) != 1 {
		t.Fatalf("cached audio was reread: %v, reads=%#v", err, reader.reads)
	}
}

func TestScanDoesNotChooseAmbiguousExactMatches(t *testing.T) {
	root := t.TempDir()
	videos := filepath.Join(root, "videos")
	if err := os.MkdirAll(videos, 0o755); err != nil {
		t.Fatal(err)
	}
	video := filepath.Join(videos, "250101 Artist - Song.mkv")
	if err := os.WriteFile(video, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	mapPath, cachePath := filepath.Join(root, "map.json"), filepath.Join(root, "cache.json")
	if err := mapping.Write(mapPath, nil); err != nil {
		t.Fatal(err)
	}
	if err := metadata.WriteCache(cachePath, map[string]metadata.Entry{"one": {FilePath: "audio/one.flac", Artist: "Artist", Title: "Song"}, "two": {FilePath: "audio/two.flac", Artist: "artist", Title: "song"}}); err != nil {
		t.Fatal(err)
	}
	items, err := (Service{Config: config.Config{DataDirectory: filepath.Join(root, "data"), MappingFile: mapPath, FlacCacheFile: cachePath, VideoDirectories: []string{videos}}}).Scan(context.Background())
	if err != nil || len(items) != 1 || items[0].AudioPath != "" {
		t.Fatalf("ambiguous = %#v, %v", items, err)
	}
}

func TestScanExcludesConfiguredDirectorySubtreesCaseInsensitively(t *testing.T) {
	root := t.TempDir()
	videos := filepath.Join(root, "videos")
	excluded := filepath.Join(videos, "Archived")
	keep := filepath.Join(videos, "keep", "250101 Artist - Keep.mkv")
	skip := filepath.Join(excluded, "nested", "250101 Artist - Skip.mkv")
	for _, path := range []string{keep, skip} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	mapPath, cachePath := writeScanData(t, root)
	items, err := (Service{Config: config.Config{DataDirectory: filepath.Join(root, "data"), MappingFile: mapPath, FlacCacheFile: cachePath, VideoDirectories: []string{videos}, IgnoredVideoDirectories: []string{strings.ToLower(excluded)}}}).Scan(context.Background())
	if err != nil || len(items) != 1 || items[0].Filename != filepath.Base(keep) {
		t.Fatalf("excluded scan = %#v, %v", items, err)
	}
}

func TestIgnoredAndExcludedVideosDoNotChangeMappings(t *testing.T) {
	root := t.TempDir()
	videos := filepath.Join(root, "videos")
	video := filepath.Join(videos, "250101 Artist - Song.mkv")
	excludedVideo := filepath.Join(videos, "archived", "250101 Artist - Archived.mkv")
	if err := os.MkdirAll(videos, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(video, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(excludedVideo), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(excludedVideo, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	mapPath, cachePath := writeScanData(t, root)
	service := Service{Config: config.Config{DataDirectory: filepath.Join(root, "data"), MappingFile: mapPath, FlacCacheFile: cachePath, VideoDirectories: []string{videos}, IgnoredVideoDirectories: []string{filepath.Dir(excludedVideo)}}}
	if err := service.Ignore(video); err != nil {
		t.Fatal(err)
	}
	if items, err := service.Scan(context.Background()); err != nil || len(items) != 0 {
		t.Fatalf("ignored scan = %#v, %v", items, err)
	}
	if entries, err := mapping.Read(mapPath); err != nil || len(entries) != 0 {
		t.Fatalf("scan wrote mappings = %#v, %v", entries, err)
	}
	if err := service.Restore(video); err != nil {
		t.Fatal(err)
	}
	if items, err := service.Scan(context.Background()); err != nil || len(items) != 1 {
		t.Fatalf("restored scan = %#v, %v", items, err)
	}
}

func TestIgnoredFileIsMissingSafeAndDeterministic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "ignored-videos.json")
	if paths, err := ReadIgnored(path); err != nil || len(paths) != 0 {
		t.Fatalf("missing ignored file = %#v, %v", paths, err)
	}
	input := []string{"C:/Videos/B.mkv", "c:/videos/a.mkv", "C:/VIDEOS/A.MKV", ""}
	if err := WriteIgnored(path, input); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	reversed := append([]string(nil), input...)
	slices.Reverse(reversed)
	if err := WriteIgnored(path, reversed); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(path)
	if err != nil || string(first) != string(second) {
		t.Fatalf("ignored file was not deterministic: %q, %q, %v", first, second, err)
	}
	paths, err := ReadIgnored(path)
	if err != nil || len(paths) != 2 {
		t.Fatalf("ignored file = %#v, %v", paths, err)
	}
	if leftovers, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".ignored-videos-*.tmp")); err != nil || len(leftovers) != 0 {
		t.Fatalf("atomic temporary files = %#v, %v", leftovers, err)
	}
}

func writeScanData(t *testing.T, root string) (string, string) {
	t.Helper()
	mapPath, cachePath := filepath.Join(root, "data", "map.json"), filepath.Join(root, "data", "cache.json")
	if err := mapping.Write(mapPath, nil); err != nil {
		t.Fatal(err)
	}
	if err := metadata.WriteCache(cachePath, map[string]metadata.Entry{}); err != nil {
		t.Fatal(err)
	}
	return mapPath, cachePath
}
