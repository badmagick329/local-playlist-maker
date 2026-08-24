package updater

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"playlistmaker/charm/internal/catalog"
	"playlistmaker/charm/internal/config"
	"playlistmaker/charm/internal/metadata"
)

func TestScanSuggestsCatalogueTrackAndPreservesIgnoredVideos(t *testing.T) {
	root := t.TempDir()
	videos := filepath.Join(root, "videos")
	if err := os.MkdirAll(videos, 0o755); err != nil {
		t.Fatal(err)
	}
	mapped := filepath.Join(videos, "240101 Artist - Song.mkv")
	unmapped := filepath.Join(videos, "240102 Artist - Song Performance.mkv")
	ignored := filepath.Join(videos, "240103 Artist - Song Relay.mkv")
	for _, path := range []string{mapped, unmapped, ignored} {
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	catalogPath := filepath.Join(root, "data", "catalog.json")
	media := catalog.New()
	media.Tracks = []catalog.Track{{ID: "trk_song", Artist: "Artist", Title: "Song", LocalAudioPath: filepath.Join(root, "audio", "song.flac")}}
	media.Videos = []catalog.Video{{Path: mapped, TrackID: "trk_song"}}
	if err := catalog.Write(catalogPath, media); err != nil {
		t.Fatal(err)
	}
	cachePath := filepath.Join(root, "data", "cache.json")
	if err := metadata.WriteCache(cachePath, map[string]metadata.Entry{"song": {FilePath: media.Tracks[0].LocalAudioPath, Artist: "Artist", Title: "Song"}}); err != nil {
		t.Fatal(err)
	}
	service := Service{Config: config.Config{DataDirectory: filepath.Join(root, "data"), MediaCatalogFile: catalogPath, FlacCacheFile: cachePath, VideoDirectories: []string{videos}}}
	if err := service.Ignore(ignored); err != nil {
		t.Fatal(err)
	}
	items, err := service.Scan(context.Background())
	if err != nil || len(items) != 1 || items[0].VideoPath != unmapped || items[0].AudioPath != "trk_song" {
		t.Fatalf("scan = %#v, %v", items, err)
	}
}

func TestCreateAddsVideoOnlyTrackAndConfirmLinksExistingTrack(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "catalog.json")
	media := catalog.New()
	media.Tracks = []catalog.Track{{ID: "trk_existing", Artist: "Existing", Title: "Track"}}
	if err := catalog.Write(path, media); err != nil {
		t.Fatal(err)
	}
	service := Service{Config: config.Config{MediaCatalogFile: path}}
	if err := service.Create("video-one.mkv", "Artist", "Title"); err != nil {
		t.Fatal(err)
	}
	if err := service.Confirm("video-two.mkv", "trk_existing"); err != nil {
		t.Fatal(err)
	}
	loaded, err := catalog.Read(path)
	if err != nil || len(loaded.Tracks) != 2 || len(loaded.Videos) != 2 {
		t.Fatalf("catalogue = %#v, %v", loaded, err)
	}
	for _, track := range loaded.Tracks {
		if track.Artist == "Artist" && (track.ReleaseDate != "" || track.LocalAudioPath != "") {
			t.Fatalf("video-only track gained invented metadata: %#v", track)
		}
	}
}

func TestSearchReturnsCatalogueTracksInsteadOfAudioPaths(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog.json")
	media := catalog.New()
	media.Tracks = []catalog.Track{{ID: "trk_one", Artist: "Artist", Title: "One"}, {ID: "trk_two", Artist: "Other", Title: "Two"}}
	if err := catalog.Write(path, media); err != nil {
		t.Fatal(err)
	}
	items, err := (Service{Config: config.Config{MediaCatalogFile: path}}).Search(context.Background(), "artist one")
	if err != nil || len(items) != 1 || items[0].Path != "trk_one" {
		t.Fatalf("search = %#v, %v", items, err)
	}
}
