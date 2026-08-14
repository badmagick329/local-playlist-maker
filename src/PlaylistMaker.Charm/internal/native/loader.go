package native

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"playlistmaker/charm/internal/backend"
	"playlistmaker/charm/internal/config"
	"playlistmaker/charm/internal/library"
	"playlistmaker/charm/internal/pathid"
)

type Loader struct{ Config config.Config }
type cacheEntry struct {
	FilePath    string `json:"filePath"`
	Artist      string `json:"artist"`
	Title       string `json:"title"`
	Date        string `json:"date"`
	TrackNumber int    `json:"trackNumber"`
}
type trackBuild struct {
	track library.Track
	first int
}

func (l Loader) Load(ctx context.Context) (backend.LibrarySnapshot, error) {
	if err := ctx.Err(); err != nil {
		return backend.LibrarySnapshot{}, err
	}
	mappings, err := loadMappings(l.Config.MusicVideoToAudioMap)
	if err != nil {
		return backend.LibrarySnapshot{}, err
	}
	if err := ctx.Err(); err != nil {
		return backend.LibrarySnapshot{}, err
	}
	cache, err := loadCache(l.Config.FlacCacheFile)
	if err != nil {
		return backend.LibrarySnapshot{}, err
	}
	if err := ctx.Err(); err != nil {
		return backend.LibrarySnapshot{}, err
	}
	return buildLibrary(mappings, cache)
}

func loadCache(path string) (map[string]cacheEntry, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read FLAC cache %q: %w", path, err)
	}
	var entries []cacheEntry
	if err := json.Unmarshal(contents, &entries); err != nil {
		return nil, fmt.Errorf("parse FLAC cache %q: %w", path, err)
	}
	index := make(map[string]cacheEntry, len(entries))
	for indexInFile, entry := range entries {
		if strings.TrimSpace(entry.FilePath) == "" || strings.TrimSpace(entry.Artist) == "" || strings.TrimSpace(entry.Title) == "" {
			return nil, fmt.Errorf("FLAC cache %q contains invalid entry %d", path, indexInFile+1)
		}
		entry.FilePath = pathid.Normalize(entry.FilePath)
		index[pathid.ComparisonKey(entry.FilePath)] = entry
	}
	return index, nil
}

func buildLibrary(mappings []mappingEntry, cache map[string]cacheEntry) (backend.LibrarySnapshot, error) {
	missing := make([]string, 0)
	missingIndex := make(map[string]bool)
	tracks := make([]trackBuild, 0)
	trackPositions := make(map[string]int)
	for entryIndex, entry := range mappings {
		metadata, ok := cache[pathid.ComparisonKey(entry.audioPath)]
		if !ok {
			key := pathid.ComparisonKey(entry.audioPath)
			if !missingIndex[key] {
				missingIndex[key] = true
				missing = append(missing, entry.audioPath)
			}
			continue
		}
		trackKey := pathid.ComparisonKey(entry.audioPath)
		position, exists := trackPositions[trackKey]
		if !exists {
			release, label, ok := releaseDate(metadata.Date, entry.audioPath)
			if !ok {
				return backend.LibrarySnapshot{}, fmt.Errorf("cache metadata for %q has no valid release date", entry.audioPath)
			}
			position = len(tracks)
			trackPositions[trackKey] = position
			tracks = append(tracks, trackBuild{first: entryIndex, track: library.Track{ID: entry.audioPath, Artist: metadata.Artist, Title: metadata.Title, ReleaseDate: release, ReleaseDateLabel: label, SearchTextByCategory: make(map[library.Category]string)}})
		}
		variant, err := makeVariant(entry)
		if err != nil {
			return backend.LibrarySnapshot{}, err
		}
		tracks[position].track.Variants = append(tracks[position].track.Variants, variant)
	}
	if len(missing) > 0 {
		return backend.LibrarySnapshot{}, fmt.Errorf("%d mapped audio file(s) are missing from the FLAC cache (for example: %s); use bridge mode or refresh the cache", len(missing), strings.Join(missing[:min(3, len(missing))], ", "))
	}
	result := make([]library.Track, 0, len(tracks))
	for _, built := range tracks {
		track := built.track
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
		result = append(result, track)
	}
	return backend.LibrarySnapshot{Tracks: result}, nil
}

func makeVariant(entry mappingEntry) (library.Variant, error) {
	date, label, ok := videoDate(entry.videoPath)
	if !ok {
		return library.Variant{}, fmt.Errorf("could not parse video date from %q", entry.videoPath)
	}
	modified := time.Time{}
	if info, err := os.Stat(entry.videoPath); err == nil {
		modified = info.ModTime().UTC()
	}
	return library.Variant{ID: entry.videoPath, VideoPath: entry.videoPath, AudioPath: entry.audioPath, Filename: filepath.Base(entry.videoPath), Category: classify(entry.videoPath), Date: date, DateLabel: label, ModifiedAt: modified}, nil
}

var fullDatePattern = regexp.MustCompile(`(?i)(?:\b|_)(?:(\d{4})|(?:20)?(\d{2}))[./-]?(\d{2})[./-]?(\d{2})(?:\b|_)`)

func releaseDate(tag, audioPath string) (time.Time, string, bool) {
	if matches := fullDatePattern.FindAllStringSubmatch(audioPath, -1); len(matches) == 1 {
		year := matches[0][1]
		if year == "" {
			year = "20" + matches[0][2]
		}
		return dateLabel(year + "-" + matches[0][3] + "-" + matches[0][4])
	}
	return dateLabel(tag)
}
func dateLabel(value string) (time.Time, string, bool) {
	for _, layout := range []string{"2006-01-02", "2006-01", "2006"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, value, true
		}
	}
	return time.Time{}, "", false
}
func videoDate(path string) (time.Time, string, bool) {
	name := filepath.Base(path)
	if len(name) < 6 {
		return time.Time{}, "", false
	}
	value := name[:6]
	if !regexp.MustCompile(`^\d{6}$`).MatchString(value) {
		return time.Time{}, "", false
	}
	return dateLabel("20" + value[:2] + "-" + value[2:4] + "-" + value[4:])
}

func classify(path string) library.Category {
	name := strings.ToLower(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	match := func(pattern string) bool { return regexp.MustCompile(pattern).MatchString(name) }
	switch {
	case match(`.+\s+-\s+.+\(.*band live.*\)$`):
		return library.BandLive
	case match(`.+\s+-\s+.+\sperformance$`):
		return library.Performance
	case match(`.+\s+-\s+.+\schoreography$`):
		return library.Choreography
	case match(`.+\s+-\s+.+\srelay$`):
		return library.Relay
	case match(`.+\s+-\s+.+\sbe original$`):
		return library.BeOriginal
	case match(`.+\s+-\s+.+\(.*fancam.*\)$`):
		return library.Fancam
	case match(`.+\s+-\s+.+\(.*concert.*\)$`):
		return library.Concert
	case strings.Contains(strings.ReplaceAll(path, "/", `\`), `Music\uhdkpop`):
		return library.MusicShow
	case match(`.+\s+-\s+.+\((areia\s+)?remix\)$`):
		return library.Remix
	case match(`.+\s+-\s+.+\(.*live audio.*\)$`):
		return library.LiveAudio
	default:
		return library.MusicVideo
	}
}
