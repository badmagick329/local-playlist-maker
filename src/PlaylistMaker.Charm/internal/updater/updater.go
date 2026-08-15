// Package updater scans configured video folders and confirms explicit mappings.
package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"playlistmaker/charm/internal/config"
	"playlistmaker/charm/internal/library"
	"playlistmaker/charm/internal/mapping"
	"playlistmaker/charm/internal/metadata"
	"playlistmaker/charm/internal/pathid"
	"playlistmaker/charm/internal/videoname"
)

type Item struct {
	VideoPath   string
	Filename    string
	Artist      string
	Title       string
	AudioPath   string
	AudioArtist string
	AudioTitle  string
	Reason      string
}

type Audio struct {
	Path   string
	Artist string
	Title  string
}

type Service struct {
	Config config.Config
	Reader metadata.Reader
}

type scanIndex struct {
	mapped   map[string]bool
	evidence map[string]map[string]int
	exact    map[string][]metadata.Entry
	title    map[string][]metadata.Entry
	artist   map[string][]metadata.Entry
}

func (s Service) Scan(ctx context.Context) ([]Item, error) {
	mapped, err := mapping.Read(s.Config.MappingFile)
	if err != nil {
		return nil, err
	}
	ignoredPaths, err := ReadIgnored(s.IgnoredPath())
	if err != nil {
		return nil, err
	}
	ignored := make(map[string]bool, len(ignoredPaths))
	for _, path := range ignoredPaths {
		ignored[pathid.ComparisonKey(path)] = true
	}
	cache, err := s.refreshAudioCache(ctx)
	if err != nil {
		return nil, err
	}
	index := buildScanIndex(mapped, cache)
	paths := []string{}
	for _, root := range s.Config.VideoDirectories {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if entry.IsDir() && excluded(path, s.Config.IgnoredVideoDirectories) {
				return filepath.SkipDir
			}
			if !entry.IsDir() && supported(path) && !excluded(path, s.Config.IgnoredVideoDirectories) && !ignored[pathid.ComparisonKey(path)] && !index.mapped[pathid.ComparisonKey(path)] {
				paths = append(paths, pathid.Normalize(path))
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Slice(paths, func(i, j int) bool { return pathid.ComparisonKey(paths[i]) < pathid.ComparisonKey(paths[j]) })
	items := make([]Item, 0, len(paths))
	for _, path := range paths {
		item := Item{VideoPath: path, Filename: filepath.Base(path)}
		parsed := videoname.Parse(item.Filename)
		item.Artist, item.Title = parsed.Artist, parsed.Title
		key := matchKey(item.Artist, item.Title)
		if candidates := index.evidence[key]; len(candidates) == 1 {
			for audio := range candidates {
				item.AudioPath, item.Reason = audio, "Exact match"
				if metadata, ok := cache[pathid.ComparisonKey(audio)]; ok {
					item.AudioArtist, item.AudioTitle = metadata.Artist, metadata.Title
				}
			}
		}
		if item.AudioPath == "" {
			matches := index.exact[key]
			if len(matches) == 1 {
				item.AudioPath, item.AudioArtist, item.AudioTitle, item.Reason = matches[0].FilePath, matches[0].Artist, matches[0].Title, "Exact match"
			}
		}
		if item.AudioPath == "" {
			matches := index.title[normalize(item.Title)]
			if len(matches) == 1 {
				item.AudioPath, item.AudioArtist, item.AudioTitle, item.Reason = matches[0].FilePath, matches[0].Artist, matches[0].Title, "Possible match"
			}
		}
		if item.AudioPath == "" {
			if audio, ok := fuzzyMatch(item.Title, index.artist[normalize(item.Artist)]); ok {
				item.AudioPath, item.AudioArtist, item.AudioTitle, item.Reason = audio.FilePath, audio.Artist, audio.Title, "Possible match"
			}
		}
		items = append(items, item)
	}
	return items, nil
}

func buildScanIndex(mapped []mapping.Entry, cache map[string]metadata.Entry) scanIndex {
	index := scanIndex{
		mapped:   make(map[string]bool, len(mapped)),
		evidence: make(map[string]map[string]int),
		exact:    make(map[string][]metadata.Entry),
		title:    make(map[string][]metadata.Entry),
		artist:   make(map[string][]metadata.Entry),
	}
	for _, audio := range cache {
		key := matchKey(audio.Artist, audio.Title)
		index.exact[key] = append(index.exact[key], audio)
		index.title[normalize(audio.Title)] = append(index.title[normalize(audio.Title)], audio)
		index.artist[normalize(audio.Artist)] = append(index.artist[normalize(audio.Artist)], audio)
	}
	for _, entries := range index.exact {
		sort.Slice(entries, func(i, j int) bool {
			return pathid.ComparisonKey(entries[i].FilePath) < pathid.ComparisonKey(entries[j].FilePath)
		})
	}
	for _, entries := range index.artist {
		sort.Slice(entries, func(i, j int) bool {
			return pathid.ComparisonKey(entries[i].FilePath) < pathid.ComparisonKey(entries[j].FilePath)
		})
	}
	for _, entries := range index.title {
		sort.Slice(entries, func(i, j int) bool {
			return pathid.ComparisonKey(entries[i].FilePath) < pathid.ComparisonKey(entries[j].FilePath)
		})
	}
	for _, entry := range mapped {
		index.mapped[pathid.ComparisonKey(entry.VideoPath)] = true
		if audio, ok := cache[pathid.ComparisonKey(entry.AudioPath)]; ok {
			key := matchKey(audio.Artist, audio.Title)
			if index.evidence[key] == nil {
				index.evidence[key] = map[string]int{}
			}
			index.evidence[key][audio.FilePath]++
		}
	}
	return index
}

func (s Service) refreshAudioCache(ctx context.Context) (map[string]metadata.Entry, error) {
	paths, err := s.discoverAudio(ctx)
	if err != nil {
		return nil, err
	}
	reader := s.Reader
	if reader == nil {
		reader = metadata.FLACReader{}
	}
	entries, _, err := metadata.Ensure(ctx, s.Config.FlacCacheFile, paths, reader)
	return entries, err
}

func (s Service) discoverAudio(ctx context.Context) ([]string, error) {
	paths := []string{}
	seen := map[string]bool{}
	for _, root := range s.Config.AudioDirectories {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if entry.IsDir() || !strings.EqualFold(filepath.Ext(path), ".flac") {
				return nil
			}
			path = pathid.Normalize(path)
			key := pathid.ComparisonKey(path)
			if !seen[key] {
				seen[key] = true
				paths = append(paths, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Slice(paths, func(i, j int) bool { return pathid.ComparisonKey(paths[i]) < pathid.ComparisonKey(paths[j]) })
	return paths, nil
}

func (s Service) Ignored(ctx context.Context) ([]Item, error) {
	paths, err := ReadIgnored(s.IgnoredPath())
	if err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(paths))
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		item := Item{VideoPath: path, Filename: filepath.Base(path)}
		parsed := videoname.Parse(item.Filename)
		item.Artist, item.Title = parsed.Artist, parsed.Title
		items = append(items, item)
	}
	return items, nil
}

func (s Service) Ignore(videoPath string) error {
	paths, err := ReadIgnored(s.IgnoredPath())
	if err != nil {
		return err
	}
	return WriteIgnored(s.IgnoredPath(), append(paths, videoPath))
}

func (s Service) Restore(videoPath string) error {
	paths, err := ReadIgnored(s.IgnoredPath())
	if err != nil {
		return err
	}
	key := pathid.ComparisonKey(videoPath)
	return WriteIgnored(s.IgnoredPath(), removePath(paths, key))
}

func (s Service) IgnoredPath() string {
	return filepath.Join(s.Config.DataDirectory, "ignored-videos.json")
}

func ReadIgnored(path string) ([]string, error) {
	contents, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read ignored videos: %w", err)
	}
	if len(strings.TrimSpace(string(contents))) == 0 {
		return nil, nil
	}
	var paths []string
	if err := json.Unmarshal(contents, &paths); err != nil {
		return nil, fmt.Errorf("parse ignored videos: %w", err)
	}
	return normalizePaths(paths), nil
}

func WriteIgnored(path string, paths []string) error {
	contents, err := json.MarshalIndent(normalizePaths(paths), "", "  ")
	if err != nil {
		return err
	}
	contents = append(contents, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".ignored-videos-*.tmp")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if _, err = temporary.Write(contents); err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(temporaryName, path)
}

func excluded(path string, directories []string) bool {
	pathKey := strings.TrimRight(pathid.ComparisonKey(path), `\\/`)
	for _, directory := range directories {
		directoryKey := strings.TrimRight(pathid.ComparisonKey(directory), `\\/`)
		if pathKey == directoryKey || strings.HasPrefix(pathKey, directoryKey+`\\`) || strings.HasPrefix(pathKey, directoryKey+`/`) {
			return true
		}
	}
	return false
}

func normalizePaths(paths []string) []string {
	unique := map[string]string{}
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		path = pathid.Normalize(path)
		key := pathid.ComparisonKey(path)
		if existing, ok := unique[key]; !ok || path < existing {
			unique[key] = path
		}
	}
	result := make([]string, 0, len(unique))
	for _, path := range unique {
		result = append(result, path)
	}
	sort.Slice(result, func(i, j int) bool { return pathid.ComparisonKey(result[i]) < pathid.ComparisonKey(result[j]) })
	return result
}

func removePath(paths []string, key string) []string {
	return slices.DeleteFunc(paths, func(path string) bool { return pathid.ComparisonKey(path) == key })
}

func (s Service) Search(ctx context.Context, query string) ([]Audio, error) {
	cache, err := metadata.ReadCache(s.Config.FlacCacheFile)
	if err != nil {
		return nil, err
	}
	needle := normalize(query)
	result := []Audio{}
	for _, entry := range cache {
		if needle == "" || strings.Contains(normalize(entry.Artist+" "+entry.Title), needle) {
			result = append(result, Audio{Path: entry.FilePath, Artist: entry.Artist, Title: entry.Title})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return pathid.ComparisonKey(result[i].Path) < pathid.ComparisonKey(result[j].Path)
	})
	return result, nil
}

func (s Service) Confirm(videoPath, audioPath string) error {
	entries, err := mapping.Read(s.Config.MappingFile)
	if err != nil {
		return err
	}
	return mapping.Write(s.Config.MappingFile, mapping.Upsert(entries, videoPath, audioPath))
}

func supported(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mkv", ".mp4", ".webm", ".mov", ".m4v", ".avi":
		return true
	}
	return false
}

func matchKey(artist, title string) string { return normalize(artist) + "\x00" + normalize(title) }
func normalize(value string) string        { return videoname.Normalize(value) }

func fuzzyMatch(videoTitle string, candidates []metadata.Entry) (metadata.Entry, bool) {
	if strings.TrimSpace(videoTitle) == "" {
		return metadata.Entry{}, false
	}
	best := metadata.Entry{}
	bestScore, tied := 0, false
	for _, candidate := range candidates {
		score, ok := library.FuzzyScore(videoTitle, candidate.Title)
		if !ok || score <= 0 {
			continue
		}
		if score > bestScore {
			best, bestScore, tied = candidate, score, false
		} else if score == bestScore {
			tied = true
		}
	}
	return best, bestScore > 0 && !tied
}
