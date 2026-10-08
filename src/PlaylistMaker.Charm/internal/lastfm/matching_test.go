package lastfm

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"playlistmaker/charm/internal/library"
)

func TestExactMatchingStoredPrecedenceAndAmbiguity(t *testing.T) {
	tracks := []library.Track{testTrack("one", "Artist", "Song"), testTrack("two", "Other", "Title")}
	scrobbles := []Scrobble{{Artist: "ARTIST!", Title: "Song", PlayedAtUTC: time.Unix(1, 0)}, {Artist: "Alias", Title: "Hit", PlayedAtUTC: time.Unix(2, 0)}}
	s := Service{DataDirectory: t.TempDir()}
	s.index = buildIndex(scrobbles, []Match{{SourceKey: SourceKey("Alias", "Hit"), Status: "match", TrackID: "two", Provenance: "agent"}}, nil)
	s.resolve(tracks)
	if s.index.Matches[SourceKey("Artist", "Song")].TrackID != "one" || s.index.Matches[SourceKey("Alias", "Hit")].TrackID != "two" {
		t.Fatalf("matches=%#v", s.index.Matches)
	}
	duplicate := append(tracks, testTrack("three", "Artist", "Song"))
	s2 := Service{}
	s2.index = buildIndex(scrobbles, nil, nil)
	s2.resolve(duplicate)
	if _, ok := s2.index.Matches[SourceKey("Artist", "Song")]; ok {
		t.Fatal("ambiguous alias matched")
	}
}

func TestNoMatchSurvivesUnrelatedCatalogueChanges(t *testing.T) {
	key := SourceKey("Missing", "Song")
	unrelated := testTrack("one", "Artist", "Song")
	s := Service{}
	s.index = buildIndex([]Scrobble{{Artist: "Missing", Title: "Song", PlayedAtUTC: time.Unix(3, 0)}}, []Match{{SourceKey: key, Status: "no_match", Provenance: "agent"}}, []SpotifyMetadata{{URI: "spotify:track:other", Name: "Other", Artists: []string{"Someone"}}})
	s.resolve([]library.Track{unrelated})
	added := testTrack("two", "New", "Track")
	relinked := unrelated
	relinked.SpotifyURI = "spotify:track:other"
	s.resolve([]library.Track{relinked, added})
	if s.index.Matches[key].Status != "no_match" {
		t.Fatalf("no-match dropped by unrelated change: %#v", s.index.Matches[key])
	}
	if s.Status().Unresolved != 0 {
		t.Fatal("kept no-match counted as unresolved")
	}
}

func TestNoMatchReopensWhenCatalogueGainsItsIdentity(t *testing.T) {
	key := SourceKey("Missing", "Song")
	scrobbles := []Scrobble{{Artist: "Missing", Title: "Song", PlayedAtUTC: time.Unix(3, 0)}}
	noMatch := []Match{{SourceKey: key, Status: "no_match", Provenance: "agent"}}
	base := []library.Track{testTrack("one", "Artist", "Song")}

	s := Service{}
	s.index = buildIndex(scrobbles, noMatch, nil)
	s.resolve(append(base, testTrack("two", "MISSING", "song!")))
	if m := s.index.Matches[key]; m.Status != "match" || m.TrackID != "two" {
		t.Fatalf("same normalized identity did not reopen and auto-match: %#v", m)
	}

	s = Service{}
	s.index = buildIndex(scrobbles, noMatch, nil)
	s.resolve(append(base, testTrack("two", "Missing", "Song"), testTrack("three", "Missing", "Song")))
	if _, ok := s.index.Matches[key]; ok || s.Status().Unresolved != 1 {
		t.Fatalf("ambiguous identity should be unresolved again: %#v", s.index.Matches[key])
	}

	linked := testTrack("two", "Local", "Name")
	linked.SpotifyURI = "spotify:track:two"
	s = Service{}
	s.index = buildIndex(scrobbles, noMatch, []SpotifyMetadata{{URI: linked.SpotifyURI, Name: "Song", Artists: []string{"Missing"}}})
	s.resolve(append(base, linked))
	if m := s.index.Matches[key]; m.Status != "match" || m.TrackID != "two" {
		t.Fatalf("Spotify alias did not reopen and auto-match: %#v", m)
	}
}

func TestSpotifyAliasMatchesUniquelyAndAttachDoesNotAlterHistory(t *testing.T) {
	track := testTrack("one", "Local", "Name")
	track.SpotifyURI = "spotify:track:1"
	history := library.History{PlayedCount: 7}
	track.History = history
	s := Service{}
	s.index = buildIndex([]Scrobble{{Artist: "Canonical", Title: "Hit", PlayedAtUTC: time.Unix(4, 0)}}, nil, []SpotifyMetadata{{URI: track.SpotifyURI, Name: "Hit", Artists: []string{"Canonical"}}})
	s.resolve([]library.Track{track})
	attached := s.Attach([]library.Track{track})
	if attached[0].LastFM.PlayedCount != 1 || attached[0].History.PlayedCount != 7 {
		t.Fatalf("attached=%#v", attached[0])
	}
}

func TestLocalAndSpotifyAliasesAreDeduplicatedByTrackAndCollisionsStayUnresolved(t *testing.T) {
	identity := Scrobble{Artist: "Artist", Title: "Song", PlayedAtUTC: time.Unix(1, 0)}
	one, two := testTrack("one", "Artist", "Song"), testTrack("two", "Other", "Title")
	one.SpotifyURI = "spotify:track:one"
	two.SpotifyURI = "spotify:track:two"
	s := Service{}
	s.index = buildIndex([]Scrobble{identity}, nil, []SpotifyMetadata{{URI: one.SpotifyURI, Name: "Song", Artists: []string{"Artist"}}})
	s.resolve([]library.Track{one, two})
	if s.index.Matches[SourceKey("Artist", "Song")].TrackID != "one" {
		t.Fatal("same-track aliases were treated as ambiguous")
	}
	s = Service{}
	s.index = buildIndex([]Scrobble{identity}, nil, []SpotifyMetadata{{URI: two.SpotifyURI, Name: "Song", Artists: []string{"Artist"}}})
	s.resolve([]library.Track{one, two})
	if _, ok := s.index.Matches[SourceKey("Artist", "Song")]; ok {
		t.Fatal("cross-track alias collision matched")
	}
}

func TestResetAgentDecisionsRetainsAutomaticMatches(t *testing.T) {
	tracks := []library.Track{testTrack("one", "A", "B")}
	autoKey, agentKey := SourceKey("A", "B"), SourceKey("X", "Y")
	s := Service{DataDirectory: t.TempDir()}
	s.index = buildIndex([]Scrobble{{Artist: "A", Title: "B", PlayedAtUTC: day(1)}, {Artist: "X", Title: "Y", PlayedAtUTC: day(2)}}, []Match{{SourceKey: autoKey, Status: "match", TrackID: "one", Provenance: "auto"}, {SourceKey: agentKey, Status: "no_match", Provenance: "agent"}}, nil)
	if err := s.ResetAgentDecisions(tracks); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.index.Matches[agentKey]; ok {
		t.Fatal("agent decision retained")
	}
	if s.index.Matches[autoKey].TrackID != "one" {
		t.Fatal("automatic match removed")
	}
}

func TestFingerprintAndCaseIDAreDeterministic(t *testing.T) {
	a := []library.Track{testTrack("b", "B", "T"), testTrack("a", "A", "T")}
	b := []library.Track{a[1], a[0]}
	if CatalogueFingerprint(a) != CatalogueFingerprint(b) {
		t.Fatal("fingerprint depends on input order")
	}
	if CaseID("key") != CaseID("key") || CaseID("key") == CaseID("other") {
		t.Fatal("case IDs are not stable")
	}
}

func TestCacheLoadOfflineAndMalformedOwnedFile(t *testing.T) {
	dir := t.TempDir()
	_ = writeScrobbles(filepath.Join(dir, ScrobblesFile), []Scrobble{{Artist: "A", Title: "B", PlayedAtUTC: time.Unix(1, 0)}})
	s := Service{DataDirectory: dir}
	tracks, err := s.Load([]library.Track{testTrack("x", "A", "B")})
	if err != nil || tracks[0].LastFM.PlayedCount != 1 {
		t.Fatalf("tracks=%#v err=%v", tracks, err)
	}
	if err := os.WriteFile(filepath.Join(dir, MatchesFile), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Load(tracks); err == nil {
		t.Fatal("expected malformed cache error")
	}
}

func testTrack(id, artist, title string) library.Track {
	date := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	return library.Track{ID: id, Artist: artist, Title: title, ReleaseDate: date, ReleaseDateLabel: "2025-01-01", BaseSearchText: artist + " " + title, Variants: []library.Variant{{ID: "v-" + id, TrackID: id, Filename: id + ".mkv", VideoPath: id + ".mkv", Category: library.MusicVideo, Date: date, DateLabel: "2025-01-01"}}}
}
