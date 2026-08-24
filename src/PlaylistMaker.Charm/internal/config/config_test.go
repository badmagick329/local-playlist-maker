package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadResolvesEveryConfiguredFileAgainstTheConfigDirectory(t *testing.T) {
	root := t.TempDir()
	configDirectory := filepath.Join(root, "settings")
	absoluteIgnored := filepath.Join(root, "absolute-ignored")
	absoluteAudio := filepath.Join(root, "absolute-audio")
	if err := os.MkdirAll(configDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(configDirectory, "config.yaml")
	writeConfig(t, path, `
dataDirectory: state
mediaCatalogFile: maps/media-catalog.json
videoDirectories: [videos, other-videos]
ignoredVideoDirectories: [ignored, "`+filepath.ToSlash(absoluteIgnored)+`"]
audioDirectories: [audio, "`+filepath.ToSlash(absoluteAudio)+`"]
flacCacheFile: cache/flac_cache.json
playlistTemplate: "{playlistPath}"
videoPlaylistCommand: [mpv.exe, "--playlist={playlistPath}"]
videoPlaylistSuffix: _videos.m3u
videoSingleFileCommand: [mpv.exe]
localTrackingStartCommand: [foobar2000.exe, "{audioPath}"]
localTrackingStopCommand: [foobar2000.exe, /stop]
playlistTxtFilePath: imports/queue.txt
playbackHistoryEnabled: true
playbackHistoryMinimumWatchedPercent: 50
`)

	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := func(relative string) string { return filepath.Join(configDirectory, relative) }
	if loaded.DataDirectory != want("state") || loaded.FlacCacheFile != filepath.Join(want("state"), "cache/flac_cache.json") || loaded.PlaylistTxtFilePath != want("imports/queue.txt") {
		t.Fatalf("relative paths were not resolved from the config directory: %#v", loaded)
	}
	if loaded.MediaCatalogFile != want("maps/media-catalog.json") || len(loaded.VideoDirectories) != 2 || loaded.VideoDirectories[0] != want("videos") || loaded.VideoDirectories[1] != want("other-videos") {
		t.Fatalf("mapping/update paths = %#v", loaded)
	}
	if len(loaded.IgnoredVideoDirectories) != 2 || loaded.IgnoredVideoDirectories[0] != want("ignored") || loaded.IgnoredVideoDirectories[1] != absoluteIgnored {
		t.Fatalf("ignored video directories = %#v", loaded.IgnoredVideoDirectories)
	}
	if len(loaded.AudioDirectories) != 2 || loaded.AudioDirectories[0] != want("audio") || loaded.AudioDirectories[1] != absoluteAudio {
		t.Fatalf("audio directories = %#v", loaded.AudioDirectories)
	}
	if loaded.VideoPlaylistCommand[0] != "mpv.exe" || loaded.VideoPlaylistCommand[1] != "--playlist={playlistPath}" {
		t.Fatalf("command arguments were incorrectly treated as paths: %#v", loaded.VideoPlaylistCommand)
	}
}

func TestLoadKeepsExplicitZeroAndDefaultsOmittedHistoryThreshold(t *testing.T) {
	directory := t.TempDir()
	withDefault := filepath.Join(directory, "default.yaml")
	withZero := filepath.Join(directory, "zero.yaml")
	contents := validConfig("playbackHistoryMinimumWatchedPercent: %s\n")
	writeConfig(t, withDefault, validConfig(""))
	writeConfig(t, withZero, fmt.Sprintf(contents, "0"))

	defaultConfig, err := Load(withDefault)
	if err != nil {
		t.Fatal(err)
	}
	zeroConfig, err := Load(withZero)
	if err != nil {
		t.Fatal(err)
	}
	if defaultConfig.PlaybackHistoryMinimumWatchedPercent != 50 || zeroConfig.PlaybackHistoryMinimumWatchedPercent != 0 {
		t.Fatalf("history thresholds = %d and %d, want 50 and 0", defaultConfig.PlaybackHistoryMinimumWatchedPercent, zeroConfig.PlaybackHistoryMinimumWatchedPercent)
	}
}

func TestDiscoverUsesExplicitPathThenExecutableDirectory(t *testing.T) {
	root := t.TempDir()
	explicit := filepath.Join(root, "requested.yml")
	writeConfig(t, explicit, "fixture")
	got, err := Discover(explicit, filepath.Join(root, "bin", "playlistmaker.exe"))
	if err != nil || got != explicit {
		t.Fatalf("explicit discovery = %q, %v", got, err)
	}
	executable := filepath.Join(root, "bin", "playlistmaker.exe")
	if err := os.MkdirAll(filepath.Dir(executable), 0o755); err != nil {
		t.Fatal(err)
	}
	writeConfig(t, filepath.Join(filepath.Dir(executable), "config.yaml"), "fixture")
	got, err = Discover("", executable)
	if err != nil || got != filepath.Join(filepath.Dir(executable), "config.yaml") {
		t.Fatalf("fallback discovery = %q, %v", got, err)
	}
	if _, err := Discover("", filepath.Join(root, "missing", "playlistmaker.exe")); err == nil || !strings.Contains(err.Error(), "--config") {
		t.Fatalf("missing discovery error = %v", err)
	}
	if _, err := Discover(filepath.Join(root, "missing.yml"), executable); err == nil || !strings.Contains(err.Error(), "explicit config") {
		t.Fatalf("missing explicit discovery error = %v", err)
	}
}

func TestLoadReportsTheMissingFieldAndResolvedConfigPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	writeConfig(t, path, "dataDirectory: state\n")

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "mediaCatalogFile") || !strings.Contains(err.Error(), "invalid config") || !strings.Contains(err.Error(), filepath.Base(path)) {
		t.Fatalf("error = %v, want the missing field and resolved config path", err)
	}
}

func TestLoadAllowsSpotifyOnlyConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	contents := strings.ReplaceAll(validConfig("spotifyClientId: client\nspotifyDeviceName: Living Room\nspotifyRedirectUri: http://127.0.0.1/callback\n"), "audioDirectories: [audio]\n", "audioDirectories: []\n")
	contents = strings.ReplaceAll(contents, "localTrackingStartCommand: [foobar2000.exe, \"{audioPath}\"]\nlocalTrackingStopCommand: [foobar2000.exe, /stop]\n", "")
	writeConfig(t, path, contents)
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.AudioDirectories) != 0 || len(loaded.LocalTrackingStartCommand) != 0 || len(loaded.LocalTrackingStopCommand) != 0 {
		t.Fatalf("Spotify-only config = %#v", loaded)
	}
}

func TestLoadRequiresBothLocalTrackingCommands(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	contents := strings.ReplaceAll(validConfig(""), "localTrackingStopCommand: [foobar2000.exe, /stop]\n", "")
	writeConfig(t, path, contents)
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "must either both") {
		t.Fatalf("one local command error = %v", err)
	}
}

func writeConfig(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func validConfig(extra string) string {
	return `dataDirectory: state
mediaCatalogFile: maps/media-catalog.json
videoDirectories: [videos]
audioDirectories: [audio]
flacCacheFile: cache/flac_cache.json
playlistTemplate: "{playlistPath}"
videoPlaylistCommand: [mpv.exe]
videoPlaylistSuffix: _videos.m3u
videoSingleFileCommand: [mpv.exe]
localTrackingStartCommand: [foobar2000.exe, "{audioPath}"]
localTrackingStopCommand: [foobar2000.exe, /stop]
playlistTxtFilePath: imports/queue.txt
` + extra
}
