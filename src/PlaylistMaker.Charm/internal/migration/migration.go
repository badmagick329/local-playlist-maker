// Package migration performs the explicit one-time move from audio-path mappings.
package migration

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"playlistmaker/charm/internal/catalog"
	"playlistmaker/charm/internal/history"
	"playlistmaker/charm/internal/metadata"
	"playlistmaker/charm/internal/pathid"
)

type Report struct {
	Tracks                   int
	Videos                   int
	HistoryEventsUpdated     int
	UnresolvedHistoryEntries int
	MappingBackupPath        string
	HistoryBackupPath        string
}

func Run(mappingPath, cataloguePath, historyPath, cachePath string) (Report, error) {
	if _, err := os.Stat(cataloguePath); err == nil {
		return Report{}, fmt.Errorf("media catalogue already exists at %q", cataloguePath)
	} else if !os.IsNotExist(err) {
		return Report{}, err
	}
	mappingContents, err := os.ReadFile(mappingPath)
	if err != nil {
		return Report{}, fmt.Errorf("read legacy mapping: %w", err)
	}
	legacy := map[string]string{}
	if err := json.Unmarshal(mappingContents, &legacy); err != nil {
		return Report{}, fmt.Errorf("parse legacy mapping: %w", err)
	}
	cache, err := metadata.ReadCache(cachePath)
	if err != nil {
		return Report{}, err
	}
	report := Report{MappingBackupPath: mappingPath + ".pre-catalogue-backup", HistoryBackupPath: historyPath + ".pre-catalogue-backup"}
	if err := copyExclusive(mappingPath, report.MappingBackupPath); err != nil {
		return Report{}, err
	}
	historyExists := false
	if _, err := os.Stat(historyPath); err == nil {
		historyExists = true
		if err := copyExclusive(historyPath, report.HistoryBackupPath); err != nil {
			return Report{}, err
		}
	} else if !os.IsNotExist(err) {
		return Report{}, err
	}

	type pair struct{ video, audio string }
	pairs := make([]pair, 0, len(legacy))
	uniqueAudio := map[string]string{}
	for video, audio := range legacy {
		video, audio = pathid.Normalize(video), pathid.Normalize(audio)
		pairs = append(pairs, pair{video: video, audio: audio})
		uniqueAudio[pathid.ComparisonKey(audio)] = audio
	}
	sort.Slice(pairs, func(i, j int) bool {
		return pathid.ComparisonKey(pairs[i].video) < pathid.ComparisonKey(pairs[j].video)
	})
	audioKeys := make([]string, 0, len(uniqueAudio))
	for key := range uniqueAudio {
		audioKeys = append(audioKeys, key)
	}
	sort.Strings(audioKeys)
	media := catalog.New()
	trackByAudio := make(map[string]string, len(audioKeys))
	for _, key := range audioKeys {
		audioPath := uniqueAudio[key]
		cached, ok := cache[key]
		if !ok {
			return Report{}, fmt.Errorf("legacy mapped audio %q is missing from the FLAC cache", audioPath)
		}
		id, err := catalog.NewTrackID()
		if err != nil {
			return Report{}, err
		}
		media.Tracks = append(media.Tracks, catalog.Track{ID: id, Artist: cached.Artist, Title: cached.Title, ReleaseDate: cached.Date, LocalAudioPath: audioPath})
		trackByAudio[key] = id
	}
	for _, item := range pairs {
		media.Videos = append(media.Videos, catalog.Video{Path: item.video, TrackID: trackByAudio[pathid.ComparisonKey(item.audio)]})
	}
	if err := catalog.Write(cataloguePath, media); err != nil {
		return Report{}, err
	}
	report.Tracks, report.Videos = len(media.Tracks), len(media.Videos)
	if historyExists {
		updated, unresolved, err := migrateHistory(historyPath, trackByAudio)
		if err != nil {
			return Report{}, err
		}
		report.HistoryEventsUpdated, report.UnresolvedHistoryEntries = updated, unresolved
	}
	return report, nil
}

func migrateHistory(path string, trackByAudio map[string]string) (int, int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	lines := make([][]byte, 0)
	updated, unresolved := 0, 0
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := append([]byte(nil), scanner.Bytes()...)
		var event history.Event
		if json.Unmarshal(line, &event) == nil && event.TrackID == "" {
			if trackID := trackByAudio[pathid.ComparisonKey(event.AudioPath)]; trackID != "" {
				event.TrackID = trackID
				line, err = json.Marshal(event)
				if err != nil {
					return 0, 0, err
				}
				updated++
			} else {
				unresolved++
			}
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		_ = file.Close()
		return 0, 0, err
	}
	if err := file.Close(); err != nil {
		return 0, 0, err
	}
	contents := []byte(strings.TrimSuffix(string(joinLines(lines)), "\n") + "\n")
	return updated, unresolved, atomicWrite(path, contents, 0o600)
}

func joinLines(lines [][]byte) []byte {
	var result []byte
	for _, line := range lines {
		result = append(result, line...)
		result = append(result, '\n')
	}
	return result
}

func copyExclusive(source, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create untouched backup %q: %w", target, err)
	}
	ok := false
	defer func() {
		_ = output.Close()
		if !ok {
			_ = os.Remove(target)
		}
	}()
	if _, err := io.Copy(output, input); err != nil {
		return err
	}
	if err := output.Sync(); err != nil {
		return err
	}
	ok = true
	return output.Close()
}

func atomicWrite(path string, contents []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".migration-*.tmp")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err = temporary.Write(contents); err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, path)
}
