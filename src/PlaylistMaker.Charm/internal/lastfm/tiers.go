package lastfm

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	"playlistmaker/charm/internal/library"
	"playlistmaker/charm/internal/videoname"
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
	tiers = append(tiers, folded, spaceless(folded, "normalized alias ignoring accents, character width and spaces"))
	cleaned := matchTier{
		reason:    "normalized alias without credits, video tags or track numbers",
		catalogue: func(artist, title string) []string { return []string{cleanKey(artist, title)} },
		source:    func(id *Identity) []string { return []string{cleanKey(id.Artist, id.Title)} },
	}
	return append(tiers, cleaned, spaceless(cleaned, "normalized alias without credits, video tags, track numbers or spaces"))
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

var (
	creditStart   = regexp.MustCompile(`(?i)^\s*(feat|ft|featuring|with|prod)\b`)
	trailingFeat  = regexp.MustCompile(`(?i)\s+(feat|ft|featuring)\b.*$`)
	leadingNumber = regexp.MustCompile(`^\d{1,2}(\.|\s*-)\s+`)
	// Tags that mark an upload or a release of the original recording. Korean
	// and Japanese versions are not among them: the catalogue holds those as
	// separate tracks, and dropping the marker would merge two recordings.
	trailingTags = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\s*[\(\[]\s*official\s+(music\s+)?(video|audio|mv)\s*[\)\]]$`),
		regexp.MustCompile(`(?i)\s*[\(\[]\s*(m/v|mv)\s*[\)\]]$`),
		regexp.MustCompile(`(?i)\s+m/v$`),
		regexp.MustCompile(`(?i)\s*-\s*kr\.?\s*ver(\.|sion)?\s*-?$`),
		regexp.MustCompile(`(?i)\s*[\(\[-]?\s*bonus\s+track\s*[\)\]]?$`),
	}
)

// cleanKey keys the artist and title without the text scrobblers add and the
// catalogue lacks: feature credits, upload tags and album track numbers.
// "version" and "ver" become one word, so "Korean Ver." meets "Korean
// Version". It folds first, so it also covers the folded tier's hits.
func cleanKey(artist, title string) string {
	return SourceKey(cleanArtist(artist), cleanTitle(title))
}

func cleanArtist(artist string) string {
	artist = fold(artist)
	if cleaned := strings.TrimSpace(trailingFeat.ReplaceAllString(dropCredits(artist), "")); cleaned != "" {
		return cleaned
	}
	return artist
}

func cleanTitle(title string) string {
	title = fold(title)
	cleaned := strings.TrimSpace(trailingFeat.ReplaceAllString(dropCredits(title), ""))
	for before := ""; before != cleaned; {
		before = cleaned
		for _, tag := range trailingTags {
			cleaned = strings.TrimSpace(tag.ReplaceAllString(cleaned, ""))
		}
	}
	cleaned = leadingNumber.ReplaceAllString(cleaned, "")
	if videoname.Normalize(cleaned) == "" {
		return title
	}
	return strings.TrimSpace(strings.ReplaceAll(" "+videoname.Normalize(cleaned)+" ", " version ", " ver "))
}

// dropCredits removes bracketed credits such as "(feat. Loopy, SOYEON
// ((G)I-DLE))", nested brackets included.
func dropCredits(value string) string {
	var out strings.Builder
	for i := 0; i < len(value); {
		if value[i] == '(' || value[i] == '[' {
			if end := closingBracket(value, i); end > 0 && creditStart.MatchString(value[i+1:end]) {
				i = end + 1
				continue
			}
		}
		out.WriteByte(value[i])
		i++
	}
	return out.String()
}

// closingBracket returns the index of the bracket closing the one at open, or
// -1 when it is unbalanced.
func closingBracket(value string, open int) int {
	depth := 0
	for i := open; i < len(value); i++ {
		switch value[i] {
		case '(', '[':
			depth++
		case ')', ']':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
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
