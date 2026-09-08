package tracksession

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLockReaderWaitsForPendingDeletion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock.json")
	if err := os.WriteFile(path, []byte("lock"), 0o600); err != nil {
		t.Fatal(err)
	}
	reader, err := openLockReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	// Keep deletion pending until the next reader has entered the handoff.
	closed := make(chan error, 1)
	time.AfterFunc(10*time.Millisecond, func() { closed <- reader.Close() })
	next, openErr := openLockReader(path)
	if next != nil {
		_ = next.Close()
	}
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
	if !os.IsNotExist(openErr) {
		t.Fatalf("pending deletion did not resolve to a released lock: %v", openErr)
	}
}
