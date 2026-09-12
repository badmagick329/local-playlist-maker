package updater

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"playlistmaker/charm/internal/catalog"
	"playlistmaker/charm/internal/config"
	"playlistmaker/charm/internal/metadata"
)

func TestConfirmRejectsUnavailableAudioWithoutChangingCatalogue(t *testing.T) {
	for _, selection := range []string{"catalogue track", "cached audio"} {
		for _, state := range []string{"missing", "directory"} {
			t.Run(selection+"/"+state, func(t *testing.T) {
				root := t.TempDir()
				audio := filepath.Join(root, "selected.flac")
				if state == "directory" {
					if err := os.Mkdir(audio, 0o755); err != nil {
						t.Fatal(err)
					}
				}
				service := Service{Config: config.Config{MediaCatalogFile: filepath.Join(root, "catalog.json"), FlacCacheFile: filepath.Join(root, "cache.json")}}
				media := catalog.New()
				selected := audio
				if selection == "catalogue track" {
					selected = "trk_selected"
					media.Tracks = []catalog.Track{{ID: selected, Artist: "Artist", Title: "Song", LocalAudioPath: audio}}
				}
				if err := catalog.Write(service.Config.MediaCatalogFile, media); err != nil {
					t.Fatal(err)
				}
				if err := metadata.WriteCache(service.Config.FlacCacheFile, map[string]metadata.Entry{audio: {FilePath: audio, Artist: "Artist", Title: "Song"}}); err != nil {
					t.Fatal(err)
				}
				before, err := os.ReadFile(service.Config.MediaCatalogFile)
				if err != nil {
					t.Fatal(err)
				}
				err = service.Confirm(filepath.Join(root, "video.mkv"), selected, false)
				want := "access selected audio:"
				if state == "directory" {
					want = "selected audio is a folder:"
				}
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Fatalf("confirmation error = %v, want %q", err, want)
				}
				after, err := os.ReadFile(service.Config.MediaCatalogFile)
				if err != nil || !bytes.Equal(before, after) {
					t.Fatalf("rejected selection changed catalogue: %v", err)
				}
			})
		}
	}
}
