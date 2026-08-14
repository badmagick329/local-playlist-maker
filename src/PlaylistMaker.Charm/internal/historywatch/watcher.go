package historywatch

import (
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watcher coalesces relevant history-file changes from its containing directory.
// Watching the directory keeps history replacement and first-file creation visible.
type Watcher struct {
	historyPath string
	events      <-chan fsnotify.Event
	errors      <-chan error
	closeSource func() error
	newTimer    func(time.Duration) debounceTimer
	delay       time.Duration

	changes chan struct{}
	done    chan struct{}
	closed  sync.Once
	wg      sync.WaitGroup
}

type debounceTimer interface {
	C() <-chan time.Time
	Stop() bool
	Reset(time.Duration) bool
}

type standardTimer struct{ *time.Timer }

func (t standardTimer) C() <-chan time.Time { return t.Timer.C }

// New watches the directory containing historyPath. Its notifications are
// debounced so a JSONL append, replace, or rename causes one refresh.
func New(historyPath string, delay time.Duration) (*Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := watcher.Add(filepath.Dir(historyPath)); err != nil {
		_ = watcher.Close()
		return nil, err
	}
	return newWatcher(historyPath, watcher.Events, watcher.Errors, watcher.Close, delay, func(delay time.Duration) debounceTimer {
		return standardTimer{time.NewTimer(delay)}
	}), nil
}

func newWatcher(historyPath string, events <-chan fsnotify.Event, errors <-chan error, closeSource func() error, delay time.Duration, newTimer func(time.Duration) debounceTimer) *Watcher {
	w := &Watcher{
		historyPath: filepath.Clean(historyPath),
		events:      events,
		errors:      errors,
		closeSource: closeSource,
		newTimer:    newTimer,
		delay:       delay,
		changes:     make(chan struct{}, 1),
		done:        make(chan struct{}),
	}
	w.wg.Add(1)
	go w.run()
	return w
}

// Changes emits a coalesced notification when the history file changes.
func (w *Watcher) Changes() <-chan struct{} { return w.changes }

// Close stops the directory watcher and waits for its goroutine to exit.
func (w *Watcher) Close() error {
	var err error
	w.closed.Do(func() {
		close(w.done)
		if w.closeSource != nil {
			err = w.closeSource()
		}
		w.wg.Wait()
		close(w.changes)
	})
	return err
}

func (w *Watcher) run() {
	defer w.wg.Done()
	var timer debounceTimer
	var timerC <-chan time.Time
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()

	for {
		select {
		case <-w.done:
			return
		case event, ok := <-w.events:
			if !ok {
				return
			}
			if !w.relevant(event) {
				continue
			}
			if timer == nil {
				timer = w.newTimer(w.delay)
			} else {
				timer.Reset(w.delay)
			}
			timerC = timer.C()
		case <-timerC:
			timerC = nil
			select {
			case w.changes <- struct{}{}:
			default:
			}
		case _, ok := <-w.errors:
			if !ok {
				return
			}
			// fsnotify errors are transient. The manual refresh remains available.
		}
	}
}

func (w *Watcher) relevant(event fsnotify.Event) bool {
	if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename|fsnotify.Remove) == 0 {
		return false
	}
	name := filepath.Clean(event.Name)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(name, w.historyPath)
	}
	return name == w.historyPath
}
