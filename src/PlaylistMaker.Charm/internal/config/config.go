// Package config loads PlaylistMaker configuration without relying on the caller's
// working directory.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"playlistmaker/charm/internal/pathid"
)

type Config struct {
	ConfigPath                           string
	DataDirectory                        string   `yaml:"dataDirectory"`
	MappingFile                          string   `yaml:"mappingFile"`
	VideoDirectories                     []string `yaml:"videoDirectories"`
	IgnoredVideoDirectories              []string `yaml:"ignoredVideoDirectories"`
	FlacsMegaPlaylist                    string   `yaml:"flacsMegaPlaylist"`
	FlacCacheFile                        string   `yaml:"flacCacheFile"`
	PlaylistTemplate                     string   `yaml:"playlistTemplate"`
	VideoPlaylistCommand                 []string `yaml:"videoPlaylistCommand"`
	VideoPlaylistSuffix                  string   `yaml:"videoPlaylistSuffix"`
	AudioPlaylistCommand                 []string `yaml:"audioPlaylistCommand"`
	AudioPlaylistSuffix                  string   `yaml:"audioPlaylistSuffix"`
	VideoSingleFileCommand               []string `yaml:"videoSingleFileCommand"`
	AudioSingleFileCommand               []string `yaml:"audioSingleFileCommand"`
	PlaylistTxtFilePath                  string   `yaml:"playlistTxtFilePath"`
	PlaybackHistoryEnabled               bool     `yaml:"playbackHistoryEnabled"`
	PlaybackHistoryMinimumWatchedPercent int      `yaml:"playbackHistoryMinimumWatchedPercent"`
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
	if err := required("dataDirectory", c.DataDirectory); err != nil {
		return err
	}
	c.DataDirectory = pathid.Resolve(configDirectory, c.DataDirectory)

	if err := required("mappingFile", c.MappingFile); err != nil {
		return err
	}
	c.MappingFile = pathid.Resolve(configDirectory, c.MappingFile)
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

	for _, field := range []struct {
		name  string
		value *string
	}{
		{"flacsMegaPlaylist", &c.FlacsMegaPlaylist},
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
		{"audioPlaylistCommand", c.AudioPlaylistCommand},
		{"videoSingleFileCommand", c.VideoSingleFileCommand},
		{"audioSingleFileCommand", c.AudioSingleFileCommand},
	} {
		if len(command.value) == 0 || strings.TrimSpace(command.value[0]) == "" {
			return fmt.Errorf("%s must contain a program", command.name)
		}
	}
	if err := required("videoPlaylistSuffix", c.VideoPlaylistSuffix); err != nil {
		return err
	}
	if err := required("audioPlaylistSuffix", c.AudioPlaylistSuffix); err != nil {
		return err
	}
	if c.PlaybackHistoryMinimumWatchedPercent < 0 || c.PlaybackHistoryMinimumWatchedPercent > 100 {
		return fmt.Errorf("playbackHistoryMinimumWatchedPercent must be between 0 and 100")
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
