// Package mapping reads and writes the canonical video-to-audio map.
package mapping

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"playlistmaker/charm/internal/pathid"
)

type Entry struct {
	VideoPath string
	AudioPath string
}

func Read(path string) ([]Entry, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read mapping: %w", err)
	}
	values := map[string]string{}
	if err := json.Unmarshal(contents, &values); err != nil {
		return nil, fmt.Errorf("parse mapping: %w", err)
	}
	entries := make([]Entry, 0, len(values))
	for video, audio := range values {
		entries = append(entries, Entry{VideoPath: video, AudioPath: audio})
	}
	return deduplicate(entries)
}

func Upsert(entries []Entry, videoPath, audioPath string) []Entry {
	videoPath, audioPath = pathid.Normalize(videoPath), pathid.Normalize(audioPath)
	for index := range entries {
		if pathid.ComparisonKey(entries[index].VideoPath) == pathid.ComparisonKey(videoPath) {
			entries[index] = Entry{VideoPath: videoPath, AudioPath: audioPath}
			return entries
		}
	}
	return append(entries, Entry{VideoPath: videoPath, AudioPath: audioPath})
}

func Write(path string, entries []Entry) error {
	ordered, err := deduplicate(entries)
	if err != nil {
		return err
	}
	contents := []byte("{\n")
	for index, entry := range ordered {
		video, _ := json.Marshal(entry.VideoPath)
		audio, _ := json.Marshal(entry.AudioPath)
		contents = append(contents, "  "...)
		contents = append(contents, video...)
		contents = append(contents, ": "...)
		contents = append(contents, audio...)
		if index+1 < len(ordered) {
			contents = append(contents, ',')
		}
		contents = append(contents, '\n')
	}
	contents = append(contents, '}', '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".mapping-*.tmp")
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

func deduplicate(entries []Entry) ([]Entry, error) {
	unique := make(map[string]Entry, len(entries))
	for _, entry := range entries {
		if strings.TrimSpace(entry.VideoPath) == "" || strings.TrimSpace(entry.AudioPath) == "" {
			return nil, fmt.Errorf("mapping paths must not be empty")
		}
		entry.VideoPath = pathid.Normalize(entry.VideoPath)
		entry.AudioPath = pathid.Normalize(entry.AudioPath)
		unique[pathid.ComparisonKey(entry.VideoPath)] = entry
	}
	result := make([]Entry, 0, len(unique))
	for _, entry := range unique {
		result = append(result, entry)
	}
	sortEntries(result)
	return result, nil
}

func sortEntries(entries []Entry) {
	sort.Slice(entries, func(i, j int) bool {
		return pathid.ComparisonKey(entries[i].VideoPath) < pathid.ComparisonKey(entries[j].VideoPath)
	})
}
