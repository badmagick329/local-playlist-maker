package lastfm

import (
	"testing"
	"time"

	"playlistmaker/charm/internal/library"
)

// resolved runs matching for one scrobbled spelling and returns its match.
func resolved(t *testing.T, artist, title string, tracks []library.Track, stored ...Match) (Match, bool) {
	t.Helper()
	s := Service{}
	s.index = buildIndex([]Scrobble{{Artist: artist, Title: title, PlayedAtUTC: time.Unix(1, 0)}}, stored, nil)
	s.resolve(tracks)
	m, ok := s.index.Matches[SourceKey(artist, title)]
	return m, ok && m.Status == "match"
}

func TestAccentsAndFullwidthFoldOnlyWhenExactKeysMiss(t *testing.T) {
	if m, ok := resolved(t, "Dreamcatcher", "Déjà Vu", []library.Track{testTrack("dv", "DREAMCATCHER", "Deja Vu")}); !ok || m.TrackID != "dv" {
		t.Fatalf("accented title did not match: %#v", m)
	}
	if m, ok := resolved(t, "ＭＡＭＡＭＯＯ", "Decalcomanie", []library.Track{testTrack("d", "MAMAMOO", "Décalcomanie")}); !ok || m.TrackID != "d" {
		t.Fatalf("fullwidth artist and accented catalogue title did not match: %#v", m)
	}
	if _, ok := resolved(t, "Dreamcatcher", "Déjà Vu", []library.Track{testTrack("a", "Dreamcatcher", "Deja Vu"), testTrack("b", "Dreamcatcher", "Dejà Vu")}); ok {
		t.Fatal("folded key matched two distinct songs")
	}
	if m, ok := resolved(t, "Dreamcatcher", "Déjà Vu", []library.Track{testTrack("exact", "Dreamcatcher", "Déjà Vu"), testTrack("plain", "Dreamcatcher", "Deja Vu")}); !ok || m.TrackID != "exact" {
		t.Fatalf("folding overrode the exact hit: %#v", m)
	}
}

// A loose tier that finds several songs leaves a reviewed no_match closed;
// otherwise the case would return to review after every import.
func TestNoMatchReopensOnLooseTierOnlyForOneSong(t *testing.T) {
	noMatch := Match{SourceKey: SourceKey("Dreamcatcher", "Déjà Vu"), Status: "no_match", Provenance: "agent"}
	two := []library.Track{testTrack("a", "Dreamcatcher", "Deja Vu"), testTrack("b", "Dreamcatcher", "Dejà Vu")}
	s := Service{}
	s.index = buildIndex([]Scrobble{{Artist: "Dreamcatcher", Title: "Déjà Vu", PlayedAtUTC: time.Unix(1, 0)}}, []Match{noMatch}, nil)
	s.resolve(two)
	if s.index.Matches[noMatch.SourceKey].Status != "no_match" {
		t.Fatalf("ambiguous folded hit reopened the no_match: %#v", s.index.Matches[noMatch.SourceKey])
	}
	if m, ok := resolved(t, "Dreamcatcher", "Déjà Vu", two[:1], noMatch); !ok || m.TrackID != "a" {
		t.Fatalf("folded hit on one song did not reopen the no_match: %#v", m)
	}
}
