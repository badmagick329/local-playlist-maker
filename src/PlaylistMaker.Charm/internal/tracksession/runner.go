package tracksession

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

type Lock struct {
	SessionID string `json:"sessionId"`
	HelperPID int    `json:"helperPid"`
}

var errSessionBusy = errors.New("another tracked playback session is active")

type Runner struct {
	Runtime    *Runtime
	IsAlive    func(int) bool
	Poll       time.Duration
	DeviceName string
	Terminate  func(int) error
}

func (r Runner) Run(ctx context.Context, manifestPath string) (runErr error) {
	manifest, err := ReadManifest(manifestPath)
	if err != nil {
		return err
	}
	defer func() {
		if runErr != nil && !errors.Is(runErr, context.Canceled) {
			_ = atomicWrite(filepath.Join(filepath.Dir(manifest.LockPath), "tracking-error.txt"), []byte("Tracking stopped: "+runErr.Error()), 0o600)
		}
	}()
	lockErr := r.acquire(manifest)
	if lockErr != nil && !errors.Is(lockErr, errSessionBusy) {
		_ = WriteReady(manifest.ReadyPath, Ready{Error: lockErr.Error()})
		return lockErr
	}
	ownsLock := lockErr == nil
	defer func() {
		if ownsLock {
			if err := os.Remove(manifest.LockPath); err != nil {
				runErr = errors.Join(runErr, fmt.Errorf("release tracking session lock: %w", err))
			}
		}
	}()
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
	defer func() {
		if ownsLock {
			closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			r.Runtime.Close(closeCtx)
		}
	}()
	poll := r.Poll
	if poll == 0 {
		poll = 100 * time.Millisecond
	}
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	offset := 0
	seen := map[string]bool{}
	mpvSeen := false
	queue := playQueue{runtime: r.Runtime}
	started := false
	inputEnded := false
	pauseOnFailure := func(err error) error {
		if !inputEnded && manifest.PausePath != "" {
			if pauseErr := atomicWrite(manifest.PausePath, []byte(err.Error()+"\n"), 0o600); pauseErr != nil {
				return fmt.Errorf("%w; request mpv pause: %v", err, pauseErr)
			}
		}
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if _, cancelErr := os.Stat(manifest.CancelPath); cancelErr == nil {
				cleanup = true
				return nil
			}
			// Let mpv launch while a previous session finishes its Spotify tail.
			// Its events remain on disk until this helper owns the tracking player.
			if !ownsLock {
				if err := r.acquire(manifest); errors.Is(err, errSessionBusy) {
					continue
				} else if err != nil {
					return err
				}
				ownsLock = true
			}
			if inputEnded {
				if err := queue.tick(ctx); err != nil {
					return err
				}
				if queue.idle() {
					cleanup = true
					return nil
				}
				continue
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
				case "file-loaded", "playback-repeat":
					if addPosition(&manifest.LoadedPositions, event.PlaylistPosition) {
						if err := WriteManifest(manifestPath, manifest); err != nil {
							return err
						}
					}
					if event.PlaylistPosition >= 0 && event.PlaylistPosition < len(manifest.Entries) {
						if event.Event == "playback-repeat" {
							queue.end(ctx, "eof")
						} else {
							queue.end(ctx, "stop")
						}
						entry := manifest.Entries[event.PlaylistPosition]
						if err := queue.load(ctx, event.EventID, event.PlaylistPosition, entry.Track); err != nil {
							if started {
								return pauseOnFailure(err)
							}
							if terminateErr := r.terminate(manifest.MPVProcessID); terminateErr != nil {
								return fmt.Errorf("%w; terminate mpv: %v", err, terminateErr)
							}
							return err
						}
						started = true
					}
				case "end-file":
					if addPosition(&manifest.TerminalPositions, event.PlaylistPosition) {
						if err := WriteManifest(manifestPath, manifest); err != nil {
							return err
						}
					}
					if queue.video != nil && queue.video.position == event.PlaylistPosition {
						reason := event.EndReason
						if event.Completed {
							reason = "eof"
						}
						queue.end(ctx, reason)
					}
				case "shutdown":
					manifest.ShutdownSeen = true
					if err := WriteManifest(manifestPath, manifest); err != nil {
						return err
					}
					if event.Completed {
						queue.end(ctx, "eof")
					} else {
						queue.end(ctx, "quit")
					}
					inputEnded = true
				}
			}
			if !inputEnded && manifest.MPVProcessID != 0 && !r.alive(manifest.MPVProcessID) {
				if err := recoverHistory(manifest); err != nil {
					return err
				}
				queue.end(ctx, "quit")
				inputEnded = true
			}
			if err := queue.tick(ctx); err != nil {
				return pauseOnFailure(err)
			}
			if inputEnded && queue.idle() {
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
	if contents, err := readLock(manifest.LockPath); err == nil {
		var lock Lock
		if len(contents) == 0 {
			return errSessionBusy
		}
		if json.Unmarshal(contents, &lock) == nil && lock.HelperPID != 0 && r.alive(lock.HelperPID) {
			return errSessionBusy
		}
		_ = os.Remove(manifest.LockPath)
	} else if !os.IsNotExist(err) {
		return err
	}
	contents, _ := json.Marshal(Lock{SessionID: manifest.SessionID, HelperPID: os.Getpid()})
	file, err := os.OpenFile(manifest.LockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if os.IsExist(err) {
		return errSessionBusy
	}
	if err != nil {
		return fmt.Errorf("acquire tracking session lock: %w", err)
	}
	defer file.Close()
	_, err = file.Write(append(contents, '\n'))
	return err
}

func readLock(path string) ([]byte, error) {
	file, err := openLockReader(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(file)
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
