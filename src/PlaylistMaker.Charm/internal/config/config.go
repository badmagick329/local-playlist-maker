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
	MusicVideoToAudioMap                 []string `yaml:"musicVideoToAudioMap"`
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

	var result Config
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

	if len(c.MusicVideoToAudioMap) == 0 {
		return fmt.Errorf("musicVideoToAudioMap must contain at least one mapping file")
	}
	for index, value := range c.MusicVideoToAudioMap {
		if err := required(fmt.Sprintf("musicVideoToAudioMap[%d]", index), value); err != nil {
			return err
		}
		c.MusicVideoToAudioMap[index] = pathid.Resolve(configDirectory, value)
	}

	for _, field := range []struct {
		name  string
		value *string
	}{
		{"flacsMegaPlaylist", &c.FlacsMegaPlaylist},
		{"flacCacheFile", &c.FlacCacheFile},
		{"playlistTxtFilePath", &c.PlaylistTxtFilePath},
	} {
		if err := required(field.name, *field.value); err != nil {
			return err
		}
		*field.value = pathid.Resolve(configDirectory, *field.value)
	}

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

func required(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", name)
	}
	return nil
}
