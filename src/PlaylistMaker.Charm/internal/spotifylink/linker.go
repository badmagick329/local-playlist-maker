package spotifylink

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"playlistmaker/charm/internal/catalog"
	"playlistmaker/charm/internal/metadata"
	"playlistmaker/charm/internal/pathid"
	"playlistmaker/charm/internal/spotify"
	"playlistmaker/charm/internal/videoname"
)

type Candidate struct {
	URI              string
	Artist           string
	Title            string
	Album            string
	ReleaseDate      string
	DurationMS       int
	ReleaseDateMatch bool
}

type Item struct {
	TrackID     string
	Artist      string
	Title       string
	Album       string
	ReleaseDate string
	Candidates  []Candidate
	Reason      string
}

type ScanResult struct {
	Items      []Item
	AutoLinked int
}

type ScanProgress struct {
	Phase       string
	Current     int
	Total       int
	AutoLinked  int
	ReviewCount int
	Artist      string
	Title       string
}

type Service struct {
	CatalogPath string
	CachePath   string
	Auth        *spotify.Auth
	Client      *spotify.Client
	Wait        func(context.Context, time.Duration) error
}

func (s Service) Scan(ctx context.Context) (ScanResult, error) {
	return s.ScanWithProgress(ctx, nil)
}

func (s Service) ScanWithProgress(ctx context.Context, report func(ScanProgress)) (ScanResult, error) {
	if s.Auth == nil || s.Client == nil || strings.TrimSpace(s.Auth.ClientID) == "" {
		return ScanResult{}, fmt.Errorf("Spotify is not configured")
	}
	if report != nil {
		report(ScanProgress{Phase: "authenticating"})
	}
	if _, err := s.Auth.AccessToken(ctx, true); err != nil {
		return ScanResult{}, err
	}
	media, err := catalog.Read(s.CatalogPath)
	if err != nil {
		return ScanResult{}, err
	}
	cache, err := metadata.ReadCache(s.CachePath)
	if err != nil {
		return ScanResult{}, err
	}
	total := 0
	for _, track := range media.Tracks {
		if track.SpotifyURI == "" && !track.SpotifyIgnored {
			total++
		}
	}
	if report != nil {
		report(ScanProgress{Phase: "scanning", Total: total})
	}
	result := ScanResult{}
	dirty := 0
	current := 0
	flush := func() error {
		if dirty == 0 {
			return nil
		}
		if err := catalog.Write(s.CatalogPath, media); err != nil {
			return err
		}
		dirty = 0
		return nil
	}
	fail := func(scanErr error) (ScanResult, error) {
		if err := flush(); err != nil {
			return result, err
		}
		return result, scanErr
	}
	for index := range media.Tracks {
		track := &media.Tracks[index]
		if track.SpotifyURI != "" || track.SpotifyIgnored {
			continue
		}
		current++
		if report != nil {
			report(ScanProgress{Phase: "scanning", Current: current, Total: total, AutoLinked: result.AutoLinked, ReviewCount: len(result.Items), Artist: track.Artist, Title: track.Title})
		}
		cached := cache[pathid.ComparisonKey(track.LocalAudioPath)]
		item := Item{TrackID: track.ID, Artist: track.Artist, Title: track.Title, Album: cached.Album, ReleaseDate: track.ReleaseDate}
		if cached.ISRC != "" {
			matches, err := retryRateLimit(ctx, s.Wait, func() ([]spotify.Track, error) {
				return s.Client.Search(ctx, "isrc:"+cached.ISRC, 10)
			})
			if err != nil {
				return fail(err)
			}
			item.Candidates = candidates(matches)
			if len(item.Candidates) == 1 {
				track.SpotifyURI = item.Candidates[0].URI
				result.AutoLinked++
				dirty++
				if dirty >= 25 {
					if err := flush(); err != nil {
						return result, err
					}
				}
				continue
			}
			if len(item.Candidates) > 1 {
				item.Reason = "Multiple ISRC matches require confirmation"
				result.Items = append(result.Items, item)
				continue
			}
		}
		matches, err := retryRateLimit(ctx, s.Wait, func() ([]spotify.Track, error) {
			return s.Client.Search(ctx, fmt.Sprintf("artist:%q track:%q", track.Artist, track.Title), 10)
		})
		if err != nil {
			return fail(err)
		}
		exact := exactMatches(track.Artist, track.Title, matches)
		if len(exact) == 1 {
			track.SpotifyURI = exact[0].URI
			result.AutoLinked++
			dirty++
			if dirty >= 25 {
				if err := flush(); err != nil {
					return result, err
				}
			}
			continue
		}
		if len(exact) > 1 {
			item.Candidates = prioritizeReleaseDate(track.ReleaseDate, exact)
			item.Reason = "Multiple exact editions require confirmation"
		} else {
			item.Candidates = ranked(track.Artist, track.Title, track.ReleaseDate, matches)
			item.Reason = "Fuzzy suggestions require confirmation"
		}
		result.Items = append(result.Items, item)
	}
	if err := flush(); err != nil {
		return result, err
	}
	return result, nil
}

func (s Service) Search(ctx context.Context, query string) ([]Candidate, error) {
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}
	tracks, err := retryRateLimit(ctx, s.Wait, func() ([]spotify.Track, error) {
		return s.Client.Search(ctx, query, 10)
	})
	if err != nil {
		return nil, err
	}
	return candidates(tracks), nil
}

func (s Service) Validate(ctx context.Context, value string) (Candidate, error) {
	track, err := retryRateLimit(ctx, s.Wait, func() (spotify.Track, error) {
		return s.Client.Track(ctx, value)
	})
	if err != nil {
		return Candidate{}, err
	}
	return candidate(track), nil
}

func (s Service) Confirm(trackID, spotifyURI string) error {
	media, err := catalog.Read(s.CatalogPath)
	if err != nil {
		return err
	}
	found := false
	for index := range media.Tracks {
		if media.Tracks[index].ID == trackID {
			media.Tracks[index].SpotifyURI, media.Tracks[index].SpotifyIgnored = spotifyURI, false
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("catalogue track %q no longer exists", trackID)
	}
	return catalog.Write(s.CatalogPath, media)
}

func (s Service) Ignore(trackID string) error {
	media, err := catalog.Read(s.CatalogPath)
	if err != nil {
		return err
	}
	for index := range media.Tracks {
		if media.Tracks[index].ID == trackID {
			media.Tracks[index].SpotifyURI, media.Tracks[index].SpotifyIgnored = "", true
			return catalog.Write(s.CatalogPath, media)
		}
	}
	return fmt.Errorf("catalogue track %q no longer exists", trackID)
}

func candidates(values []spotify.Track) []Candidate {
	result := make([]Candidate, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		if value.URI == "" || value.IsPlayable != nil && !*value.IsPlayable || seen[value.URI] {
			continue
		}
		seen[value.URI] = true
		result = append(result, candidate(value))
	}
	return result
}

func candidate(value spotify.Track) Candidate {
	artists := make([]string, len(value.Artists))
	for index := range value.Artists {
		artists[index] = value.Artists[index].Name
	}
	return Candidate{URI: value.URI, Artist: strings.Join(artists, ", "), Title: value.Name, Album: value.Album.Name, ReleaseDate: value.Album.ReleaseDate, DurationMS: value.DurationMS}
}

func exactMatches(artist, title string, values []spotify.Track) []Candidate {
	artist, title = normalize(artist), normalize(title)
	result := []Candidate{}
	for _, value := range values {
		current := candidate(value)
		artistMatch := false
		for _, item := range value.Artists {
			artistMatch = artistMatch || normalize(item.Name) == artist
		}
		if artistMatch && normalize(value.Name) == title {
			result = append(result, current)
		}
	}
	return result
}

func ranked(artist, title, releaseDate string, values []spotify.Track) []Candidate {
	type scored struct {
		value     Candidate
		score     int
		dateMatch bool
	}
	items := []scored{}
	for _, value := range values {
		current := candidate(value)
		artistScore, artistOK := fuzzyScore(artist, current.Artist)
		titleScore, titleOK := fuzzyScore(title, current.Title)
		if artistOK || titleOK {
			current.ReleaseDateMatch = releaseDateMatches(releaseDate, current.ReleaseDate)
			items = append(items, scored{value: current, score: artistScore + titleScore, dateMatch: current.ReleaseDateMatch})
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].dateMatch != items[j].dateMatch {
			return items[i].dateMatch
		}
		return items[i].score > items[j].score
	})
	result := make([]Candidate, len(items))
	for index := range items {
		result[index] = items[index].value
	}
	return result
}

func prioritizeReleaseDate(releaseDate string, values []Candidate) []Candidate {
	result := append([]Candidate(nil), values...)
	for index := range result {
		result[index].ReleaseDateMatch = releaseDateMatches(releaseDate, result[index].ReleaseDate)
	}
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].ReleaseDateMatch && !result[j].ReleaseDateMatch
	})
	return result
}

func releaseDateMatches(left, right string) bool {
	left, right = strings.TrimSpace(left), strings.TrimSpace(right)
	if left == "" || right == "" {
		return false
	}
	if len(left) > len(right) {
		left, right = right, left
	}
	return strings.HasPrefix(right, left)
}

func fuzzyScore(left, right string) (int, bool) {
	return libraryFuzzy(left, right)
}

var libraryFuzzy = func(left, right string) (int, bool) {
	left, right = normalize(left), normalize(right)
	if left == right {
		return 1000, true
	}
	leftTokens := strings.Fields(left)
	score := 0
	for _, token := range leftTokens {
		if strings.Contains(right, token) {
			score += len(token)
		}
	}
	return score, score > 0
}

func normalize(value string) string { return videoname.Normalize(value) }

func retryRateLimit[T any](ctx context.Context, wait func(context.Context, time.Duration) error, operation func() (T, error)) (T, error) {
	value, err := operation()
	var rateLimit *spotify.RateLimitError
	if err == nil || !errors.As(err, &rateLimit) || !rateLimit.Valid || rateLimit.RetryAfter > 30*time.Second {
		return value, err
	}
	if wait == nil {
		wait = waitContext
	}
	if err := wait(ctx, rateLimit.RetryAfter); err != nil {
		var zero T
		return zero, err
	}
	return operation()
}

func waitContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
