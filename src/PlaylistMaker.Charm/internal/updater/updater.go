// Package updater separates media discovery from explicit catalogue changes.
package updater

import (
	"playlistmaker/charm/internal/config"
	"playlistmaker/charm/internal/metadata"
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

type ScanResult struct {
	Items   []Item
	Removed int
}

type Service struct {
	Config config.Config
	Reader metadata.Reader
}

func matchKey(artist, title string) string { return normalize(artist) + "\x00" + normalize(title) }

func normalize(value string) string { return videoname.Normalize(value) }
