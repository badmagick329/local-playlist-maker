package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadResolvesEveryConfiguredFileAgainstTheConfigDirectory(t *testing.T) {
	root := t.TempDir()
	configDirectory := filepath.Join(root, "settings")
	if err := os.MkdirAll(configDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(configDirectory, "config.yaml")
	writeConfig(t, path, `
dataDirectory: state
musicVideoToAudioMap: [maps/one.json, maps/two.json]
flacsMegaPlaylist: media/all.m3u8
flacCacheFile: cache/flac_cache.json
playlistTemplate: "{playlistPath}"
videoPlaylistCommand: [mpv.exe, "--playlist={playlistPath}"]
videoPlaylistSuffix: _videos.m3u
audioPlaylistCommand: [foobar2000.exe, "{playlistPath}"]
audioPlaylistSuffix: _audios.m3u8
videoSingleFileCommand: [mpv.exe]
audioSingleFileCommand: [foobar2000.exe]
playlistTxtFilePath: imports/queue.txt
playbackHistoryEnabled: true
playbackHistoryMinimumWatchedPercent: 50
`)

	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := func(relative string) string { return filepath.Join(configDirectory, relative) }
	if loaded.DataDirectory != want("state") || loaded.FlacsMegaPlaylist != want("media/all.m3u8") || loaded.FlacCacheFile != want("cache/flac_cache.json") || loaded.PlaylistTxtFilePath != want("imports/queue.txt") {
		t.Fatalf("relative paths were not resolved from the config directory: %#v", loaded)
	}
	if got, expected := loaded.MusicVideoToAudioMap, []string{want("maps/one.json"), want("maps/two.json")}; len(got) != len(expected) || got[0] != expected[0] || got[1] != expected[1] {
		t.Fatalf("mapping files = %#v, want %#v", got, expected)
	}
	if loaded.VideoPlaylistCommand[0] != "mpv.exe" || loaded.VideoPlaylistCommand[1] != "--playlist={playlistPath}" {
		t.Fatalf("command arguments were incorrectly treated as paths: %#v", loaded.VideoPlaylistCommand)
	}
}

func TestLoadReportsTheMissingFieldAndResolvedConfigPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	writeConfig(t, path, "dataDirectory: state\n")

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "musicVideoToAudioMap") || !strings.Contains(err.Error(), "invalid config") || !strings.Contains(err.Error(), filepath.Base(path)) {
		t.Fatalf("error = %v, want the missing field and resolved config path", err)
	}
}

func writeConfig(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
