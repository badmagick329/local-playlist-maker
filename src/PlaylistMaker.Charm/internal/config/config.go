// Package config loads PlaylistMaker configuration without relying on the caller's
// working directory.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"playlistmaker/charm/internal/library"
	"playlistmaker/charm/internal/pathid"
)

type CategoryPreset struct {
	Name    string             `yaml:"name"`
	Include []library.Category `yaml:"include"`
	Exclude []library.Category `yaml:"exclude"`
}

type Config struct {
	ConfigPath                           string
	DataDirectory                        string           `yaml:"dataDirectory"`
	MediaCatalogFile                     string           `yaml:"mediaCatalogFile"`
	VideoDirectories                     []string         `yaml:"videoDirectories"`
	IgnoredVideoDirectories              []string         `yaml:"ignoredVideoDirectories"`
	AudioDirectories                     []string         `yaml:"audioDirectories"`
	FlacCacheFile                        string           `yaml:"flacCacheFile"`
	PlaylistTemplate                     string           `yaml:"playlistTemplate"`
	VideoPlaylistCommand                 []string         `yaml:"videoPlaylistCommand"`
	VideoPlaylistSuffix                  string           `yaml:"videoPlaylistSuffix"`
	VideoSingleFileCommand               []string         `yaml:"videoSingleFileCommand"`
	LocalTrackingStartCommand            []string         `yaml:"localTrackingStartCommand"`
	LocalTrackingStopCommand             []string         `yaml:"localTrackingStopCommand"`
	SpotifyClientID                      string           `yaml:"spotifyClientId"`
	SpotifyDeviceName                    string           `yaml:"spotifyDeviceName"`
	SpotifyRedirectURI                   string           `yaml:"spotifyRedirectUri"`
	LastFMUsername                       string           `yaml:"lastfmUsername"`
	LastFMAPIKey                         string           `yaml:"lastfmApiKey"`
	PlaylistTxtFilePath                  string           `yaml:"playlistTxtFilePath"`
	PlaybackHistoryEnabled               bool             `yaml:"playbackHistoryEnabled"`
	PlaybackHistoryMinimumWatchedPercent int              `yaml:"playbackHistoryMinimumWatchedPercent"`
	CategoryPresets                      []CategoryPreset `yaml:"categoryPresets"`
}

func Load(configPath string) (Config, error) {
	absConfigPath, err := filepath.Abs(configPath)
	if err != nil {
		return Config{}, fmt.Errorf("resolve config path %q: %w", configPath, err)
	}
	contents, err := os.ReadFile(absConfigPath)
	if err != nil {
		return Config{}, fmt.Errorf("read config %q: %w", absConfigPath, err)
	}

	result := Config{PlaybackHistoryMinimumWatchedPercent: 50}
	if err := yaml.Unmarshal(contents, &result); err != nil {
		return Config{}, fmt.Errorf("parse config %q: %w", absConfigPath, err)
	}
	result.ConfigPath = absConfigPath
	if err := result.resolveAndValidate(filepath.Dir(absConfigPath)); err != nil {
		return Config{}, fmt.Errorf("invalid config %q: %w", absConfigPath, err)
	}
	return result, nil
}

func (c *Config) resolveAndValidate(configDirectory string) error {
	if err := validateCategoryPresets(c.CategoryPresets); err != nil {
		return err
	}
	if err := required("dataDirectory", c.DataDirectory); err != nil {
		return err
	}
	c.DataDirectory = pathid.Resolve(configDirectory, c.DataDirectory)

	if err := required("mediaCatalogFile", c.MediaCatalogFile); err != nil {
		return err
	}
	c.MediaCatalogFile = pathid.Resolve(configDirectory, c.MediaCatalogFile)
	if len(c.VideoDirectories) == 0 {
		return fmt.Errorf("videoDirectories must contain at least one directory")
	}
	for index, value := range c.VideoDirectories {
		if err := required(fmt.Sprintf("videoDirectories[%d]", index), value); err != nil {
			return err
		}
		c.VideoDirectories[index] = pathid.Resolve(configDirectory, value)
	}
	for index, value := range c.IgnoredVideoDirectories {
		if err := required(fmt.Sprintf("ignoredVideoDirectories[%d]", index), value); err != nil {
			return err
		}
		c.IgnoredVideoDirectories[index] = pathid.Resolve(configDirectory, value)
	}
	for index, value := range c.AudioDirectories {
		if err := required(fmt.Sprintf("audioDirectories[%d]", index), value); err != nil {
			return err
		}
		c.AudioDirectories[index] = pathid.Resolve(configDirectory, value)
	}

	for _, field := range []struct {
		name  string
		value *string
	}{
		{"playlistTxtFilePath", &c.PlaylistTxtFilePath},
	} {
		if err := required(field.name, *field.value); err != nil {
			return err
		}
		*field.value = pathid.Resolve(configDirectory, *field.value)
	}
	if err := required("flacCacheFile", c.FlacCacheFile); err != nil {
		return err
	}
	c.FlacCacheFile = pathid.Resolve(c.DataDirectory, c.FlacCacheFile)

	if err := required("playlistTemplate", c.PlaylistTemplate); err != nil {
		return err
	}
	for _, command := range []struct {
		name  string
		value []string
	}{
		{"videoPlaylistCommand", c.VideoPlaylistCommand},
		{"videoSingleFileCommand", c.VideoSingleFileCommand},
	} {
		if len(command.value) == 0 || strings.TrimSpace(command.value[0]) == "" {
			return fmt.Errorf("%s must contain a program", command.name)
		}
	}
	localStart := len(c.LocalTrackingStartCommand) > 0 && strings.TrimSpace(c.LocalTrackingStartCommand[0]) != ""
	localStop := len(c.LocalTrackingStopCommand) > 0 && strings.TrimSpace(c.LocalTrackingStopCommand[0]) != ""
	if localStart != localStop {
		return fmt.Errorf("localTrackingStartCommand and localTrackingStopCommand must either both be configured or both be absent")
	}
	if err := required("videoPlaylistSuffix", c.VideoPlaylistSuffix); err != nil {
		return err
	}
	if c.SpotifyClientID != "" || c.SpotifyDeviceName != "" || c.SpotifyRedirectURI != "" {
		if err := required("spotifyClientId", c.SpotifyClientID); err != nil {
			return err
		}
		if err := required("spotifyDeviceName", c.SpotifyDeviceName); err != nil {
			return err
		}
		if err := required("spotifyRedirectUri", c.SpotifyRedirectURI); err != nil {
			return err
		}
	}
	if c.LastFMUsername != "" || c.LastFMAPIKey != "" {
		if err := required("lastfmUsername", c.LastFMUsername); err != nil {
			return err
		}
		if err := required("lastfmApiKey", c.LastFMAPIKey); err != nil {
			return err
		}
	}
	if c.PlaybackHistoryMinimumWatchedPercent < 0 || c.PlaybackHistoryMinimumWatchedPercent > 100 {
		return fmt.Errorf("playbackHistoryMinimumWatchedPercent must be between 0 and 100")
	}
	return nil
}

func validateCategoryPresets(presets []CategoryPreset) error {
	if len(presets) > 5 {
		return fmt.Errorf("categoryPresets allows at most five presets")
	}
	known := make(map[library.Category]bool, len(library.Categories))
	for _, category := range library.Categories {
		known[category] = true
	}
	names := make(map[string]bool, len(presets))
	for index, preset := range presets {
		name := strings.TrimSpace(preset.Name)
		label := fmt.Sprintf("categoryPresets[%d]", index)
		if name != "" {
			label += " (" + name + ")"
		}
		if name == "" {
			return fmt.Errorf("%s name must be non-empty", label)
		}
		key := strings.ToLower(name)
		if names[key] {
			return fmt.Errorf("%s name must be unique", label)
		}
		names[key] = true
		if len(preset.Include) > 0 && len(preset.Exclude) > 0 {
			return fmt.Errorf("%s must define either include or exclude, not both", label)
		}
		if len(preset.Include) == 0 && len(preset.Exclude) == 0 {
			return fmt.Errorf("%s must define a non-empty include or exclude list", label)
		}
		seen := map[library.Category]bool{}
		for field, categories := range map[string][]library.Category{"include": preset.Include, "exclude": preset.Exclude} {
			for _, category := range categories {
				if !known[category] {
					return fmt.Errorf("%s %s contains unknown category %q", label, field, category)
				}
				if seen[category] {
					return fmt.Errorf("%s %s contains duplicate category %q", label, field, category)
				}
				seen[category] = true
			}
		}
		presets[index].Name = name
	}
	return nil
}

// Discover returns a config path without loading it. An explicit path keeps its
// caller-relative meaning; otherwise only the executable directory is searched.
func Discover(explicitPath, executablePath string) (string, error) {
	if strings.TrimSpace(explicitPath) != "" {
		path, err := filepath.Abs(explicitPath)
		if err != nil {
			return "", fmt.Errorf("resolve explicit config path %q: %w", explicitPath, err)
		}
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("read explicit config %q: %w", path, err)
		}
		return path, nil
	}
	if strings.TrimSpace(executablePath) == "" {
		return "", fmt.Errorf("PlaylistMaker config was not found; pass --config with its path")
	}
	candidate := filepath.Join(filepath.Dir(executablePath), "config.yaml")
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}
	return "", fmt.Errorf("PlaylistMaker config was not found beside %q; pass --config with its path", executablePath)
}

func required(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", name)
	}
	return nil
}
