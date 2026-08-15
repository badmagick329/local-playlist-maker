// Package updater scans configured video folders and confirms explicit mappings.
package updater

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"playlistmaker/charm/internal/config"
	"playlistmaker/charm/internal/mapping"
	"playlistmaker/charm/internal/metadata"
	"playlistmaker/charm/internal/pathid"
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

type Service struct{ Config config.Config }

func (s Service) Scan(ctx context.Context) ([]Item, error) {
	mapped, err := mapping.Read(s.Config.MappingFile)
	if err != nil {
		return nil, err
	}
	mappedPaths := map[string]bool{}
	used := map[string]map[string]int{}
	cache, err := metadata.ReadCache(s.Config.FlacCacheFile)
	if err != nil {
		return nil, err
	}
	for _, entry := range mapped {
		mappedPaths[pathid.ComparisonKey(entry.VideoPath)] = true
		if audio, ok := cache[pathid.ComparisonKey(entry.AudioPath)]; ok {
			key := matchKey(audio.Artist, audio.Title)
			if used[key] == nil {
				used[key] = map[string]int{}
			}
			used[key][audio.FilePath]++
		}
	}
	paths := []string{}
	for _, root := range s.Config.VideoDirectories {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if !entry.IsDir() && supported(path) && !mappedPaths[pathid.ComparisonKey(path)] {
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
		item.Artist, item.Title = parse(item.Filename)
		key := matchKey(item.Artist, item.Title)
		if candidates := used[key]; len(candidates) == 1 {
			for audio, count := range candidates {
				item.AudioPath, item.Reason = audio, fmt.Sprintf("Exact match; %d videos already linked", count)
				if metadata, ok := cache[pathid.ComparisonKey(audio)]; ok {
					item.AudioArtist, item.AudioTitle = metadata.Artist, metadata.Title
				}
			}
		} else if len(candidates) == 0 {
			matches := []metadata.Entry{}
			for _, audio := range cache {
				if matchKey(audio.Artist, audio.Title) == key {
					matches = append(matches, audio)
				}
			}
			if len(matches) == 1 {
				item.AudioPath, item.AudioArtist, item.AudioTitle, item.Reason = matches[0].FilePath, matches[0].Artist, matches[0].Title, "Exact cache match"
			}
		}
		items = append(items, item)
	}
	return items, nil
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

func parse(filename string) (string, string) {
	name := strings.TrimSuffix(filename, filepath.Ext(filename))
	if len(name) >= 7 && allDigits(name[:6]) && name[6] == ' ' {
		name = name[7:]
	}
	artist, title, found := strings.Cut(name, " - ")
	if !found {
		return "", ""
	}
	for _, suffix := range []string{" Performance", " Choreography", " Relay", " Be Original"} {
		if index := strings.Index(strings.ToLower(title), strings.ToLower(suffix)); index >= 0 {
			title = title[:index]
			break
		}
	}
	return strings.TrimSpace(artist), strings.TrimSpace(title)
}

func allDigits(value string) bool {
	for _, value := range value {
		if value < '0' || value > '9' {
			return false
		}
	}
	return true
}
func matchKey(artist, title string) string { return normalize(artist) + "\x00" + normalize(title) }
func normalize(value string) string {
	return strings.ToLower(strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return r
		}
		return ' '
	}, value))
}
