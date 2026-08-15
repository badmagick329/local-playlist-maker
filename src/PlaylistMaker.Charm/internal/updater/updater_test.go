package updater

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"playlistmaker/charm/internal/config"
	"playlistmaker/charm/internal/mapping"
	"playlistmaker/charm/internal/metadata"
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
	items, err := (Service{Config: config.Config{MappingFile: mapPath, FlacCacheFile: cachePath, VideoDirectories: []string{videos}}}).Scan(context.Background())
	if err != nil || len(items) != 2 {
		t.Fatalf("scan = %#v, %v", items, err)
	}
	byFilename := map[string]Item{}
	for _, item := range items {
		byFilename[item.Filename] = item
	}
	if item := byFilename[filepath.Base(used)]; !strings.Contains(item.Reason, "1 videos already linked") || item.AudioPath != "audio\\song.flac" {
		t.Fatalf("used suggestion = %#v", item)
	}
	if item := byFilename[filepath.Base(cacheOnly)]; item.Reason != "Exact cache match" || item.AudioPath != "audio\\pop.flac" {
		t.Fatalf("cache suggestion = %#v", item)
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
	items, err := (Service{Config: config.Config{MappingFile: mapPath, FlacCacheFile: cachePath, VideoDirectories: []string{videos}}}).Scan(context.Background())
	if err != nil || len(items) != 1 || items[0].AudioPath != "" {
		t.Fatalf("ambiguous = %#v, %v", items, err)
	}
}
