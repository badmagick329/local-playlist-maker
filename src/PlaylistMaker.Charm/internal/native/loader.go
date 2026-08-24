package native

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"playlistmaker/charm/internal/backend"
	"playlistmaker/charm/internal/catalog"
	"playlistmaker/charm/internal/config"
	"playlistmaker/charm/internal/library"
	"playlistmaker/charm/internal/metadata"
	"playlistmaker/charm/internal/pathid"
	"playlistmaker/charm/internal/videoname"
)

type Loader struct {
	Config    config.Config
	TagReader metadata.Reader
	ReadOnly  bool
}

func (l Loader) Load(ctx context.Context) (backend.LibrarySnapshot, error) {
	if err := ctx.Err(); err != nil {
		return backend.LibrarySnapshot{}, err
	}
	media, err := catalog.Read(l.Config.MediaCatalogFile)
	if err != nil {
		return backend.LibrarySnapshot{}, err
	}
	audioPaths := make([]string, 0, len(media.Tracks))
	for _, track := range media.Tracks {
		if track.LocalAudioPath != "" {
			audioPaths = append(audioPaths, track.LocalAudioPath)
		}
	}
	if !l.ReadOnly {
		reader := l.TagReader
		if reader == nil {
			reader = metadata.FLACReader{}
		}
		if _, _, err := metadata.Ensure(ctx, l.Config.FlacCacheFile, audioPaths, reader); err != nil {
			return backend.LibrarySnapshot{}, err
		}
	}
	cache, err := metadata.ReadCache(l.Config.FlacCacheFile)
	if err != nil {
		return backend.LibrarySnapshot{}, err
	}
	return buildLibrary(media, cache)
}

func buildLibrary(media catalog.Catalog, cache map[string]metadata.Entry) (backend.LibrarySnapshot, error) {
	positions := make(map[string]int, len(media.Tracks))
	tracks := make([]library.Track, len(media.Tracks))
	for index, source := range media.Tracks {
		artist, title, releaseLabel := source.Artist, source.Title, source.ReleaseDate
		if cached, ok := cache[pathid.ComparisonKey(source.LocalAudioPath)]; ok {
			artist, title = cached.Artist, cached.Title
			if releaseLabel == "" {
				releaseLabel = cached.Date
			}
		}
		release, label, ok := dateLabel(releaseLabel)
		if releaseLabel != "" && !ok {
			return backend.LibrarySnapshot{}, fmt.Errorf("track %q has invalid release date %q", source.ID, releaseLabel)
		}
		tracks[index] = library.Track{
			ID: source.ID, Artist: artist, Title: title,
			LocalAudioPath: source.LocalAudioPath, SpotifyURI: source.SpotifyURI, SpotifyIgnored: source.SpotifyIgnored,
			ReleaseDate: release, ReleaseDateLabel: label, SearchTextByCategory: map[library.Category]string{},
		}
		positions[source.ID] = index
	}
	for _, video := range media.Videos {
		position, ok := positions[video.TrackID]
		if !ok {
			return backend.LibrarySnapshot{}, fmt.Errorf("video %q references missing track %q", video.Path, video.TrackID)
		}
		variant, err := makeVariant(video.Path, video.TrackID)
		if err != nil {
			return backend.LibrarySnapshot{}, err
		}
		tracks[position].Variants = append(tracks[position].Variants, variant)
	}
	result := tracks[:0]
	for index := range tracks {
		track := tracks[index]
		sort.SliceStable(track.Variants, func(i, j int) bool {
			left, right := track.Variants[i], track.Variants[j]
			if !left.Date.Equal(right.Date) {
				return left.Date.After(right.Date)
			}
			return pathid.ComparisonKey(left.VideoPath) < pathid.ComparisonKey(right.VideoPath)
		})
		for _, variant := range track.Variants {
			if variant.ModifiedAt.After(track.ModifiedAt) {
				track.ModifiedAt = variant.ModifiedAt
			}
			if variant.Date.After(track.NewestVideoDate) {
				track.NewestVideoDate = variant.Date
			}
			track.SearchTextByCategory[variant.Category] += " " + strings.ToLower(variant.Filename)
		}
		track.BaseSearchText = strings.ToLower(track.Artist + " " + track.Title)
		if len(track.Variants) > 0 {
			result = append(result, track)
		}
	}
	return backend.LibrarySnapshot{Tracks: result}, nil
}

func makeVariant(videoPath, trackID string) (library.Variant, error) {
	date, label, ok := videoDate(videoPath)
	if !ok {
		return library.Variant{}, fmt.Errorf("could not parse video date from %q", videoPath)
	}
	modified := time.Time{}
	if info, err := os.Stat(videoPath); err == nil {
		modified = info.ModTime().UTC()
	}
	return library.Variant{ID: videoPath, TrackID: trackID, VideoPath: videoPath, Filename: filepath.Base(videoPath), Category: classify(videoPath), Date: date, DateLabel: label, ModifiedAt: modified}, nil
}

func dateLabel(value string) (time.Time, string, bool) {
	if value == "" {
		return time.Time{}, "", true
	}
	for _, layout := range []string{"2006-01-02", "2006-01", "2006"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, value, true
		}
	}
	return time.Time{}, "", false
}

func videoDate(path string) (time.Time, string, bool) {
	value := videoname.Parse(path).Date
	switch len(value) {
	case 6:
		return dateLabel("20" + value[:2] + "-" + value[2:4] + "-" + value[4:])
	case 8:
		return dateLabel(value[:4] + "-" + value[4:6] + "-" + value[6:])
	default:
		return time.Time{}, "", false
	}
}

func classify(path string) library.Category {
	variant := strings.ToLower(videoname.Parse(path).Variant)
	switch {
	case strings.Contains(variant, "band live"):
		return library.BandLive
	case regexp.MustCompile(`^performance(?:\s+[1-9]\d*)?$`).MatchString(variant):
		return library.Performance
	case variant == "choreography":
		return library.Choreography
	case variant == "relay":
		return library.Relay
	case variant == "be original":
		return library.BeOriginal
	case strings.Contains(variant, "fancam"):
		return library.Fancam
	case strings.Contains(variant, "concert"):
		return library.Concert
	case strings.Contains(strings.ReplaceAll(path, "/", `\`), `Music\uhdkpop`):
		return library.MusicShow
	case variant == "remix" || variant == "areia remix":
		return library.Remix
	case strings.Contains(variant, "live audio"):
		return library.LiveAudio
	default:
		return library.MusicVideo
	}
}
