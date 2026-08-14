package historywatch

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

type fakeTimer struct {
	channel chan time.Time
	resets  int
	reset   chan struct{}
}

func newFakeTimer() *fakeTimer {
	return &fakeTimer{channel: make(chan time.Time, 1), reset: make(chan struct{}, 4)}
}
func (t *fakeTimer) C() <-chan time.Time { return t.channel }
func (t *fakeTimer) Stop() bool          { return true }
func (t *fakeTimer) Reset(time.Duration) bool {
	t.resets++
	t.reset <- struct{}{}
	return true
}
func (t *fakeTimer) fire() { t.channel <- time.Now() }

func TestWatcherCoalescesRelevantDirectoryEvents(t *testing.T) {
	events := make(chan fsnotify.Event, 4)
	timer := newFakeTimer()
	historyPath := filepath.Join(t.TempDir(), "play-history.jsonl")
	w := newWatcher(historyPath, events, nil, func() error { return nil }, time.Second, func(time.Duration) debounceTimer { return timer })
	defer w.Close()

	events <- fsnotify.Event{Name: filepath.Join(filepath.Dir(historyPath), "unrelated.jsonl"), Op: fsnotify.Write}
	events <- fsnotify.Event{Name: historyPath, Op: fsnotify.Write}
	events <- fsnotify.Event{Name: historyPath, Op: fsnotify.Rename}

	select {
	case <-timer.reset:
	case <-time.After(time.Second):
		t.Fatal("rapid relevant notification did not reset the debounce timer")
	}
	if timer.resets != 1 {
		t.Fatalf("timer reset count = %d, want 1", timer.resets)
	}
	timer.fire()
	select {
	case <-w.Changes():
	case <-time.After(time.Second):
		t.Fatal("coalesced notification was not emitted")
	}
	select {
	case <-w.Changes():
		t.Fatal("rapid notifications were not coalesced")
	default:
	}
}

func TestWatcherCloseStopsGoroutineAndClosesChanges(t *testing.T) {
	events := make(chan fsnotify.Event)
	w := newWatcher(filepath.Join(t.TempDir(), "play-history.jsonl"), events, nil, func() error { return nil }, time.Second, func(time.Duration) debounceTimer { return newFakeTimer() })
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if _, open := <-w.Changes(); open {
		t.Fatal("changes channel stayed open after Close")
	}
}
