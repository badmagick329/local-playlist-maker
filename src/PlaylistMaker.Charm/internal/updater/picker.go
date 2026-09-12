package updater

import (
	"context"
	"sort"
	"strings"

	"playlistmaker/charm/internal/catalog"
	"playlistmaker/charm/internal/library"
	"playlistmaker/charm/internal/metadata"
	"playlistmaker/charm/internal/pathid"
)

type Audio struct {
	Path        string
	Artist      string
	Title       string
	Album       string
	ReleaseDate string
	Source      string
	Kind        AudioKind
	VideoCount  int
	SpotifyOnly bool
}

type AudioKind int

const (
	CatalogueAudio AudioKind = iota
	UnlinkedAudio
	UnusedAudio
)

func (s Service) Search(ctx context.Context, query string) ([]Audio, error) {
	media, err := catalog.Read(s.Config.MediaCatalogFile)
	if err != nil {
		return nil, err
	}
	cache, err := metadata.ReadCache(s.Config.FlacCacheFile)
	if err != nil {
		return nil, err
	}
	paths, err := s.discoverAudio(ctx)
	if err != nil {
		return nil, err
	}
	discovered := make(map[string]bool, len(paths))
	for _, path := range paths {
		discovered[pathid.ComparisonKey(path)] = true
	}
	videos := make(map[string]int)
	for _, video := range media.Videos {
		videos[video.TrackID]++
	}
	candidates := []Audio{}
	claimed := make(map[string]bool, len(media.Tracks))
	for _, track := range media.Tracks {
		if track.LocalAudioPath != "" {
			claimed[pathid.ComparisonKey(track.LocalAudioPath)] = true
		}
		if missingAudio(track.LocalAudioPath) {
			continue
		}
		entry := cache[pathid.ComparisonKey(track.LocalAudioPath)]
		date := track.ReleaseDate
		if date == "" {
			date = entry.Date
		}
		source := track.LocalAudioPath
		if source == "" {
			source = track.SpotifyURI
		}
		if source == "" {
			source = track.ID
		}
		kind := CatalogueAudio
		if videos[track.ID] == 0 && track.LocalAudioPath == "" {
			kind = UnusedAudio
		}
		candidates = append(candidates, Audio{Kind: kind, VideoCount: videos[track.ID], SpotifyOnly: track.LocalAudioPath == "" && track.SpotifyURI != "", Path: track.ID, Artist: track.Artist, Title: track.Title, Album: entry.Album, ReleaseDate: date, Source: source})
	}
	for key, entry := range cache {
		if !claimed[key] && discovered[key] {
			candidates = append(candidates, Audio{Kind: UnlinkedAudio, Path: entry.FilePath, Artist: entry.Artist, Title: entry.Title, Album: entry.Album, ReleaseDate: entry.Date, Source: entry.FilePath})
		}
	}
	return FilterAudio(candidates, query), nil
}

// FilterAudio keeps typing independent of catalogue reads and filesystem checks.
func FilterAudio(candidates []Audio, query string) []Audio {
	type scoredAudio struct {
		audio   Audio
		score   int
		nameKey string
		pathKey string
	}
	matches := make([]scoredAudio, 0, len(candidates))
	for _, audio := range candidates {
		score, ok := library.FuzzyScore(audio.Artist+" "+audio.Title, query)
		if strings.TrimSpace(query) == "" || ok {
			matches = append(matches, scoredAudio{audio: audio, score: score, nameKey: normalize(audio.Artist + " " + audio.Title), pathKey: pathid.ComparisonKey(audio.Path)})
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].audio.Kind != matches[j].audio.Kind {
			return matches[i].audio.Kind < matches[j].audio.Kind
		}
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		left := matches[i].nameKey
		right := matches[j].nameKey
		if left != right {
			return left < right
		}
		a, b := matches[i].audio, matches[j].audio
		if a.ReleaseDate != b.ReleaseDate {
			if a.ReleaseDate == "" {
				return false
			}
			if b.ReleaseDate == "" {
				return true
			}
			return a.ReleaseDate < b.ReleaseDate
		}
		if a.Album != b.Album {
			return a.Album < b.Album
		}
		return matches[i].pathKey < matches[j].pathKey
	})
	result := make([]Audio, len(matches))
	for index := range matches {
		result[index] = matches[index].audio
	}
	return result
}
