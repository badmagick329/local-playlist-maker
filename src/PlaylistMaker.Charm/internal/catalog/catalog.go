// Package catalog owns PlaylistMaker's stable track and video identities.
package catalog

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"playlistmaker/charm/internal/pathid"
)

const SchemaVersion = 1

type Track struct {
	ID             string `json:"id"`
	Artist         string `json:"artist"`
	Title          string `json:"title"`
	ReleaseDate    string `json:"releaseDate"`
	LocalAudioPath string `json:"localAudioPath,omitempty"`
	SpotifyURI     string `json:"spotifyUri,omitempty"`
	SpotifyIgnored bool   `json:"spotifyIgnored"`
}

type Video struct {
	Path    string `json:"path"`
	TrackID string `json:"trackId"`
}

type Catalog struct {
	SchemaVersion int     `json:"schemaVersion"`
	Tracks        []Track `json:"tracks"`
	Videos        []Video `json:"videos"`
}

func New() Catalog { return Catalog{SchemaVersion: SchemaVersion} }

func NewTrackID() (string, error) {
	value := make([]byte, 12)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate track ID: %w", err)
	}
	return "trk_" + hex.EncodeToString(value), nil
}

func Read(path string) (Catalog, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return Catalog{}, fmt.Errorf("read media catalogue: %w", err)
	}
	var result Catalog
	if err := json.Unmarshal(contents, &result); err != nil {
		return Catalog{}, fmt.Errorf("parse media catalogue: %w", err)
	}
	if err := result.Validate(); err != nil {
		return Catalog{}, err
	}
	return result, nil
}

func (c Catalog) Validate() error {
	if c.SchemaVersion != SchemaVersion {
		return fmt.Errorf("media catalogue schemaVersion is %d, want %d", c.SchemaVersion, SchemaVersion)
	}
	tracks := make(map[string]bool, len(c.Tracks))
	localPaths := map[string]bool{}
	for index, track := range c.Tracks {
		if !strings.HasPrefix(track.ID, "trk_") || strings.TrimSpace(track.Artist) == "" || strings.TrimSpace(track.Title) == "" {
			return fmt.Errorf("media catalogue contains invalid track %d", index+1)
		}
		if tracks[track.ID] {
			return fmt.Errorf("media catalogue contains duplicate track ID %q", track.ID)
		}
		tracks[track.ID] = true
		if track.LocalAudioPath != "" {
			key := pathid.ComparisonKey(track.LocalAudioPath)
			if localPaths[key] {
				return fmt.Errorf("media catalogue contains duplicate local audio path %q", track.LocalAudioPath)
			}
			localPaths[key] = true
		}
		if track.SpotifyURI != "" && !strings.HasPrefix(track.SpotifyURI, "spotify:track:") {
			return fmt.Errorf("media catalogue track %q has invalid Spotify URI", track.ID)
		}
	}
	videos := map[string]bool{}
	for index, video := range c.Videos {
		if strings.TrimSpace(video.Path) == "" || !tracks[video.TrackID] {
			return fmt.Errorf("media catalogue contains invalid video %d", index+1)
		}
		key := pathid.ComparisonKey(video.Path)
		if videos[key] {
			return fmt.Errorf("media catalogue contains duplicate video path %q", video.Path)
		}
		videos[key] = true
	}
	return nil
}

func Write(path string, value Catalog) error {
	value.SchemaVersion = SchemaVersion
	for index := range value.Tracks {
		value.Tracks[index].LocalAudioPath = normalizeOptionalPath(value.Tracks[index].LocalAudioPath)
	}
	for index := range value.Videos {
		value.Videos[index].Path = pathid.Normalize(value.Videos[index].Path)
	}
	sort.Slice(value.Tracks, func(i, j int) bool { return value.Tracks[i].ID < value.Tracks[j].ID })
	sort.Slice(value.Videos, func(i, j int) bool {
		return pathid.ComparisonKey(value.Videos[i].Path) < pathid.ComparisonKey(value.Videos[j].Path)
	})
	if err := value.Validate(); err != nil {
		return err
	}
	contents, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	contents = append(contents, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".media-catalogue-*.tmp")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
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

func (c Catalog) Track(id string) (Track, bool) {
	for _, track := range c.Tracks {
		if track.ID == id {
			return track, true
		}
	}
	return Track{}, false
}

func (c *Catalog) LinkVideo(videoPath, trackID string) error {
	if _, ok := c.Track(trackID); !ok {
		return fmt.Errorf("unknown track ID %q", trackID)
	}
	videoPath = pathid.Normalize(videoPath)
	key := pathid.ComparisonKey(videoPath)
	for index := range c.Videos {
		if pathid.ComparisonKey(c.Videos[index].Path) == key {
			c.Videos[index] = Video{Path: videoPath, TrackID: trackID}
			return nil
		}
	}
	c.Videos = append(c.Videos, Video{Path: videoPath, TrackID: trackID})
	return nil
}

func normalizeOptionalPath(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return pathid.Normalize(value)
}
