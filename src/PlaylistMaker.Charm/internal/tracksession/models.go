package tracksession

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"playlistmaker/charm/internal/tracking"
)

type Entry struct {
	EntryID          string         `json:"entryId"`
	PlaylistPosition int            `json:"playlistPosition"`
	VideoPath        string         `json:"videoPath"`
	Track            tracking.Track `json:"track"`
}

type Manifest struct {
	SchemaVersion         int       `json:"schemaVersion"`
	SessionID             string    `json:"sessionId"`
	CreatedAtUTC          time.Time `json:"createdAtUtc"`
	ConfigPath            string    `json:"configPath"`
	EventPath             string    `json:"eventPath"`
	ReadyPath             string    `json:"readyPath"`
	CancelPath            string    `json:"cancelPath"`
	DiagnosticsPath       string    `json:"diagnosticsPath"`
	LockPath              string    `json:"lockPath"`
	SpotifyStatePath      string    `json:"spotifyStatePath"`
	HelperProcessID       int       `json:"helperProcessId,omitempty"`
	MPVProcessID          int       `json:"mpvProcessId,omitempty"`
	LoadedPositions       []int     `json:"loadedPositions,omitempty"`
	TerminalPositions     []int     `json:"terminalPositions,omitempty"`
	ShutdownSeen          bool      `json:"shutdownSeen,omitempty"`
	AllowUntracked        bool      `json:"allowUntracked"`
	HistoryEnabled        bool      `json:"historyEnabled"`
	HistoryPath           string    `json:"historyPath,omitempty"`
	MinimumWatchedPercent int       `json:"minimumWatchedPercent"`
	Entries               []Entry   `json:"entries"`
}

type Event struct {
	EventID          string    `json:"eventId"`
	Event            string    `json:"event"`
	EventAtUTC       time.Time `json:"eventAtUtc"`
	PlaylistPosition int       `json:"playlistPosition,omitempty"`
	EndReason        string    `json:"endReason,omitempty"`
}

type Ready struct {
	Ready bool   `json:"ready"`
	Error string `json:"error,omitempty"`
}

type Diagnostic struct {
	EventAtUTC       time.Time `json:"eventAtUtc"`
	PlaylistPosition int       `json:"playlistPosition,omitempty"`
	TrackID          string    `json:"trackId,omitempty"`
	Provider         string    `json:"provider,omitempty"`
	FallbackReason   string    `json:"fallbackReason,omitempty"`
	Error            string    `json:"error,omitempty"`
}

func Create(dataDirectory string, entries []Entry, allowUntracked, historyEnabled bool, historyPath string, minimum int) (string, Manifest, error) {
	id := fmt.Sprintf("%d", time.Now().UnixNano())
	directory := filepath.Join(dataDirectory, "tracking-sessions")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", Manifest{}, err
	}
	for index := range entries {
		entries[index].EntryID = fmt.Sprintf("%s-%d", id, index)
		entries[index].PlaylistPosition = index
	}
	manifest := Manifest{
		SchemaVersion: 1, SessionID: id, CreatedAtUTC: time.Now().UTC(), Entries: entries,
		EventPath: filepath.Join(directory, id+".events.jsonl"), ReadyPath: filepath.Join(directory, id+".ready.json"), CancelPath: filepath.Join(directory, id+".cancel"),
		DiagnosticsPath: filepath.Join(dataDirectory, "tracking-diagnostics.jsonl"), LockPath: filepath.Join(dataDirectory, "active-tracking-session.json"),
		SpotifyStatePath: filepath.Join(dataDirectory, "spotify-active-session.json"), AllowUntracked: allowUntracked,
		HistoryEnabled: historyEnabled, HistoryPath: historyPath, MinimumWatchedPercent: minimum,
	}
	if err := os.WriteFile(manifest.EventPath, nil, 0o600); err != nil {
		return "", Manifest{}, err
	}
	path := filepath.Join(directory, id+".manifest.json")
	if err := WriteManifest(path, manifest); err != nil {
		_ = os.Remove(manifest.EventPath)
		return "", Manifest{}, err
	}
	return path, manifest, nil
}

func ReadManifest(path string) (Manifest, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	var result Manifest
	if err := json.Unmarshal(contents, &result); err != nil {
		return Manifest{}, err
	}
	if result.SchemaVersion != 1 || result.SessionID == "" || len(result.Entries) == 0 {
		return Manifest{}, fmt.Errorf("invalid tracking manifest")
	}
	return result, nil
}

func WriteManifest(path string, manifest Manifest) error {
	contents, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, append(contents, '\n'), 0o600)
}

func WriteReady(path string, ready Ready) error {
	contents, err := json.Marshal(ready)
	if err != nil {
		return err
	}
	return atomicWrite(path, append(contents, '\n'), 0o600)
}

func AppendDiagnostic(path string, value Diagnostic) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	contents, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = file.Write(append(contents, '\n'))
	return err
}

func Cleanup(path string, manifest Manifest) {
	for _, target := range []string{path, manifest.EventPath, manifest.ReadyPath, manifest.CancelPath} {
		_ = os.Remove(target)
	}
}

func atomicWrite(path string, contents []byte, mode os.FileMode) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".tracking-*.tmp")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
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
