package lastfm

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"playlistmaker/charm/internal/library"
	"playlistmaker/charm/internal/spotify"
)

type spotifyLookupStub struct {
	calls int
	value spotify.Track
	err   error
}

func (s *spotifyLookupStub) Track(context.Context, string) (spotify.Track, error) {
	s.calls++
	return s.value, s.err
}

func TestSpotifyEvidenceIsFetchedOncePersistedAndReused(t *testing.T) {
	var value spotify.Track
	if err := json.Unmarshal([]byte(`{"uri":"spotify:track:one","name":"Canonical Song","duration_ms":123000,"album":{"name":"Album","release_date":"2025-01-02"},"artists":[{"name":"Canonical Artist"}],"external_ids":{"isrc":"CODE"}}`), &value); err != nil {
		t.Fatal(err)
	}
	lookup := &spotifyLookupStub{value: value}
	track := testTrack("one", "Local", "Name")
	track.SpotifyURI = value.URI
	s := Service{DataDirectory: t.TempDir(), Spotify: lookup}
	s.index = buildIndex(nil, nil, nil)
	if err := s.enrichSpotify(context.Background(), []library.Track{track}, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.enrichSpotify(context.Background(), []library.Track{track}, nil); err != nil {
		t.Fatal(err)
	}
	if lookup.calls != 1 {
		t.Fatalf("Spotify calls=%d", lookup.calls)
	}
	var cache SpotifyCache
	if err := readJSON(filepath.Join(s.DataDirectory, SpotifyCacheFile), &cache, "cache"); err != nil {
		t.Fatal(err)
	}
	if len(cache.Tracks) != 1 || cache.Tracks[0].ISRC != "CODE" || cache.Tracks[0].Artists[0] != "Canonical Artist" {
		t.Fatalf("cache=%#v", cache)
	}
}
