package tracksession

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"
)

type Handle interface {
	PID() int
	Cancel() error
}

type Starter interface {
	Start(context.Context, string, Manifest) (Handle, error)
}

type OSStarter struct {
	Executable string
	Timeout    time.Duration
}

type processHandle struct {
	command    *exec.Cmd
	cancelPath string
	done       chan struct{}
}

func (h processHandle) PID() int { return h.command.Process.Pid }
func (h processHandle) Cancel() error {
	if err := os.WriteFile(h.cancelPath, nil, 0o600); err != nil {
		return err
	}
	select {
	case <-h.done:
		return nil
	case <-time.After(5 * time.Second):
		return h.command.Process.Kill()
	}
}

func (s OSStarter) Start(ctx context.Context, manifestPath string, manifest Manifest) (Handle, error) {
	executable := s.Executable
	if executable == "" {
		var err error
		executable, err = os.Executable()
		if err != nil {
			return nil, err
		}
	}
	command := exec.Command(executable, "--track-session", manifestPath)
	configureHidden(command)
	if err := command.Start(); err != nil {
		return nil, err
	}
	handle := processHandle{command: command, cancelPath: manifest.CancelPath, done: make(chan struct{})}
	go func() {
		_ = command.Wait()
		close(handle.done)
	}()
	timeout := s.Timeout
	if timeout == 0 {
		timeout = 15 * time.Second
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			_ = handle.Cancel()
			return nil, ctx.Err()
		case <-deadline.C:
			_ = handle.Cancel()
			return nil, fmt.Errorf("tracking helper did not become ready")
		case <-handle.done:
			return nil, fmt.Errorf("tracking helper exited before becoming ready")
		case <-ticker.C:
			contents, err := os.ReadFile(manifest.ReadyPath)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				_ = handle.Cancel()
				return nil, err
			}
			var ready Ready
			if json.Unmarshal(contents, &ready) != nil {
				continue
			}
			if !ready.Ready {
				_ = handle.Cancel()
				return nil, fmt.Errorf("tracking helper initialization failed: %s", ready.Error)
			}
			return handle, nil
		}
	}
}
