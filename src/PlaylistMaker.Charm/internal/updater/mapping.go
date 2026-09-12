package updater

import (
	"fmt"
	"os"
	"strings"

	"playlistmaker/charm/internal/catalog"
	"playlistmaker/charm/internal/metadata"
	"playlistmaker/charm/internal/pathid"
)

// ExistingTracksError gives the picker a chance to reuse an identity before creating one.
type ExistingTracksError struct{ Artist, Title, Selection string }

func (e *ExistingTracksError) Error() string { return "Matching catalogue tracks already exist" }

func hasMatchingTrack(media catalog.Catalog, artist, title string) bool {
	key := matchKey(artist, title)
	for _, track := range media.Tracks {
		if matchKey(track.Artist, track.Title) == key {
			return true
		}
	}
	return false
}

func (s Service) Confirm(videoPath, selection string, allowNew bool) error {
	media, err := catalog.Read(s.Config.MediaCatalogFile)
	if err != nil {
		return err
	}
	trackID, err := s.resolveTrackSelection(&media, videoPath, selection, allowNew)
	if err != nil {
		return err
	}
	if err := media.LinkVideo(videoPath, trackID); err != nil {
		return err
	}
	return catalog.Write(s.Config.MediaCatalogFile, media)
}

// Reusing or repairing an identity takes precedence over creating a new track.
func (s Service) resolveTrackSelection(media *catalog.Catalog, videoPath, selection string, allowNew bool) (string, error) {
	if track, ok := media.Track(selection); ok {
		if track.LocalAudioPath != "" {
			if err := validateSelectedAudio(track.LocalAudioPath); err != nil {
				return "", err
			}
		}
		return track.ID, nil
	}
	cache, err := metadata.ReadCache(s.Config.FlacCacheFile)
	if err != nil {
		return "", err
	}
	entry, found := cache[pathid.ComparisonKey(selection)]
	if !found {
		return "", fmt.Errorf("unknown local track %q", selection)
	}
	if err := validateSelectedAudio(entry.FilePath); err != nil {
		return "", err
	}
	audioKey := pathid.ComparisonKey(entry.FilePath)
	for _, track := range media.Tracks {
		if pathid.ComparisonKey(track.LocalAudioPath) == audioKey {
			return track.ID, nil
		}
	}
	if id, ok := repairAudioLink(media, videoPath, entry); ok {
		return id, nil
	}
	if !allowNew && hasMatchingTrack(*media, entry.Artist, entry.Title) {
		return "", &ExistingTracksError{Artist: entry.Artist, Title: entry.Title, Selection: selection}
	}
	id, err := catalog.NewTrackID()
	if err != nil {
		return "", err
	}
	media.Tracks = append(media.Tracks, catalog.Track{ID: id, Artist: entry.Artist, Title: entry.Title, ReleaseDate: entry.Date, LocalAudioPath: entry.FilePath})
	return id, nil
}

// Preserve the existing ID so every video and history reference follows the repair.
func repairAudioLink(media *catalog.Catalog, videoPath string, entry metadata.Entry) (string, bool) {
	videoKey := pathid.ComparisonKey(videoPath)
	for _, video := range media.Videos {
		if pathid.ComparisonKey(video.Path) != videoKey {
			continue
		}
		for i := range media.Tracks {
			track := &media.Tracks[i]
			if track.ID == video.TrackID && missingAudio(track.LocalAudioPath) {
				track.LocalAudioPath, track.ReleaseDate = entry.FilePath, entry.Date
				track.Artist, track.Title = entry.Artist, entry.Title
				return track.ID, true
			}
		}
	}
	return "", false
}

// A picker snapshot cannot establish that a file still exists at confirmation time.
func validateSelectedAudio(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("access selected audio: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("selected audio is a folder: %s", path)
	}
	return nil
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
