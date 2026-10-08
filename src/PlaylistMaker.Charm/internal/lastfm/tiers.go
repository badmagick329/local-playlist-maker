package lastfm

import (
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	"playlistmaker/charm/internal/library"
)

// matchTier is one way of comparing an identity's names with the catalogue's.
// resolve tries the tiers in order and the first one with any hit decides, so
// a looser tier never overrides a stricter one and an ambiguous strict hit is
// not resolved by loosening further. Every tier answers under the one-song
// rule (songTrack).
type matchTier struct {
	reason string
	// catalogue returns the keys a catalogue spelling, local or Spotify,
	// answers to.
	catalogue func(artist, title string) []string
	// source returns the keys an identity looks up.
	source func(id *Identity) []string
	// reopensAmbiguous lets a hit on several songs reopen a no_match. Only the
	// exact tiers do: they define the names that fit an identity. A looser
	// tier reopens a no_match only when it finds one song, so a reviewed case
	// cannot bounce back to review on every import.
	reopensAmbiguous bool
}

func (s *Service) matchTiers() []matchTier {
	tiers := []matchTier{{
		reason:           "exact normalized alias",
		catalogue:        func(artist, title string) []string { return []string{SourceKey(artist, title)} },
		source:           func(id *Identity) []string { return []string{id.Key} },
		reopensAmbiguous: true,
	}}
	tiers = append(tiers, spaceless(tiers[0], "normalized alias ignoring spaces"))
	folded := matchTier{
		reason:    "normalized alias ignoring accents and character width",
		catalogue: func(artist, title string) []string { return []string{SourceKey(fold(artist), fold(title))} },
		source:    func(id *Identity) []string { return []string{SourceKey(fold(id.Artist), fold(id.Title))} },
	}
	return append(tiers, folded, spaceless(folded, "normalized alias ignoring accents, character width and spaces"))
}

// spaceless derives a tier whose keys ignore spaces, which it tries only after
// the spaced keys miss: "U-KISS" normalizes to "u kiss" and "UKISS" to "ukiss".
func spaceless(t matchTier, reason string) matchTier {
	strip := func(keys []string) []string {
		result := make([]string, len(keys))
		for i, k := range keys {
			result[i] = spacelessKey(k)
		}
		return result
	}
	return matchTier{
		reason:           reason,
		catalogue:        func(artist, title string) []string { return strip(t.catalogue(artist, title)) },
		source:           func(id *Identity) []string { return strip(t.source(id)) },
		reopensAmbiguous: t.reopensAmbiguous,
	}
}

// fold maps fullwidth and other compatibility forms to their plain letters and
// drops accents, so "Déjà Vu" meets "Deja Vu" and "ＡＢ" meets "AB". It stays
// inside matching: SourceKey and videoname.Normalize keep accents, so stored
// match keys and the other packages are unaffected.
func fold(value string) string {
	result, _, err := transform.String(transform.Chain(norm.NFKD, runes.Remove(runes.In(unicode.Mn)), norm.NFC), value)
	if err != nil {
		return value
	}
	return result
}

// tierLookup indexes the catalogue under one tier's keys.
func tierLookup(t matchTier, tracks []library.Track, spotify map[string]SpotifyMetadata) map[string]map[string]bool {
	lookup := map[string]map[string]bool{}
	add := func(artist, title, trackID string) {
		for _, key := range t.catalogue(artist, title) {
			if lookup[key] == nil {
				lookup[key] = map[string]bool{}
			}
			lookup[key][trackID] = true
		}
	}
	for _, track := range tracks {
		add(track.Artist, track.Title, track.ID)
		if md, ok := spotify[track.SpotifyURI]; ok && track.SpotifyURI != "" {
			for _, artist := range md.Artists {
				add(artist, md.Name, track.ID)
			}
		}
	}
	return lookup
}
