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

	"playlistmaker/charm/internal/catalog"
	"playlistmaker/charm/internal/config"
	"playlistmaker/charm/internal/library"
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

// ExistingTracksError gives the picker a chance to reuse an identity before creating one.
type ExistingTracksError struct{ Artist, Title, Selection string }

func (e *ExistingTracksError) Error() string { return "Matching catalogue tracks already exist" }

func hasMatchingTrack(media catalog.Catalog, artist, title string) bool {
	for _, track := range media.Tracks {
		if matchKey(track.Artist, track.Title) == matchKey(artist, title) {
			return true
		}
	}
	return false
}

type ScanResult struct {
	Items   []Item
	Removed int
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
			if entry.IsDir() && excluded(path, s.Config.IgnoredVideoDirectories) {
				return filepath.SkipDir
			}
			if !entry.IsDir() && !excluded(path, s.Config.IgnoredVideoDirectories) {
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
		managed := within(video.Path, s.Config.VideoDirectories) && !excluded(video.Path, s.Config.IgnoredVideoDirectories)
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
		if pathKey == directoryKey || strings.HasPrefix(pathKey, directoryKey+`\`) || strings.HasPrefix(pathKey, directoryKey+`/`) {
			return true
		}
	}
	return false
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
	add := func(audio Audio) { candidates = append(candidates, audio) }
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
		add(Audio{Kind: kind, VideoCount: videos[track.ID], SpotifyOnly: track.LocalAudioPath == "" && track.SpotifyURI != "", Path: track.ID, Artist: track.Artist, Title: track.Title, Album: entry.Album, ReleaseDate: date, Source: source})
	}
	for key, entry := range cache {
		if !claimed[key] && discovered[key] {
			add(Audio{Kind: UnlinkedAudio, Path: entry.FilePath, Artist: entry.Artist, Title: entry.Title, Album: entry.Album, ReleaseDate: entry.Date, Source: entry.FilePath})
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

func (s Service) Confirm(videoPath, selection string, allowNew bool) error {
	media, err := catalog.Read(s.Config.MediaCatalogFile)
	if err != nil {
		return err
	}
	trackID := selection
	if track, ok := media.Track(trackID); ok && track.LocalAudioPath != "" {
		info, statErr := os.Stat(track.LocalAudioPath)
		if statErr != nil {
			return fmt.Errorf("access selected audio: %w", statErr)
		}
		if info.IsDir() {
			return fmt.Errorf("selected audio is a folder: %s", track.LocalAudioPath)
		}
	}
	if _, ok := media.Track(trackID); !ok {
		cache, readErr := metadata.ReadCache(s.Config.FlacCacheFile)
		if readErr != nil {
			return readErr
		}
		entry, found := cache[pathid.ComparisonKey(selection)]
		if !found {
			return fmt.Errorf("unknown local track %q", selection)
		}
		info, statErr := os.Stat(entry.FilePath)
		if statErr != nil {
			return fmt.Errorf("access selected audio: %w", statErr)
		}
		if info.IsDir() {
			return fmt.Errorf("selected audio is a folder: %s", entry.FilePath)
		}
		matchedTrack := false
		for _, track := range media.Tracks {
			if pathid.ComparisonKey(track.LocalAudioPath) == pathid.ComparisonKey(entry.FilePath) {
				trackID = track.ID
				matchedTrack = true
				break
			}
		}
		if !matchedTrack {
			// Repair the existing identity so every video and history reference follows it.
			for _, video := range media.Videos {
				if pathid.ComparisonKey(video.Path) != pathid.ComparisonKey(videoPath) {
					continue
				}
				for i := range media.Tracks {
					track := &media.Tracks[i]
					if track.ID == video.TrackID && missingAudio(track.LocalAudioPath) {
						track.LocalAudioPath, track.ReleaseDate = entry.FilePath, entry.Date
						track.Artist, track.Title = entry.Artist, entry.Title
						trackID, matchedTrack = track.ID, true
					}
				}
			}
		}
		if !matchedTrack {
			if !allowNew && hasMatchingTrack(media, entry.Artist, entry.Title) {
				return &ExistingTracksError{Artist: entry.Artist, Title: entry.Title, Selection: selection}
			}
			trackID, err = catalog.NewTrackID()
			if err != nil {
				return err
			}
			media.Tracks = append(media.Tracks, catalog.Track{ID: trackID, Artist: entry.Artist, Title: entry.Title, ReleaseDate: entry.Date, LocalAudioPath: entry.FilePath})
		}
	}
	if err := media.LinkVideo(videoPath, trackID); err != nil {
		return err
	}
	return catalog.Write(s.Config.MediaCatalogFile, media)
}

func (s Service) Create(videoPath, artist, title string, allowNew bool) error {
	media, err := catalog.Read(s.Config.MediaCatalogFile)
	if err != nil {
		return err
	}
	if !allowNew && hasMatchingTrack(media, artist, title) {
		return &ExistingTracksError{Artist: artist, Title: title}
	}
	id, err := catalog.NewTrackID()
	if err != nil {
		return err
	}
	media.Tracks = append(media.Tracks, catalog.Track{ID: id, Artist: strings.TrimSpace(artist), Title: strings.TrimSpace(title)})
	if err := media.LinkVideo(videoPath, id); err != nil {
		return err
	}
	return catalog.Write(s.Config.MediaCatalogFile, media)
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

// Only confirmed absence invalidates a link; access errors are not evidence of a rename.
func missingAudio(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return os.IsNotExist(err) || err == nil && info.IsDir()
}
