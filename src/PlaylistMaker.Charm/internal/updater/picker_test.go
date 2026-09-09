package updater

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"playlistmaker/charm/internal/catalog"
	"playlistmaker/charm/internal/config"
	"playlistmaker/charm/internal/metadata"
)

func TestPickerSeparatesUnusedAndUnlinkedAndScopesDiscovery(t *testing.T) {
	root := t.TempDir()
	music := filepath.Join(root, "music")
	linked := filepath.Join(root, "outside", "linked.flac")
	outside := filepath.Join(root, "outside", "unlinked.flac")
	available := filepath.Join(music, "album.flac")
	missing := filepath.Join(music, "missing.flac")
	touchAudio(t, linked, outside, available)
	service := Service{Config: config.Config{MediaCatalogFile: filepath.Join(root, "catalog.json"), FlacCacheFile: filepath.Join(root, "cache.json"), AudioDirectories: []string{music}, DataDirectory: root}}
	media := catalog.New()
	media.Tracks = []catalog.Track{
		{ID: "trk_active", Artist: "CORTIS", Title: "REDRED", LocalAudioPath: linked, SpotifyURI: "spotify:track:same"},
		{ID: "trk_unused", Artist: "Cortis", Title: "REDRED", SpotifyURI: "spotify:track:same"},
		{ID: "trk_video", Artist: "Cortis", Title: "REDRED"},
	}
	media.Videos = []catalog.Video{{Path: filepath.Join(root, "one.mkv"), TrackID: "trk_active"}, {Path: filepath.Join(root, "two.mkv"), TrackID: "trk_video"}}
	if err := catalog.Write(service.Config.MediaCatalogFile, media); err != nil {
		t.Fatal(err)
	}
	entries := map[string]metadata.Entry{}
	for _, path := range []string{linked, outside, available, missing} {
		entries[path] = metadata.Entry{FilePath: path, Artist: "CORTIS", Title: "REDRED", Album: "Release"}
	}
	if err := metadata.WriteCache(service.Config.FlacCacheFile, entries); err != nil {
		t.Fatal(err)
	}
	results, err := service.Search(context.Background(), "redred")
	if err != nil || len(results) != 4 {
		t.Fatalf("results = %#v, %v", results, err)
	}
	if results[0].Kind != CatalogueAudio || results[1].Kind != CatalogueAudio || results[2].Kind != UnlinkedAudio || results[3].Kind != UnusedAudio {
		t.Fatalf("groups = %#v", results)
	}
	if results[3].VideoCount != 0 || !results[3].SpotifyOnly {
		t.Fatalf("unused = %#v", results[3])
	}
	for _, result := range results {
		if result.Path == outside || result.Path == missing {
			t.Fatalf("stale discovery candidate = %#v", result)
		}
	}
	scanned, err := service.refreshAudioCache(context.Background(), media)
	if err != nil || len(scanned) != 2 {
		t.Fatalf("scan cache = %#v, %v", scanned, err)
	}
	// Relinking away derives unused state without changing or deleting the identity.
	if err := service.Confirm(filepath.Join(root, "two.mkv"), "trk_active", false); err != nil {
		t.Fatal(err)
	}
	results, err = service.Search(context.Background(), "redred")
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range results {
		if result.Path == "trk_video" && result.Kind != UnusedAudio {
			t.Fatal("old identity stayed active")
		}
	}
	loaded, err := catalog.Read(service.Config.MediaCatalogFile)
	if err != nil || len(loaded.Tracks) != 3 {
		t.Fatal("relink removed history identity")
	}
}

func TestCreationOffersExistingIdentityWithoutSilentlyMergingReleases(t *testing.T) {
	for _, local := range []bool{false, true} {
		name := "video-only"
		if local {
			name = "local release"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			service := Service{Config: config.Config{MediaCatalogFile: filepath.Join(root, "catalog.json"), FlacCacheFile: filepath.Join(root, "cache.json")}}
			media := catalog.New()
			media.Tracks = []catalog.Track{{ID: "trk_existing", Artist: "CORTIS", Title: "REDRED", SpotifyURI: "spotify:track:existing"}}
			if err := catalog.Write(service.Config.MediaCatalogFile, media); err != nil {
				t.Fatal(err)
			}
			audio := filepath.Join(root, "new.flac")
			touchAudio(t, audio)
			if err := metadata.WriteCache(service.Config.FlacCacheFile, map[string]metadata.Entry{audio: {FilePath: audio, Artist: "Cortis", Title: "REDRED", Album: "Other release"}}); err != nil {
				t.Fatal(err)
			}
			create := func(allow bool) error {
				if local {
					return service.Confirm("video.mkv", audio, allow)
				}
				return service.Create("video.mkv", "Cortis", "REDRED", allow)
			}
			before, _ := os.ReadFile(service.Config.MediaCatalogFile)
			var duplicate *ExistingTracksError
			if err := create(false); !errors.As(err, &duplicate) {
				t.Fatalf("expected choices, got %v", err)
			}
			after, _ := os.ReadFile(service.Config.MediaCatalogFile)
			if string(before) != string(after) {
				t.Fatal("duplicate check mutated catalogue")
			}
			if local && duplicate.Selection != audio {
				t.Fatal("lost requested release")
			}
			if err := create(true); err != nil {
				t.Fatal(err)
			}
			loaded, err := catalog.Read(service.Config.MediaCatalogFile)
			if err != nil || len(loaded.Tracks) != 2 || loaded.Videos[0].TrackID == "trk_existing" {
				t.Fatalf("explicit separate release = %#v, %v", loaded, err)
			}
		})
	}
}
