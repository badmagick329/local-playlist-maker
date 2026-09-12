package updater

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"playlistmaker/charm/internal/catalog"
	"playlistmaker/charm/internal/metadata"
	"playlistmaker/charm/internal/pathid"
)

func (s Service) refreshAudioCache(ctx context.Context, media catalog.Catalog) (map[string]metadata.Entry, error) {
	paths, err := s.discoverAudio(ctx)
	if err != nil {
		return nil, err
	}
	// Explicit links remain valid even when discovery folders change.
	for _, track := range media.Tracks {
		if track.LocalAudioPath != "" {
			if info, err := os.Stat(track.LocalAudioPath); err == nil && !info.IsDir() {
				paths = append(paths, track.LocalAudioPath)
			}
		}
	}
	reader := s.Reader
	if reader == nil {
		reader = metadata.FLACReader{}
	}
	entries, _, err := metadata.Ensure(ctx, s.Config.FlacCacheFile, paths, reader)
	if entries == nil {
		return nil, err
	}
	present := make(map[string]metadata.Entry, len(entries))
	for _, path := range paths {
		key := pathid.ComparisonKey(path)
		if entry, ok := entries[key]; ok {
			present[key] = entry
		}
	}
	return present, nil
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

// Only confirmed absence invalidates a link; access errors are not evidence of a rename.
func missingAudio(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return os.IsNotExist(err) || err == nil && info.IsDir()
}
