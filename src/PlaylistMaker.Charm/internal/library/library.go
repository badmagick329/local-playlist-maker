package library

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"playlistmaker/charm/internal/videoname"
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
	PlayedCount        int
	CompletedCount     int
	StoppedCount       int
	SkippedCount       int
	NotStartedCount    int
	AbandonedCount     int
	LastPlayedAtUTC    *time.Time
	LastAttemptedAtUTC *time.Time
	Recent             []HistoryEvent
}

type HistoryEvent struct {
	Outcome string
	AtUTC   time.Time
	Percent *float64
}

type Sort int

const (
	ModifiedNewest Sort = iota
	ModifiedOldest
	ArtistAscending
	ArtistDescending
	TitleAscending
	TitleDescending
	ReleaseNewest
	ReleaseOldest
	VideoNewest
	VideoOldest
	Relevance
)

func (s Sort) String() string {
	switch s {
	case ModifiedOldest:
		return "Modified oldest"
	case ArtistAscending:
		return "Artist A-Z"
	case ArtistDescending:
		return "Artist Z-A"
	case TitleAscending:
		return "Title A-Z"
	case TitleDescending:
		return "Title Z-A"
	case ReleaseNewest:
		return "Track release newest"
	case ReleaseOldest:
		return "Track release oldest"
	case VideoNewest:
		return "Video date newest"
	case VideoOldest:
		return "Video date oldest"
	case Relevance:
		return "Relevance"
	default:
		return "Modified newest"
	}
}

var Sorts = []Sort{ModifiedNewest, ModifiedOldest, ArtistAscending, ArtistDescending, TitleAscending, TitleDescending, ReleaseNewest, ReleaseOldest, VideoNewest, VideoOldest, Relevance}

type DateRange struct {
	Label string
	Start time.Time
	End   time.Time
}

func (r DateRange) Contains(value time.Time) bool {
	return !value.Before(r.Start) && !value.After(r.End)
}

type Query struct {
	SearchText   string
	Enabled      map[Category]bool
	TrackRelease *DateRange
	VideoDate    *DateRange
	Sort         Sort
}

func ParseDateRange(value string) (*DateRange, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parts := strings.Split(value, "..")
	if len(parts) > 2 {
		return nil, fmt.Errorf("date range must use START..END")
	}
	if len(parts) == 1 {
		parts = []string{parts[0], parts[0]}
	}
	start, _, err := parseDateEndpoint(parts[0], false)
	if err != nil {
		return nil, err
	}
	end, _, err := parseDateEndpoint(parts[1], true)
	if err != nil {
		return nil, err
	}
	if start.After(end) {
		return nil, fmt.Errorf("range start is after its end")
	}
	return &DateRange{Label: value, Start: start, End: end}, nil
}
func parseDateEndpoint(value string, end bool) (time.Time, string, error) {
	layouts := []string{"2006-01-02", "2006-01", "2006"}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			switch layout {
			case "2006-01-02":
				if end {
					return parsed.AddDate(0, 0, 1).Add(-time.Nanosecond), value, nil
				}
			case "2006":
				if end {
					return parsed.AddDate(1, 0, 0).Add(-time.Nanosecond), value, nil
				}
			case "2006-01":
				if end {
					return parsed.AddDate(0, 1, 0).Add(-time.Nanosecond), value, nil
				}
			}
			return parsed, value, nil
		}
	}
	return time.Time{}, "", fmt.Errorf("invalid date %q", value)
}

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

func FilterAndSort(all []Track, query Query) []Track {
	normalizedQuery := normalize(query.SearchText)
	tokens := strings.Fields(normalizedQuery)
	type scored struct {
		track Track
		score int
	}
	matches := make([]scored, 0, len(all))

	for _, track := range all {
		if query.TrackRelease != nil && !query.TrackRelease.Contains(track.ReleaseDate) {
			continue
		}
		eligible := EligibleVariants(track, query)
		if len(eligible) == 0 {
			continue
		}
		candidate := track.BaseSearchText
		for _, variant := range eligible {
			candidate += " " + normalize(variant.Filename)
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
		switch query.Sort {
		case ArtistAscending, ArtistDescending:
			if value := strings.Compare(strings.ToLower(left.track.Artist), strings.ToLower(right.track.Artist)); value != 0 {
				return (value < 0) == (query.Sort == ArtistAscending)
			}
		case TitleAscending, TitleDescending:
			if value := strings.Compare(strings.ToLower(left.track.Title), strings.ToLower(right.track.Title)); value != 0 {
				return (value < 0) == (query.Sort == TitleAscending)
			}
		case ReleaseNewest, ReleaseOldest:
			if !left.track.ReleaseDate.Equal(right.track.ReleaseDate) {
				return left.track.ReleaseDate.After(right.track.ReleaseDate) == (query.Sort == ReleaseNewest)
			}
		case VideoNewest, VideoOldest, ModifiedNewest, ModifiedOldest, Relevance:
			leftDefault, _ := DefaultVariant(left.track, query)
			rightDefault, _ := DefaultVariant(right.track, query)
			leftValue, rightValue := leftDefault.ModifiedAt, rightDefault.ModifiedAt
			newest := query.Sort != ModifiedOldest
			if query.Sort == VideoNewest || query.Sort == VideoOldest {
				leftValue, rightValue = leftDefault.Date, rightDefault.Date
				newest = query.Sort == VideoNewest
			}
			if !leftValue.Equal(rightValue) {
				return leftValue.After(rightValue) == newest
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

func EligibleVariants(track Track, query Query) []Variant {
	result := make([]Variant, 0, len(track.Variants))
	for _, variant := range track.Variants {
		if query.Enabled[variant.Category] && (query.VideoDate == nil || query.VideoDate.Contains(variant.Date)) {
			result = append(result, variant)
		}
	}
	return result
}

func DefaultVariant(track Track, query Query) (Variant, bool) {
	eligible := EligibleVariants(track, query)
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

func normalize(value string) string {
	return videoname.Normalize(value)
}

// FuzzyScore uses the same fzf-style matcher as library search.
func FuzzyScore(candidate, query string) (int, bool) {
	return fuzzyScore(normalize(candidate), strings.Fields(normalize(query)))
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
