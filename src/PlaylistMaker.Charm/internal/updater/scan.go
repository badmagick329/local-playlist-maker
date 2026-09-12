package updater

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"playlistmaker/charm/internal/catalog"
	"playlistmaker/charm/internal/library"
	"playlistmaker/charm/internal/metadata"
	"playlistmaker/charm/internal/pathid"
	"playlistmaker/charm/internal/videoname"
)

type scanIndex struct {
	mapped   map[string]bool
	evidence map[string]map[string]int
	exact    map[string][]metadata.Entry
	title    map[string][]metadata.Entry
	artist   map[string][]metadata.Entry
}

func (s Service) Scan(ctx context.Context) (ScanResult, error) {
	media, err := catalog.Read(s.Config.MediaCatalogFile)
	if err != nil {
		return ScanResult{}, err
	}
	ignoredPaths, err := ReadIgnored(s.IgnoredPath())
	if err != nil {
		return ScanResult{}, err
	}
	ignored := make(map[string]bool, len(ignoredPaths))
	for _, path := range ignoredPaths {
		ignored[pathid.ComparisonKey(path)] = true
	}
	cache, err := s.refreshAudioCache(ctx, media)
	if err != nil {
		return ScanResult{}, err
	}
	index := buildScanIndex(media, cache)
	// A broken audio link must return to review even when its video is mapped.
	for _, video := range media.Videos {
		track, _ := media.Track(video.TrackID)
		if missingAudio(track.LocalAudioPath) {
			delete(index.mapped, pathid.ComparisonKey(video.Path))
		}
	}
	paths := []string{}
	present := make(map[string]bool)
	for _, root := range s.Config.VideoDirectories {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if entry.IsDir() && within(path, s.Config.IgnoredVideoDirectories) {
				return filepath.SkipDir
			}
			if !entry.IsDir() && !within(path, s.Config.IgnoredVideoDirectories) {
				normalized := pathid.Normalize(path)
				present[pathid.ComparisonKey(normalized)] = true
				if supported(path) && !ignored[pathid.ComparisonKey(normalized)] && !index.mapped[pathid.ComparisonKey(normalized)] {
					paths = append(paths, normalized)
				}
			}
			return nil
		})
		if err != nil {
			return ScanResult{}, err
		}
	}
	removed := 0
	keptVideos := media.Videos[:0]
	for _, video := range media.Videos {
		key := pathid.ComparisonKey(video.Path)
		managed := within(video.Path, s.Config.VideoDirectories) && !within(video.Path, s.Config.IgnoredVideoDirectories)
		if managed && !present[key] {
			removed++
			continue
		}
		keptVideos = append(keptVideos, video)
	}
	media.Videos = keptVideos
	if removed > 0 {
		if err := catalog.Write(s.Config.MediaCatalogFile, media); err != nil {
			return ScanResult{}, err
		}
	}
	sort.Slice(paths, func(i, j int) bool { return pathid.ComparisonKey(paths[i]) < pathid.ComparisonKey(paths[j]) })
	items := make([]Item, 0, len(paths))
	trackByAudio := make(map[string]catalog.Track, len(media.Tracks))
	trackByID := make(map[string]catalog.Track, len(media.Tracks))
	for _, track := range media.Tracks {
		trackByID[track.ID] = track
		if track.LocalAudioPath != "" {
			trackByAudio[pathid.ComparisonKey(track.LocalAudioPath)] = track
		}
	}
	for _, path := range paths {
		item := Item{VideoPath: path, Filename: filepath.Base(path)}
		parsed := videoname.Parse(item.Filename)
		item.Artist, item.Title = parsed.Artist, parsed.Title
		key := matchKey(item.Artist, item.Title)
		if candidates := index.evidence[key]; len(candidates) == 1 {
			for trackID := range candidates {
				track := trackByID[trackID]
				item.AudioPath, item.AudioArtist, item.AudioTitle, item.Reason = track.ID, track.Artist, track.Title, "Exact match"
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
		if track, ok := trackByAudio[pathid.ComparisonKey(item.AudioPath)]; ok {
			item.AudioPath, item.AudioArtist, item.AudioTitle = track.ID, track.Artist, track.Title
		}
		items = append(items, item)
	}
	return ScanResult{Items: items, Removed: removed}, nil
}

func buildScanIndex(media catalog.Catalog, cache map[string]metadata.Entry) scanIndex {
	index := scanIndex{
		mapped:   make(map[string]bool, len(media.Videos)),
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
	for _, groups := range []map[string][]metadata.Entry{index.exact, index.artist, index.title} {
		for _, entries := range groups {
			sort.Slice(entries, func(i, j int) bool {
				return pathid.ComparisonKey(entries[i].FilePath) < pathid.ComparisonKey(entries[j].FilePath)
			})
		}
	}
	for _, video := range media.Videos {
		index.mapped[pathid.ComparisonKey(video.Path)] = true
	}
	for _, track := range media.Tracks {
		if audio, ok := cache[pathid.ComparisonKey(track.LocalAudioPath)]; ok {
			key := matchKey(audio.Artist, audio.Title)
			if index.evidence[key] == nil {
				index.evidence[key] = map[string]int{}
			}
			index.evidence[key][track.ID]++
		}
	}
	return index
}

func supported(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mkv", ".mp4", ".webm", ".mov", ".m4v", ".avi":
		return true
	}
	return false
}

func fuzzyMatch(videoTitle string, candidates []metadata.Entry) (metadata.Entry, bool) {
	if strings.TrimSpace(videoTitle) == "" {
		return metadata.Entry{}, false
	}
	best := metadata.Entry{}
	bestScore, tied := 0, false
	for _, candidate := range candidates {
		candidateScore, candidateOK := library.FuzzyScore(candidate.Title, videoTitle)
		videoScore, videoOK := library.FuzzyScore(videoTitle, candidate.Title)
		score := max(candidateScore, videoScore)
		if (!candidateOK && !videoOK) || score <= 0 {
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

func within(path string, directories []string) bool {
	pathKey := strings.TrimRight(pathid.ComparisonKey(path), `\\/`)
	for _, directory := range directories {
		directoryKey := strings.TrimRight(pathid.ComparisonKey(directory), `\\/`)
		if pathKey == directoryKey || strings.HasPrefix(pathKey, directoryKey+`\`) || strings.HasPrefix(pathKey, directoryKey+`/`) {
			return true
		}
	}
	return false
}
