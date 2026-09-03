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
	result, err := service.Scan(context.Background())
	if err != nil || len(result.Items) != 1 || result.Items[0].VideoPath != unmapped || result.Items[0].AudioPath != "trk_song" {
		t.Fatalf("scan = %#v, %v", result, err)
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
	result, err := service.Scan(context.Background())
	if err != nil || len(result.Items) != 1 {
		t.Fatalf("scan = %#v, %v", result, err)
	}
	if result.Items[0].AudioPath != "trk_switchblade" || result.Items[0].Reason != "Possible match" {
		t.Fatalf("switchblade suggestion = %#v", result.Items[0])
	}
}

func TestScanKeepsUniqueUnclaimedLocalAudioSuggestion(t *testing.T) {
	root := t.TempDir()
	videos := filepath.Join(root, "videos")
	if err := os.MkdirAll(videos, 0o755); err != nil {
		t.Fatal(err)
	}
	video := filepath.Join(videos, "260831 Girls Generation - Skibidi.mkv")
	if err := os.WriteFile(video, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	audioPath := filepath.Join(root, "01 Skibidi.flac")
	catalogPath := filepath.Join(root, "data", "catalog.json")
	cachePath := filepath.Join(root, "data", "cache.json")
	if err := catalog.Write(catalogPath, catalog.New()); err != nil {
		t.Fatal(err)
	}
	if err := metadata.WriteCache(cachePath, map[string]metadata.Entry{
		pathid.ComparisonKey(audioPath): {FilePath: audioPath, Artist: "Girls' Generation-HRS", Title: "Skibidi"},
	}); err != nil {
		t.Fatal(err)
	}
	service := Service{Config: config.Config{DataDirectory: filepath.Join(root, "data"), MediaCatalogFile: catalogPath, FlacCacheFile: cachePath, VideoDirectories: []string{videos}}}
	result, err := service.Scan(context.Background())
	if err != nil || len(result.Items) != 1 {
		t.Fatalf("scan = %#v, %v", result, err)
	}
	item := result.Items[0]
	if item.AudioPath != audioPath || item.AudioTitle != "Skibidi" || item.Reason != "Possible match" {
		t.Fatalf("suggestion = %#v", item)
	}
}

func TestScanRemovesMissingManagedVideosAndKeepsExcludedMappings(t *testing.T) {
	root := t.TempDir()
	videos := filepath.Join(root, "videos")
	ignored := filepath.Join(videos, "archive")
	if err := os.MkdirAll(ignored, 0o755); err != nil {
		t.Fatal(err)
	}
	present := filepath.Join(videos, "240101 Present - Song.mkv")
	newVideo := filepath.Join(videos, "240102 New - Song.mkv")
	if err := os.WriteFile(present, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newVideo, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	catalogPath := filepath.Join(root, "data", "catalog.json")
	media := catalog.New()
	media.Tracks = []catalog.Track{
		{ID: "trk_present", Artist: "Present", Title: "Song"},
		{ID: "trk_missing", Artist: "Missing", Title: "Song"},
		{ID: "trk_ignored", Artist: "Ignored", Title: "Song"},
		{ID: "trk_outside", Artist: "Outside", Title: "Song"},
	}
	media.Videos = []catalog.Video{
		{Path: present, TrackID: "trk_present"},
		{Path: filepath.Join(videos, "240103 Missing - Song.mkv"), TrackID: "trk_missing"},
		{Path: filepath.Join(ignored, "240104 Ignored - Song.mkv"), TrackID: "trk_ignored"},
		{Path: filepath.Join(root, "other", "240105 Outside - Song.mkv"), TrackID: "trk_outside"},
	}
	if err := catalog.Write(catalogPath, media); err != nil {
		t.Fatal(err)
	}
	cachePath := filepath.Join(root, "data", "cache.json")
	if err := metadata.WriteCache(cachePath, map[string]metadata.Entry{}); err != nil {
		t.Fatal(err)
	}
	service := Service{Config: config.Config{
		DataDirectory: filepath.Join(root, "data"), MediaCatalogFile: catalogPath, FlacCacheFile: cachePath,
		VideoDirectories: []string{videos}, IgnoredVideoDirectories: []string{ignored},
	}}
	result, err := service.Scan(context.Background())
	if err != nil || result.Removed != 1 || len(result.Items) != 1 || result.Items[0].VideoPath != newVideo {
		t.Fatalf("scan = %#v, %v", result, err)
	}
	loaded, err := catalog.Read(catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Tracks) != 4 || len(loaded.Videos) != 3 {
		t.Fatalf("catalogue counts = %d tracks, %d videos", len(loaded.Tracks), len(loaded.Videos))
	}
	for _, video := range loaded.Videos {
		if video.TrackID == "trk_missing" {
			t.Fatal("managed missing mapping was retained")
		}
	}
}

func TestScanDoesNotPruneWhenVideoRootCannotBeRead(t *testing.T) {
	root := t.TempDir()
	catalogPath := filepath.Join(root, "data", "catalog.json")
	missingVideo := filepath.Join(root, "videos", "240101 Missing - Song.mkv")
	media := catalog.New()
	media.Tracks = []catalog.Track{{ID: "trk_missing", Artist: "Missing", Title: "Song"}}
	media.Videos = []catalog.Video{{Path: missingVideo, TrackID: "trk_missing"}}
	if err := catalog.Write(catalogPath, media); err != nil {
		t.Fatal(err)
	}
	cachePath := filepath.Join(root, "data", "cache.json")
	if err := metadata.WriteCache(cachePath, map[string]metadata.Entry{}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	service := Service{Config: config.Config{
		DataDirectory: filepath.Join(root, "data"), MediaCatalogFile: catalogPath, FlacCacheFile: cachePath,
		VideoDirectories: []string{filepath.Join(root, "videos")},
	}}
	result, err := service.Scan(context.Background())
	if err == nil || result.Removed != 0 {
		t.Fatalf("scan = %#v, %v", result, err)
	}
	after, err := os.ReadFile(catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("catalogue changed after failed root scan")
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
	root := t.TempDir()
	path := filepath.Join(root, "catalog.json")
	cachePath := filepath.Join(root, "cache.json")
	media := catalog.New()
	claimed := filepath.Join(root, "claimed.flac")
	available := filepath.Join(root, "skibidi.flac")
	media.Tracks = []catalog.Track{{ID: "trk_one", Artist: "Artist", Title: "One", LocalAudioPath: claimed}, {ID: "trk_two", Artist: "Other", Title: "Two"}}
	if err := catalog.Write(path, media); err != nil {
		t.Fatal(err)
	}
	if err := metadata.WriteCache(cachePath, map[string]metadata.Entry{
		pathid.ComparisonKey(claimed):   {FilePath: claimed, Artist: "Artist", Title: "One"},
		pathid.ComparisonKey(available): {FilePath: available, Artist: "Girls' Generation-HRS", Title: "Skibidi"},
	}); err != nil {
		t.Fatal(err)
	}
	service := Service{Config: config.Config{MediaCatalogFile: path, FlacCacheFile: cachePath}}
	items, err := service.Search(context.Background(), "artist one")
	if err != nil || len(items) != 1 || items[0].Path != "trk_one" {
		t.Fatalf("search = %#v, %v", items, err)
	}
	items, err = service.Search(context.Background(), "sibidi")
	if err != nil || len(items) != 1 || items[0].Path != available {
		t.Fatalf("fuzzy local search = %#v, %v", items, err)
	}
}

func TestConfirmCreatesTrackForUnclaimedLocalAudio(t *testing.T) {
	root := t.TempDir()
	catalogPath := filepath.Join(root, "catalog.json")
	cachePath := filepath.Join(root, "cache.json")
	audioPath := filepath.Join(root, "skibidi.flac")
	if err := catalog.Write(catalogPath, catalog.New()); err != nil {
		t.Fatal(err)
	}
	if err := metadata.WriteCache(cachePath, map[string]metadata.Entry{
		pathid.ComparisonKey(audioPath): {FilePath: audioPath, Artist: "Girls' Generation-HRS", Title: "Skibidi", Date: "2026-08-31"},
	}); err != nil {
		t.Fatal(err)
	}
	service := Service{Config: config.Config{MediaCatalogFile: catalogPath, FlacCacheFile: cachePath}}
	if err := service.Confirm("260831 Girls Generation - Skibidi.mkv", audioPath); err != nil {
		t.Fatal(err)
	}
	loaded, err := catalog.Read(catalogPath)
	if err != nil || len(loaded.Tracks) != 1 || len(loaded.Videos) != 1 {
		t.Fatalf("catalogue = %#v, %v", loaded, err)
	}
	if loaded.Tracks[0].LocalAudioPath != audioPath || loaded.Videos[0].TrackID != loaded.Tracks[0].ID {
		t.Fatalf("local track was not linked: %#v", loaded)
	}
}
