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

func matchTiers(aliases map[string][]string) []matchTier {
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
	tiers = append(tiers, cleaned, spaceless(cleaned, "normalized alias without credits, video tags, track numbers or spaces"))
	parts := func(artist, title string) []string {
		return partKeys(nameParts(cleanArtist(artist), true), nameParts(cleanTitle(title), false))
	}
	split := matchTier{
		reason:    "one spelling of a title or artist written in two scripts or with a bracketed translation",
		catalogue: parts,
		source:    func(id *Identity) []string { return parts(id.Artist, id.Title) },
	}
	tiers = append(tiers, split, spaceless(split, "one spelling of a title or artist written in two scripts or with a bracketed translation, ignoring spaces"))
	aliased := matchTier{
		reason:    "learned artist alias",
		catalogue: parts,
		source: func(id *Identity) []string {
			var artists []string
			for _, part := range nameParts(cleanArtist(id.Artist), true) {
				artists = append(artists, aliases[part]...)
			}
			return partKeys(artists, nameParts(cleanTitle(id.Title), false))
		},
	}
	return append(tiers, aliased, spaceless(aliased, "learned artist alias, ignoring spaces"))
}

// minimumAliasDecisions is how many reviewed matches, on different songs,
// must agree before a source artist becomes an alias of a catalogue artist.
// The owner chose two: one decision would turn a single wrong agent match
// into a rule for every song by that artist.
const minimumAliasDecisions = 2

// artistAliases learns which artist names mean the same act. Scrobbles
// spelled in both scripts, such as "Red Velvet (레드벨벳)", pair both names,
// so a scrobble credited only "레드벨벳" can find "Red Velvet". Reviewed
// matches teach names never scrobbled in both forms: two imported
// matches of "손담비" songs to Son Dam Bi tracks map 손담비 to Son Dam Bi.
// Automatic matches teach nothing, since they came from these aliases.
// Aliases are derived on every resolve and never stored. A name with two
// different partners is ambiguous, so none of its aliases is used.
func (s *Service) artistAliases(byID map[string]library.Track) map[string][]string {
	// partners maps a name to its partners keyed without spaces, so "Ji Yeon"
	// and "Jiyeon" do not conflict. Every partner counts against ambiguity,
	// but only a confirmed one becomes an alias.
	type partner struct {
		name      string
		decisions int
		confirmed bool
	}
	partners := map[string]map[string]*partner{}
	add := func(a, b string) *partner {
		if partners[a] == nil {
			partners[a] = map[string]*partner{}
		}
		if partners[a][spacelessKey(b)] == nil {
			partners[a][spacelessKey(b)] = &partner{name: b}
		}
		return partners[a][spacelessKey(b)]
	}
	for _, id := range s.index.Identities {
		if parts := nameParts(cleanArtist(id.Artist), true); len(parts) == 3 {
			add(parts[1], parts[2]).confirmed = true
			add(parts[2], parts[1]).confirmed = true
		}
	}
	for _, m := range s.index.Matches {
		track, ok := byID[m.TrackID]
		if m.Status != "match" || m.Provenance == "auto" || !ok {
			continue
		}
		source, target := matchName(cleanArtist(m.Artist)), matchName(cleanArtist(track.Artist))
		if spacelessKey(source) != spacelessKey(target) {
			p := add(source, target)
			p.decisions++
			p.confirmed = p.confirmed || p.decisions >= minimumAliasDecisions
		}
	}
	aliases := map[string][]string{}
	for name, values := range partners {
		if len(values) == 1 {
			for _, p := range values {
				if p.confirmed {
					aliases[name] = []string{p.name}
				}
			}
		}
	}
	return aliases
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
	return nameKey(matchName(cleanArtist(artist)), matchName(cleanTitle(title)))
}

func nameKey(artist, title string) string { return artist + "\x00" + title }

// matchName normalizes a cleaned name for the cleaned and later tiers.
func matchName(value string) string {
	return strings.TrimSpace(strings.ReplaceAll(" "+videoname.Normalize(value)+" ", " version ", " ver "))
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
	return cleaned
}

var (
	trailingBracket = regexp.MustCompile(`\s+[\(\[]([^()\[\]]*)[\)\]]$`)
	versionWord     = regexp.MustCompile(`\b(remix|ver|version|mix|edit|live|bootleg|acoustic|remaster|remastered|mashup|inst|mr)\b`)
	versionEnding   = regexp.MustCompile(`\b(remix|ver|version|mix|edit|live|bootleg|acoustic|remaster|remastered|mashup|inst|mr)$`)
)

// nameParts returns a cleaned name and, when it is spelled twice, each
// spelling: "불티 (Spark)" yields "불티" and "spark", "자각몽 Lucid Dream" yields
// "자각몽" and "lucid dream". A bracket holding a version word is a different
// recording, not a translation, so "Poppy (Korean Ver.)" stays whole, and a
// script run ending in one ("사랑 Live") does not split either. A single
// script never splits, so "I Got Love" cannot become "I". Artists split only
// across scripts ("Red Velvet (레드벨벳)"), so "Jennie (BLACKPINK)" stays whole.
func nameParts(value string, artist bool) []string {
	whole := matchName(value)
	parts := []string{whole}
	if match := trailingBracket.FindStringSubmatchIndex(value); match != nil {
		outside, inside := matchName(value[:match[0]]), matchName(value[match[2]:match[3]])
		if outside == "" || inside == "" || versionWord.MatchString(inside) || artist && !differentScripts(outside, inside) {
			return parts
		}
		return append(parts, outside, inside)
	}
	words := strings.Fields(whole)
	split := 0
	for i := 1; i < len(words); i++ {
		if scriptOf(words[i]) != scriptOf(words[i-1]) {
			if split > 0 {
				return parts
			}
			split = i
		}
	}
	if split == 0 {
		return parts
	}
	first, second := strings.Join(words[:split], " "), strings.Join(words[split:], " ")
	if !differentScripts(first, second) || versionEnding.MatchString(first) || versionEnding.MatchString(second) {
		return parts
	}
	return append(parts, first, second)
}

type script int

const (
	otherScript script = iota
	latinScript
	cjkScript
)

// scriptOf classes a normalized word as Latin or CJK (Hangul, Han, kana);
// digits and mixed words are neither, so they never split a name.
func scriptOf(word string) script {
	result := otherScript
	for _, r := range word {
		var current script
		switch {
		case unicode.In(r, unicode.Hangul, unicode.Han, unicode.Hiragana, unicode.Katakana):
			current = cjkScript
		case unicode.In(r, unicode.Latin):
			current = latinScript
		default:
			return otherScript
		}
		if result != otherScript && current != result {
			return otherScript
		}
		result = current
	}
	return result
}

// differentScripts reports whether one name is wholly Latin and the other
// wholly CJK.
func differentScripts(a, b string) bool {
	sa, sb := nameScript(a), nameScript(b)
	return sa != otherScript && sb != otherScript && sa != sb
}

func nameScript(name string) script {
	result := otherScript
	for i, word := range strings.Fields(name) {
		s := scriptOf(word)
		if s == otherScript || i > 0 && s != result {
			return otherScript
		}
		result = s
	}
	return result
}

// partKeys pairs every artist part with every title part.
func partKeys(artists, titles []string) []string {
	keys := make([]string, 0, len(artists)*len(titles))
	for _, a := range artists {
		for _, t := range titles {
			keys = append(keys, nameKey(a, t))
		}
	}
	return keys
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
