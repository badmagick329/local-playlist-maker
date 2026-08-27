package updater

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"playlistmaker/charm/internal/catalog"
	"playlistmaker/charm/internal/config"
	"playlistmaker/charm/internal/metadata"
	"playlistmaker/charm/internal/pathid"
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

func TestScanFuzzySuggestsLongerCatalogueTitle(t *testing.T) {
	root := t.TempDir()
	videos := filepath.Join(root, "videos")
	if err := os.MkdirAll(videos, 0o755); err != nil {
		t.Fatal(err)
	}
	video := filepath.Join(videos, "260826 aespa - Switchblade (Concert).mp4")
	if err := os.WriteFile(video, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	audioPath := filepath.Join(root, "audio", "switchblade.flac")
	otherAudioPath := filepath.Join(root, "audio", "other.flac")
	catalogPath := filepath.Join(root, "data", "catalog.json")
	media := catalog.New()
	media.Tracks = []catalog.Track{
		{ID: "trk_switchblade", Artist: "aespa", Title: "Switchblade", LocalAudioPath: audioPath},
		{ID: "trk_other", Artist: "aespa", Title: "Another Song", LocalAudioPath: otherAudioPath},
	}
	if err := catalog.Write(catalogPath, media); err != nil {
		t.Fatal(err)
	}
	cachePath := filepath.Join(root, "data", "cache.json")
	if err := metadata.WriteCache(cachePath, map[string]metadata.Entry{
		pathid.ComparisonKey(audioPath):      {FilePath: audioPath, Artist: "aespa", Title: "Switchblade (feat. Ty Dolla $ign)"},
		pathid.ComparisonKey(otherAudioPath): {FilePath: otherAudioPath, Artist: "aespa", Title: "Another Song"},
	}); err != nil {
		t.Fatal(err)
	}
	service := Service{Config: config.Config{DataDirectory: filepath.Join(root, "data"), MediaCatalogFile: catalogPath, FlacCacheFile: cachePath, VideoDirectories: []string{videos}}}
	items, err := service.Scan(context.Background())
	if err != nil || len(items) != 1 {
		t.Fatalf("scan = %#v, %v", items, err)
	}
	if items[0].AudioPath != "trk_switchblade" || items[0].Reason != "Possible match" {
		t.Fatalf("switchblade suggestion = %#v", items[0])
	}
}

func TestFuzzyMatchIsSymmetricAndRejectsAmbiguousTitles(t *testing.T) {
	tests := []struct {
		name       string
		videoTitle string
		candidates []metadata.Entry
		wantPath   string
		wantOK     bool
	}{
		{
			name:       "video title is shorter",
			videoTitle: "Switchblade",
			candidates: []metadata.Entry{{FilePath: "long.flac", Artist: "aespa", Title: "Switchblade (feat. Ty Dolla $ign)"}},
			wantPath:   "long.flac",
			wantOK:     true,
		},
		{
			name:       "video title is longer",
			videoTitle: "Switchblade (feat. Ty Dolla $ign)",
			candidates: []metadata.Entry{{FilePath: "short.flac", Artist: "aespa", Title: "Switchblade"}},
			wantPath:   "short.flac",
			wantOK:     true,
		},
		{
			name:       "unrelated title",
			videoTitle: "Switchblade",
			candidates: []metadata.Entry{{FilePath: "other.flac", Artist: "aespa", Title: "Another Song"}},
			wantOK:     false,
		},
		{
			name:       "equal best matches are ambiguous",
			videoTitle: "Switchblade",
			candidates: []metadata.Entry{
				{FilePath: "first.flac", Artist: "aespa", Title: "Switchblade (Live)"},
				{FilePath: "second.flac", Artist: "aespa", Title: "Switchblade (Live)"},
			},
			wantOK: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := fuzzyMatch(test.videoTitle, test.candidates)
			if ok != test.wantOK || test.wantOK && got.FilePath != test.wantPath {
				t.Fatalf("fuzzyMatch() = %#v, %v; want %q, %v", got, ok, test.wantPath, test.wantOK)
			}
		})
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
