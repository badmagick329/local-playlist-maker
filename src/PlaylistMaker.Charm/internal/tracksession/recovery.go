package tracksession

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"playlistmaker/charm/internal/history"
)

func recoverHistory(manifest Manifest) error {
	if !manifest.HistoryEnabled {
		return nil
	}
	existing, err := history.TerminalEntryIDs(manifest.HistoryPath, manifest.SessionID)
	if err != nil {
		return err
	}
	loaded, terminal := positions(manifest.LoadedPositions), positions(manifest.TerminalPositions)
	for _, entry := range manifest.Entries {
		if existing[entry.EntryID] || terminal[entry.PlaylistPosition] {
			continue
		}
		outcome := "not_started"
		if loaded[entry.PlaylistPosition] {
			outcome = "abandoned"
		}
		counted := false
		if err := history.Append(manifest.HistoryPath, history.Event{
			SchemaVersion: 3, Event: outcome, EventAtUTC: time.Now().UTC(), SessionID: manifest.SessionID,
			EntryID: entry.EntryID, PlaylistPosition: entry.PlaylistPosition, PlaylistSize: len(manifest.Entries),
			SelectionSource: "charm-tui", TrackID: entry.Track.TrackID, VideoPath: entry.VideoPath,
			AudioPath: entry.Track.LocalAudioPath, Artist: entry.Track.Artist, Title: entry.Track.Title,
			EndReason: "mpv-process-exited-without-shutdown", CountedAsPlayed: &counted,
		}); err != nil {
			return err
		}
		existing[entry.EntryID] = true
	}
	return nil
}

func RecoverStale(_ context.Context, dataDirectory string, alive func(int) bool) error {
	if alive == nil {
		alive = processAlive
	}
	paths, err := filepath.Glob(filepath.Join(dataDirectory, "tracking-sessions", "*.manifest.json"))
	if err != nil {
		return err
	}
	for _, path := range paths {
		manifest, readErr := ReadManifest(path)
		if readErr != nil {
			return fmt.Errorf("read stale tracking manifest %q: %w", path, readErr)
		}
		helperPID := manifest.HelperProcessID
		if helperPID == 0 {
			if contents, lockErr := os.ReadFile(manifest.LockPath); lockErr == nil {
				var lock Lock
				if json.Unmarshal(contents, &lock) == nil && lock.SessionID == manifest.SessionID {
					helperPID = lock.HelperPID
				}
			}
		}
		if helperPID > 0 && alive(helperPID) || manifest.MPVProcessID > 0 && alive(manifest.MPVProcessID) {
			continue
		}
		events, _, readErr := readEvents(manifest.EventPath, 0)
		if readErr != nil && !os.IsNotExist(readErr) {
			return readErr
		}
		for _, event := range events {
			switch event.Event {
			case "file-loaded":
				addPosition(&manifest.LoadedPositions, event.PlaylistPosition)
			case "end-file":
				addPosition(&manifest.TerminalPositions, event.PlaylistPosition)
			case "shutdown":
				manifest.ShutdownSeen = true
			}
		}
		if !manifest.ShutdownSeen {
			if err := recoverHistory(manifest); err != nil {
				return err
			}
		}
		Cleanup(path, manifest)
	}
	return nil
}

func positions(values []int) map[int]bool {
	result := map[int]bool{}
	for _, value := range values {
		result[value] = true
	}
	return result
}
