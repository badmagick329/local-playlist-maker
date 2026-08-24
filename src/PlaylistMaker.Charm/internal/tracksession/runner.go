package tracksession

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type Lock struct {
	SessionID string `json:"sessionId"`
	HelperPID int    `json:"helperPid"`
}

type Runner struct {
	Runtime    *Runtime
	IsAlive    func(int) bool
	Poll       time.Duration
	DeviceName string
	Terminate  func(int) error
}

func (r Runner) Run(ctx context.Context, manifestPath string) error {
	manifest, err := ReadManifest(manifestPath)
	if err != nil {
		return err
	}
	if err := r.acquire(manifest); err != nil {
		_ = WriteReady(manifest.ReadyPath, Ready{Error: err.Error()})
		return err
	}
	defer os.Remove(manifest.LockPath)
	cleanup := false
	defer func() {
		if cleanup {
			Cleanup(manifestPath, manifest)
		}
	}()
	manifest.HelperProcessID = os.Getpid()
	if err := WriteManifest(manifestPath, manifest); err != nil {
		_ = WriteReady(manifest.ReadyPath, Ready{Error: err.Error()})
		return err
	}
	if r.Runtime == nil {
		err = fmt.Errorf("tracking runtime is unavailable")
		_ = WriteReady(manifest.ReadyPath, Ready{Error: err.Error()})
		return err
	}
	r.Runtime.AllowUntracked = manifest.AllowUntracked
	r.Runtime.DiagnosticsPath = manifest.DiagnosticsPath
	if err := r.Runtime.Prepare(ctx, r.DeviceName, manifest.Entries); err != nil {
		_ = WriteReady(manifest.ReadyPath, Ready{Error: err.Error()})
		r.Runtime.Close(context.Background())
		return err
	}
	if err := WriteReady(manifest.ReadyPath, Ready{Ready: true}); err != nil {
		r.Runtime.Close(context.Background())
		return err
	}
	defer r.Runtime.Close(context.Background())
	poll := r.Poll
	if poll == 0 {
		poll = 100 * time.Millisecond
	}
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	offset := 0
	seen := map[string]bool{}
	mpvSeen := false
	activePosition := -1
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if _, cancelErr := os.Stat(manifest.CancelPath); cancelErr == nil {
				cleanup = true
				return nil
			}
			latest, readErr := ReadManifest(manifestPath)
			if readErr == nil {
				manifest = latest
				mpvSeen = mpvSeen || manifest.MPVProcessID != 0
			}
			if manifest.MPVProcessID == 0 {
				if time.Since(manifest.CreatedAtUTC) > 30*time.Second {
					return fmt.Errorf("mpv did not attach to the tracking session")
				}
				continue
			}
			events, next, readErr := readEvents(manifest.EventPath, offset)
			if readErr != nil {
				return readErr
			}
			offset = next
			for _, event := range events {
				if event.EventID == "" || seen[event.EventID] {
					continue
				}
				seen[event.EventID] = true
				switch event.Event {
				case "file-loaded":
					if addPosition(&manifest.LoadedPositions, event.PlaylistPosition) {
						if err := WriteManifest(manifestPath, manifest); err != nil {
							return err
						}
					}
					if event.PlaylistPosition != activePosition && event.PlaylistPosition >= 0 && event.PlaylistPosition < len(manifest.Entries) {
						entry := manifest.Entries[event.PlaylistPosition]
						if err := r.Runtime.Load(ctx, event.PlaylistPosition, entry.Track); err != nil {
							if terminateErr := r.terminate(manifest.MPVProcessID); terminateErr != nil {
								return fmt.Errorf("%w; terminate mpv: %v", err, terminateErr)
							}
							return err
						}
						activePosition = event.PlaylistPosition
					}
				case "end-file":
					if addPosition(&manifest.TerminalPositions, event.PlaylistPosition) {
						if err := WriteManifest(manifestPath, manifest); err != nil {
							return err
						}
					}
					if activePosition != -1 {
						r.Runtime.End(ctx)
						activePosition = -1
					}
				case "shutdown":
					manifest.ShutdownSeen = true
					if err := WriteManifest(manifestPath, manifest); err != nil {
						return err
					}
					cleanup = true
					return nil
				}
			}
			if manifest.MPVProcessID != 0 && !r.alive(manifest.MPVProcessID) {
				if err := recoverHistory(manifest); err != nil {
					return err
				}
				cleanup = true
				return nil
			}
			if !mpvSeen && time.Since(manifest.CreatedAtUTC) > 30*time.Second {
				return fmt.Errorf("mpv did not attach to the tracking session")
			}
		}
	}
}

func addPosition(values *[]int, position int) bool {
	if position < 0 {
		return false
	}
	for _, value := range *values {
		if value == position {
			return false
		}
	}
	*values = append(*values, position)
	return true
}

func (r Runner) acquire(manifest Manifest) error {
	if contents, err := os.ReadFile(manifest.LockPath); err == nil {
		var lock Lock
		if json.Unmarshal(contents, &lock) == nil && lock.HelperPID != 0 && r.alive(lock.HelperPID) {
			return fmt.Errorf("another tracked playback session is active")
		}
		_ = os.Remove(manifest.LockPath)
	} else if !os.IsNotExist(err) {
		return err
	}
	contents, _ := json.Marshal(Lock{SessionID: manifest.SessionID, HelperPID: os.Getpid()})
	file, err := os.OpenFile(manifest.LockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("acquire tracking session lock: %w", err)
	}
	defer file.Close()
	_, err = file.Write(append(contents, '\n'))
	return err
}

func (r Runner) alive(pid int) bool {
	if r.IsAlive != nil {
		return r.IsAlive(pid)
	}
	return processAlive(pid)
}

func (r Runner) terminate(pid int) error {
	if r.Terminate != nil {
		return r.Terminate(pid)
	}
	return terminateProcess(pid)
}

func readEvents(path string, offset int) ([]Event, int, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, offset, err
	}
	if offset > len(contents) {
		offset = 0
	}
	data := contents[offset:]
	events := []Event{}
	consumed := 0
	for index, value := range data {
		if value != '\n' {
			continue
		}
		line := data[consumed:index]
		consumed = index + 1
		var event Event
		if json.Unmarshal(line, &event) == nil {
			events = append(events, event)
		}
	}
	return events, offset + consumed, nil
}
