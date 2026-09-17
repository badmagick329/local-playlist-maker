package tracksession

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"playlistmaker/charm/internal/tracking"
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
	Notify     func()
	Now        func() time.Time
	leases     map[string]*os.File
}

func (r Runner) Run(ctx context.Context, manifestPath string) error {
	manifest, err := ReadManifest(manifestPath)
	if err != nil {
		return err
	}
	if manifest.HelperProcessID != 0 && manifest.HelperProcessID != os.Getpid() && r.alive(manifest.HelperProcessID) {
		return fmt.Errorf("tracking helper is still running; close the unresponsive helper before retry")
	}
	if r.Runtime == nil {
		return fmt.Errorf("tracking runtime is unavailable")
	}
	guard := manifest
	guard.LockPath = manifestPath + ".helper-lock"
	if err := r.acquire(guard); err != nil {
		return err
	}
	defer r.release(guard.LockPath)
	ownsLock := false
	defer func() {
		if ownsLock {
			closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			r.Runtime.Close(closeCtx)
			_ = r.release(manifest.LockPath)
		}
	}()
	manifest.HelperProcessID = os.Getpid()
	if err = WriteManifest(manifestPath, manifest); err != nil {
		return err
	}
	r.Runtime.AllowUntracked, r.Runtime.DiagnosticsPath = manifest.AllowUntracked, manifest.DiagnosticsPath
	if err = r.Runtime.Prepare(ctx, r.DeviceName, manifest.Entries); err != nil {
		_ = WriteReady(manifest.ReadyPath, Ready{Error: err.Error()})
		return err
	}
	queue := playQueue{runtime: r.Runtime}
	offset, inputEnded, err := queue.restore(manifest.CheckpointPath, manifest)
	if err != nil {
		return err
	}
	restored := queue.blocked != ""
	var blockedError error
	notified := false
	if err = WriteReady(manifest.ReadyPath, Ready{Ready: true}); err != nil {
		return err
	}
	poll := r.Poll
	if poll == 0 {
		poll = 100 * time.Millisecond
	}
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	seen := map[string]bool{}
	revision := 0
	lastStatus := ""
	lastSave := time.Time{}
	lastHeartbeat := time.Time{}
	retry := false
	untracked := false
	parked := false
	pauseSince := time.Time{}
	publish := func() error {
		status := queue.status(manifest.SessionID, revision)
		if !ownsLock && queue.blocked == "" && !untracked && !parked {
			status.State, status.Message, status.Hold = "waiting", "Waiting for tracking ownership", true
		}
		key := fmt.Sprintf("%s/%s/%d/%s", status.State, status.OccurrenceID, status.IntentSequence, status.Message)
		changed := key != lastStatus
		if changed {
			revision++
			status.Revision = revision
			lastStatus = key
			target := "unknown"
			if queue.target != nil {
				target = fmt.Sprint(*queue.target)
			}
			r.Runtime.observe(-1, "", fmt.Sprintf("session=%s occurrence=%s intent=%d paused=%t target=%v state=%s: %s", manifest.SessionID, status.OccurrenceID, status.IntentSequence, queue.paused, target, status.State, status.Message))
			_ = atomicWrite(filepath.Join(filepath.Dir(manifest.LockPath), "tracking-error.txt"), []byte(status.Message+" (mpv: Play retries; Ctrl+Shift+U continues without tracking)"), 0600)
		}
		if r.now().Sub(lastHeartbeat) >= time.Second || changed {
			lastHeartbeat = r.now()
			return writeJSON(manifest.StatusPath, status)
		}
		return nil
	}
	if p, ok := r.Runtime.Spotify.(interface{ SetProtection(func() error) }); ok {
		p.SetProtection(publish)
	}
	block := func(failure error) {
		if queue.blocked == "" {
			queue.blocked = failure.Error()
			blockedError = failure
			if !notified && r.Notify != nil {
				r.Notify()
			}
			notified = true
		}
		_ = publish() // Put the video hold on disk before any bounded cleanup request.
		// Suspend command ownership. Future retries must reacquire and inspect the
		// same occurrence before issuing any resume command.
		if ownsLock {
			stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			if r.Runtime.active != nil {
				_ = r.Runtime.active.Stop(stopCtx)
			}
			cancel()
			_ = r.release(manifest.LockPath)
			ownsLock = false
			queue.suspended = true
		}
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if _, err := os.Stat(manifest.CancelPath); err == nil {
				return nil
			}
			if latest, e := ReadManifest(manifestPath); e == nil {
				manifest = latest
			}
			if manifest.MPVProcessID == 0 && r.now().Sub(manifest.CreatedAtUTC) > 30*time.Second {
				return fmt.Errorf("mpv did not attach to tracking session")
			}
			events, next, e := readEvents(manifest.EventPath, offset)
			if e != nil {
				return e
			}
			for _, event := range events {
				if event.EventID == "" || seen[event.EventID] {
					continue
				}
				seen[event.EventID] = true
				if event.SessionID != "" && event.SessionID != manifest.SessionID {
					continue
				}
				switch event.Event {
				case "file-loaded", "playback-repeat":
					reason := "stop"
					if event.Event == "playback-repeat" {
						reason = "eof"
					}
					queue.end(ctx, reason)
					if event.PlaylistPosition >= 0 && event.PlaylistPosition < len(manifest.Entries) {
						entry := manifest.Entries[event.PlaylistPosition]
						play := &queuedPlay{id: event.EventID, position: event.PlaylistPosition, track: entry.Track, videoStartedAt: r.now()}
						queue.video = play
						queue.pending = append(queue.pending, play)
						queue.paused, queue.target = event.Paused, event.PositionMS
						addPosition(&manifest.LoadedPositions, event.PlaylistPosition)
					}
				case "intent":
					if queue.intent(event) {
						parked = false
						if !event.Paused {
							retry = true
						}
					}
				case "retry":
					if queue.video != nil && event.OccurrenceID == queue.video.id {
						retry = true
					}
				case "health-failure":
					notified = true // mpv already sent this incident's desktop notification.
					r.Runtime.observe(-1, "", "mpv detected stale helper heartbeat")
				case "hold-ack":
					if queue.video != nil && event.OccurrenceID == queue.video.id && event.StatusRevision == revision {
						r.Runtime.observe(event.PlaylistPosition, "", fmt.Sprintf("hold acknowledged occurrence=%s revision=%d actualPaused=%t", event.OccurrenceID, revision, event.Paused))
					}
				case "untracked":
					if queue.video != nil && event.OccurrenceID == queue.video.id {
						if ownsLock {
							r.Runtime.End(ctx)
							_ = r.release(manifest.LockPath)
							ownsLock = false
						}
						untracked = true
						queue.active = nil
						queue.pending = nil
						queue.blocked = ""
					}
				case "end-file":
					parked = false
					addPosition(&manifest.TerminalPositions, event.PlaylistPosition)
					if queue.video != nil && queue.video.position == event.PlaylistPosition {
						reason := event.EndReason
						if event.Completed {
							reason = "eof"
						}
						queue.end(ctx, reason)
					}
				case "shutdown":
					parked = false
					manifest.ShutdownSeen = true
					reason := "quit"
					if event.Completed {
						reason = "eof"
					}
					queue.end(ctx, reason)
					inputEnded = true
				}
			}
			offset = next
			if len(events) > 0 {
				if err = WriteManifest(manifestPath, manifest); err != nil {
					return err
				}
			}
			if manifest.MPVProcessID != 0 && !r.alive(manifest.MPVProcessID) && !inputEnded {
				if err = recoverHistory(manifest); err != nil {
					return err
				}
				queue.end(ctx, "quit")
				manifest.ShutdownSeen = true
				if err = WriteManifest(manifestPath, manifest); err != nil {
					return err
				}
				inputEnded = true
			}
			if untracked {
				queue.pending = nil
				queue.active = nil
			} else {
				if queue.blocked != "" && retry && restored {
					if acquireErr := r.acquire(manifest); acquireErr == nil {
						ownsLock = true
						if p, ok := r.Runtime.Spotify.(recoverablePlayer); ok && queue.active != nil && len(queue.evidence) > 0 {
							restoreCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
							restoreErr := p.Restore(restoreCtx, queue.evidence)
							cancel()
							if restoreErr == nil {
								restored = false
								queue.suspended = false
								r.Runtime.active = r.Runtime.Spotify
							} else {
								queue.blocked = restoreErr.Error()
							}
						} else if queue.active == nil && len(queue.pending) == 0 {
							restored = false
						}
						if restored {
							_ = r.release(manifest.LockPath)
							ownsLock = false
						}
					}
				}
				if queue.blocked != "" && retry && !restored {
					queue.blocked = ""
					if p, ok := r.Runtime.Spotify.(controlledPlayer); ok {
						p.Retry()
					}
				}
				retry = false
				if queue.blocked == "" && !ownsLock && !parked {
					if err = r.acquire(manifest); err == nil {
						ownsLock = true
						queue.suspended = false
						if queue.active != nil && r.Runtime.activeProvider == "spotify" {
							if p, ok := r.Runtime.Spotify.(interface{ Reacquire() error }); ok {
								if e := p.Reacquire(); e != nil {
									block(e)
								}
							}
						}
					} else if !errors.Is(err, errSessionBusy) {
						block(err)
					}
				}
				if ownsLock && queue.blocked == "" {
					// Journal before external commands: a crash in a start/finish transaction
					// leaves ambiguity for intervention, never permission to replay.
					if err = queue.save(manifest.CheckpointPath, offset, inputEnded); err != nil {
						return err
					}
					if err = queue.tick(ctx); err != nil {
						block(err)
					} else if status := queue.status(manifest.SessionID, revision); !status.Hold {
						notified = false
					}
				}
			}
			if ownsLock && queue.blocked == "" && queue.paused {
				state := queue.status(manifest.SessionID, revision)
				if state.State == "user-paused" && (queue.active == nil || func() bool {
					p, ok := r.Runtime.Spotify.(controlledPlayer)
					if !ok {
						return false
					}
					phase, _ := p.TrackingStatus()
					return phase == "paused"
				}()) {
					if pauseSince.IsZero() {
						pauseSince = r.now()
					}
					if r.now().Sub(pauseSince) >= time.Minute {
						_ = r.release(manifest.LockPath)
						ownsLock = false
						queue.suspended = true
						parked = true
					}
				} else {
					pauseSince = time.Time{}
				}
			} else if !parked {
				pauseSince = time.Time{}
			}
			if err = publish(); err != nil {
				return err
			}
			if r.now().Sub(lastSave) >= time.Second || len(events) > 0 {
				if err = queue.save(manifest.CheckpointPath, offset, inputEnded); err != nil {
					return err
				}
				lastSave = r.now()
			}
			if inputEnded {
				if queue.blocked != "" {
					if blockedError != nil {
						return blockedError
					}
					return &tracking.PlaybackFailure{Message: queue.blocked}
				}
				if queue.idle() {
					Cleanup(manifestPath, manifest)
					_ = atomicWrite(filepath.Join(filepath.Dir(manifest.LockPath), "tracking-error.txt"), nil, 0600)
					return nil
				}
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

func (r *Runner) acquire(manifest Manifest) error {
	file, err := openLease(manifest.LockPath + ".lease")
	if err != nil {
		return err
	}
	contents, _ := json.Marshal(Lock{SessionID: manifest.SessionID, HelperPID: os.Getpid()})
	if err = atomicWrite(manifest.LockPath, append(contents, '\n'), 0600); err != nil {
		file.Close()
		return err
	}
	if r.leases == nil {
		r.leases = map[string]*os.File{}
	}
	r.leases[manifest.LockPath] = file
	return nil
}

func (r *Runner) release(path string) error {
	file := r.leases[path]
	if file == nil {
		return nil
	}
	delete(r.leases, path)
	err := os.Remove(path)
	return errors.Join(err, file.Close())
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

func (r Runner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}
