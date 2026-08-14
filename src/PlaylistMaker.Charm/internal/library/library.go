package library

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
)

type Category string

const (
	MusicVideo   Category = "Music Video"
	BandLive     Category = "Band Live"
	Performance  Category = "Performance"
	Choreography Category = "Choreography"
	Relay        Category = "Relay"
	BeOriginal   Category = "Be Original"
	Fancam       Category = "Fancam"
	Concert      Category = "Concert"
	MusicShow    Category = "Music Show"
	Remix        Category = "Remix"
	LiveAudio    Category = "Live Audio"
)

var Categories = []Category{
	MusicVideo,
	BandLive,
	Performance,
	Choreography,
	Relay,
	BeOriginal,
	Fancam,
	Concert,
	MusicShow,
	Remix,
	LiveAudio,
}

type Variant struct {
	ID         string
	VideoPath  string
	AudioPath  string
	Filename   string
	Category   Category
	Date       time.Time
	DateLabel  string
	ModifiedAt time.Time
	History    History
}

type Track struct {
	ID                   string
	Artist               string
	Title                string
	ReleaseDate          time.Time
	ReleaseDateLabel     string
	ModifiedAt           time.Time
	Variants             []Variant
	BaseSearchText       string
	SearchTextByCategory map[Category]string
	NewestVideoDate      time.Time
	History              History
}

type History struct {
	PlayedCount     int
	CompletedCount  int
	StoppedCount    int
	SkippedCount    int
	LastPlayedAtUTC *time.Time
}

type Sort int

const (
	ModifiedNewest Sort = iota
	ArtistAscending
	TitleAscending
	ReleaseNewest
	VideoNewest
)

func (s Sort) String() string {
	switch s {
	case ArtistAscending:
		return "Artist A-Z"
	case TitleAscending:
		return "Title A-Z"
	case ReleaseNewest:
		return "Track release newest"
	case VideoNewest:
		return "Video date newest"
	default:
		return "Modified newest"
	}
}

var Sorts = []Sort{ModifiedNewest, ArtistAscending, TitleAscending, ReleaseNewest, VideoNewest}

func Generate(trackCount, variantCount int) []Track {
	artists := []string{
		"aespa", "KiiiKiii", "T-ARA", "Billlie", "KATSEYE", "IVE", "LE SSERAFIM",
		"BABYMONSTER", "Red Velvet", "NewJeans", "STAYC", "fromis_9", "LOONA",
		"Dreamcatcher", "ITZY", "TWICE", "NMIXX", "PURPLE KISS", "tripleS", "XG",
	}
	titles := []string{
		"KISS N TELL", "Pop Off Pop Off", "Roly-Poly", "WORK", "Animal", "BANG BANG",
		"ICONIC BY MISTAKE", "SUGAR HONEY ICE TEA", "Cosmic", "Cool With You", "BEBE",
		"Attitude", "Singing in the Rain", "Justice", "Imaginary Friend", "Strategy",
		"KNOW ABOUT ME", "Sweet Juice", "Girls Never Die", "SHOOTING STAR",
	}

	tracks := make([]Track, trackCount)
	base := variantCount / trackCount
	extra := variantCount % trackCount
	now := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)

	for i := range tracks {
		count := base
		if i < extra {
			count++
		}
		artist := artists[i%len(artists)]
		title := titles[(i*7+i/len(artists))%len(titles)]
		if i >= len(artists) {
			title = fmt.Sprintf("%s %03d", title, i/len(artists)+1)
		}
		release := now.AddDate(0, 0, -(i*17)%3500)
		modified := now.Add(-time.Duration((i*29)%20000) * time.Minute)
		variants := make([]Variant, count)
		newestVideoDate := time.Time{}
		searchTextByCategory := make(map[Category]string, len(Categories))
		for j := range variants {
			category := Categories[(i+j*3)%len(Categories)]
			if j == 0 {
				category = MusicVideo
			}
			date := release.AddDate(0, 0, (j*11+i)%180)
			filename := fmt.Sprintf("%s - %s (%s %02d).mkv", artist, title, category, j+1)
			variants[j] = Variant{
				ID:         fmt.Sprintf("video-%04d-%02d", i, j),
				VideoPath:  fmt.Sprintf("synthetic/video-%04d-%02d.mkv", i, j),
				AudioPath:  fmt.Sprintf("synthetic/audio-%04d.flac", i),
				Filename:   filename,
				Category:   category,
				Date:       date,
				DateLabel:  date.Format("2006-01-02"),
				ModifiedAt: modified.Add(time.Duration(j) * time.Second),
			}
			searchTextByCategory[category] += " " + normalize(filename)
			if date.After(newestVideoDate) {
				newestVideoDate = date
			}
		}
		tracks[i] = Track{
			ID:                   fmt.Sprintf("track-%04d", i),
			Artist:               artist,
			Title:                title,
			ReleaseDate:          release,
			ReleaseDateLabel:     release.Format("2006-01-02"),
			ModifiedAt:           modified,
			Variants:             variants,
			BaseSearchText:       normalize(artist + " " + title),
			SearchTextByCategory: searchTextByCategory,
			NewestVideoDate:      newestVideoDate,
		}
	}

	return tracks
}

func FilterAndSort(all []Track, query string, enabled map[Category]bool, order Sort) []Track {
	normalizedQuery := normalize(query)
	tokens := strings.Fields(normalizedQuery)
	type scored struct {
		track Track
		score int
	}
	matches := make([]scored, 0, len(all))

	for _, track := range all {
		if !hasEnabledVariant(track, enabled) {
			continue
		}
		candidate := track.BaseSearchText
		for category, categorySearchText := range track.SearchTextByCategory {
			if enabled[category] {
				candidate += categorySearchText
			}
		}
		score, ok := fuzzyScore(candidate, tokens)
		if !ok {
			continue
		}
		matches = append(matches, scored{track: track, score: score})
	}

	sort.SliceStable(matches, func(i, j int) bool {
		left, right := matches[i], matches[j]
		if normalizedQuery != "" && left.score != right.score {
			return left.score > right.score
		}
		switch order {
		case ArtistAscending:
			if value := strings.Compare(strings.ToLower(left.track.Artist), strings.ToLower(right.track.Artist)); value != 0 {
				return value < 0
			}
		case TitleAscending:
			if value := strings.Compare(strings.ToLower(left.track.Title), strings.ToLower(right.track.Title)); value != 0 {
				return value < 0
			}
		case ReleaseNewest:
			if !left.track.ReleaseDate.Equal(right.track.ReleaseDate) {
				return left.track.ReleaseDate.After(right.track.ReleaseDate)
			}
		case VideoNewest:
			if !left.track.NewestVideoDate.Equal(right.track.NewestVideoDate) {
				return left.track.NewestVideoDate.After(right.track.NewestVideoDate)
			}
		default:
			if !left.track.ModifiedAt.Equal(right.track.ModifiedAt) {
				return left.track.ModifiedAt.After(right.track.ModifiedAt)
			}
		}
		return left.track.ID < right.track.ID
	})

	result := make([]Track, len(matches))
	for i := range matches {
		result[i] = matches[i].track
	}
	return result
}

func EligibleVariants(track Track, enabled map[Category]bool) []Variant {
	result := make([]Variant, 0, len(track.Variants))
	for _, variant := range track.Variants {
		if enabled[variant.Category] {
			result = append(result, variant)
		}
	}
	return result
}

func DefaultVariant(track Track, enabled map[Category]bool) (Variant, bool) {
	eligible := EligibleVariants(track, enabled)
	if len(eligible) == 0 {
		return Variant{}, false
	}
	best := eligible[0]
	for _, candidate := range eligible[1:] {
		candidateOfficial := candidate.Category == MusicVideo
		bestOfficial := best.Category == MusicVideo
		if candidateOfficial != bestOfficial {
			if candidateOfficial {
				best = candidate
			}
			continue
		}
		if candidate.Date.After(best.Date) ||
			(candidate.Date.Equal(best.Date) && candidate.ModifiedAt.After(best.ModifiedAt)) ||
			(candidate.Date.Equal(best.Date) && candidate.ModifiedAt.Equal(best.ModifiedAt) && candidate.ID < best.ID) {
			best = candidate
		}
	}
	return best, true
}

func hasEnabledVariant(track Track, enabled map[Category]bool) bool {
	for _, variant := range track.Variants {
		if enabled[variant.Category] {
			return true
		}
	}
	return false
}

func normalize(value string) string {
	return strings.ToLower(strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsSpace(r) {
			return r
		}
		return ' '
	}, value))
}

func fuzzyScore(candidate string, tokens []string) (int, bool) {
	if len(tokens) == 0 {
		return 0, true
	}
	total := 0
	for _, token := range tokens {
		if index := strings.Index(candidate, token); index >= 0 {
			total += 1000 - min(index, 900) + len(token)*8
			continue
		}
		score, ok := subsequenceScore(candidate, token)
		if !ok {
			return 0, false
		}
		total += score
	}
	return total, true
}

func subsequenceScore(candidate, token string) (int, bool) {
	candidateRunes := []rune(candidate)
	position := 0
	previous := -2
	score := 0
	for _, wanted := range token {
		found := false
		for position < len(candidateRunes) {
			current := candidateRunes[position]
			position++
			if current != wanted {
				continue
			}
			index := position - 1
			score += 20
			if index == previous+1 {
				score += 12
			}
			previous = index
			found = true
			break
		}
		if !found {
			return 0, false
		}
	}
	return score, true
}
