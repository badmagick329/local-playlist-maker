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

func TestCreditsVideoTagsAndTrackNumbersAreIgnored(t *testing.T) {
	boombayah := func() []library.Track {
		album, single := testTrack("album", "BLACKPINK", "BOOMBAYAH"), testTrack("single", "BLACKPINK", "BOOMBAYAH")
		album.SpotifyURI, single.SpotifyURI = "spotify:track:b", "spotify:track:b"
		album.Variants = append(album.Variants, album.Variants[0])
		return []library.Track{album, single}
	}
	cases := []struct {
		artist, title string
		tracks        []library.Track
		want          string
	}{
		{"HYO", "DESSERT (feat. Loopy, SOYEON ((G)I-DLE))", []library.Track{testTrack("dessert", "HYO", "DESSERT")}, "dessert"},
		{"bts feat. halsey", "boy with luv", []library.Track{testTrack("luv", "BTS", "Boy With Luv")}, "luv"},
		{"Red Velvet", "05. Oh Boy", []library.Track{testTrack("oh", "Red Velvet", "Oh Boy")}, "oh"},
		{"Davichi", "4- 봐 (Look)", []library.Track{testTrack("look", "Davichi", "봐 (Look)")}, "look"},
		{"momoland, CHROMANCE", "Wrap Me In Plastic (Official Video)", []library.Track{testTrack("wrap", "MOMOLAND, CHROMANCE", "Wrap Me In Plastic")}, "wrap"},
		{"마마무", "눈누난나 (Nun Nu Nan Na) M/V", []library.Track{testTrack("nun", "마마무", "눈누난나 (Nun Nu Nan Na)")}, "nun"},
		{"BLACKPINK", "BOOMBAYAH -KR ver.-", boombayah(), "album"},
		{"T-ara", "MR.TAXI (Korean Ver.)", []library.Track{testTrack("jp", "T-ara", "MR.TAXI"), testTrack("kr", "T-ara", "MR.TAXI - Korean Version")}, "kr"},
	}
	for _, c := range cases {
		if m, ok := resolved(t, c.artist, c.title, c.tracks); !ok || m.TrackID != c.want {
			t.Errorf("%s – %s: got %#v, want %s", c.artist, c.title, m, c.want)
		}
	}
	if m, ok := resolved(t, "T-ara", "MR.TAXI (Korean Ver.)", []library.Track{testTrack("jp", "T-ara", "MR.TAXI")}); ok {
		t.Fatalf("Korean version matched the Japanese recording: %#v", m)
	}
	if m, ok := resolved(t, "IU", "With You (Official Video)", []library.Track{testTrack("with", "IU", "With You"), testTrack("you", "IU", "You")}); !ok || m.TrackID != "with" {
		t.Fatalf("a title starting with a credit word lost its words: %#v", m)
	}
}

func TestTitlesSpelledInTwoScriptsMatchEitherSpelling(t *testing.T) {
	cases := []struct {
		artist, title string
		tracks        []library.Track
		want          string
	}{
		{"TAEYEON", "불티 (Spark)", []library.Track{testTrack("spark", "TAEYEON", "Spark")}, "spark"},
		{"aespa", "자각몽 Lucid Dream", []library.Track{testTrack("lucid", "aespa", "Lucid Dream")}, "lucid"},
		{"SUNMI", "pporappippam 보라빛 밤", []library.Track{testTrack("ppo", "SUNMI", "pporappippam")}, "ppo"},
		{"PSY", "강남스타일", []library.Track{testTrack("gangnam", "PSY", "Gangnam Style (강남스타일)")}, "gangnam"},
		{"f(x)", "피노키오 (Danger)", []library.Track{testTrack("pino", "f(x)", "Pinocchio (Danger)")}, "pino"},
		{"Red Velvet (레드벨벳)", "Umpah Umpah", []library.Track{testTrack("umpah", "Red Velvet", "Umpah Umpah")}, "umpah"},
	}
	for _, c := range cases {
		if m, ok := resolved(t, c.artist, c.title, c.tracks); !ok || m.TrackID != c.want {
			t.Errorf("%s – %s: got %#v, want %s", c.artist, c.title, m, c.want)
		}
	}
	unresolved := []struct {
		name, artist, title string
		tracks              []library.Track
	}{
		{"version brackets are not translations", "STAYC", "POPPY", []library.Track{testTrack("kr", "STAYC", "Poppy (Korean Ver.)"), testTrack("jp", "STAYC", "POPPY - Japanese Ver.")}},
		{"single-script titles do not split", "TAEYEON", "I Got Love", []library.Track{testTrack("i", "TAEYEON", "I")}},
		{"a version run is not a translation", "Davichi", "사랑 Live", []library.Track{testTrack("love", "Davichi", "Live")}},
		{"same-script artist brackets do not split", "Jennie (BLACKPINK)", "SOLO", []library.Track{testTrack("solo", "BLACKPINK", "SOLO")}},
	}
	for _, c := range unresolved {
		if m, ok := resolved(t, c.artist, c.title, c.tracks); ok {
			t.Errorf("%s: %s – %s matched %#v", c.name, c.artist, c.title, m)
		}
	}
}
