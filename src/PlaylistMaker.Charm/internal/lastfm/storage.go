package lastfm

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

var renameFile = os.Rename

func readScrobbles(path string) ([]Scrobble, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read Last.fm scrobbles: %w", err)
	}
	defer f.Close()
	var result []Scrobble
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		var v Scrobble
		if err := json.Unmarshal(scanner.Bytes(), &v); err != nil {
			return nil, fmt.Errorf("parse Last.fm scrobbles: %w", err)
		}
		if v.SchemaVersion != SchemaVersion {
			return nil, fmt.Errorf("Last.fm scrobbles schemaVersion is %d, want %d", v.SchemaVersion, SchemaVersion)
		}
		result = append(result, v)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read Last.fm scrobbles: %w", err)
	}
	return result, nil
}

func writeScrobbles(path string, values []Scrobble) error {
	sort.Slice(values, func(i, j int) bool { return values[i].PlayedAtUTC.Before(values[j].PlayedAtUTC) })
	return atomicWrite(path, func(f *os.File) error {
		enc := json.NewEncoder(f)
		for _, v := range values {
			v.SchemaVersion = SchemaVersion
			if err := enc.Encode(v); err != nil {
				return err
			}
		}
		return nil
	})
}

func readCheckpointScrobbles(path, syncID string) ([]Scrobble, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read Last.fm sync checkpoint: %w", err)
	}
	defer f.Close()
	var result []Scrobble
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		var record checkpointScrobble
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return nil, fmt.Errorf("parse Last.fm sync checkpoint: %w", err)
		}
		if record.SchemaVersion != SchemaVersion {
			return nil, fmt.Errorf("Last.fm sync checkpoint record schemaVersion is %d, want %d", record.SchemaVersion, SchemaVersion)
		}
		if record.Scrobble.SchemaVersion != SchemaVersion {
			return nil, fmt.Errorf("Last.fm sync checkpoint scrobble schemaVersion is %d, want %d", record.Scrobble.SchemaVersion, SchemaVersion)
		}
		if record.SyncID == syncID {
			result = append(result, record.Scrobble)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read Last.fm sync checkpoint: %w", err)
	}
	return result, nil
}

func appendCheckpointScrobbles(path, syncID string, values []Scrobble) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open Last.fm sync checkpoint: %w", err)
	}
	enc := json.NewEncoder(f)
	for _, value := range values {
		value.SchemaVersion = SchemaVersion
		if err = enc.Encode(checkpointScrobble{SchemaVersion: SchemaVersion, SyncID: syncID, Scrobble: value}); err != nil {
			break
		}
	}
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("write Last.fm sync checkpoint: %w", err)
	}
	return nil
}

func readJSON(path string, value any, label string) error {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", label, err)
	}
	if err = json.Unmarshal(b, value); err != nil {
		return fmt.Errorf("parse %s: %w", label, err)
	}
	return nil
}
func writeJSON(path string, value any) error {
	return atomicWrite(path, func(f *os.File) error { enc := json.NewEncoder(f); enc.SetIndent("", "  "); return enc.Encode(value) })
}
func atomicWrite(path string, write func(*os.File) error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".lastfm-*.tmp")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = write(f); err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return renameWithRetry(name, path)
}

func renameWithRetry(source, destination string) error {
	deadline := time.Now().Add(3 * time.Second)
	var err error
	for {
		if err = renameFile(source, destination); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return err
		}
		time.Sleep(25 * time.Millisecond)
	}
}
