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

	"playlistmaker/charm/internal/pathid"
	"playlistmaker/charm/internal/videoname"
)

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
