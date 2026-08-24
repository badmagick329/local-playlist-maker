package catalog

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCatalogRoundTripKeepsStableTrackRelationships(t *testing.T) {
	id, err := NewTrackID()
	if err != nil || !strings.HasPrefix(id, "trk_") {
		t.Fatalf("track ID = %q, %v", id, err)
	}
	path := filepath.Join(t.TempDir(), "catalog.json")
	value := New()
	value.Tracks = []Track{{ID: id, Artist: "Artist", Title: "Title", LocalAudioPath: `C:\Music\Track.flac`, SpotifyURI: "spotify:track:abc"}}
	if err := value.LinkVideo(`C:\Videos\One.mkv`, id); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, value); err != nil {
		t.Fatal(err)
	}
	loaded, err := Read(path)
	if err != nil || len(loaded.Tracks) != 1 || len(loaded.Videos) != 1 || loaded.Videos[0].TrackID != id {
		t.Fatalf("catalogue = %#v, %v", loaded, err)
	}
	loaded.Tracks[0].LocalAudioPath = ""
	if err := Write(path, loaded); err != nil {
		t.Fatalf("video-only track rejected: %v", err)
	}
}

func TestCatalogRejectsDanglingVideoTrackID(t *testing.T) {
	value := New()
	value.Videos = []Video{{Path: "one.mkv", TrackID: "trk_missing"}}
	if err := value.Validate(); err == nil {
		t.Fatal("expected dangling track ID error")
	}
}
