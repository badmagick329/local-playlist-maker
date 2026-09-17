package tracksession

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestWaitingLockReaderAllowsOwnerRelease(t *testing.T) {
	manifest := Manifest{SessionID: "first", LockPath: filepath.Join(t.TempDir(), "lock.json")}
	runner := Runner{IsAlive: func(int) bool { return true }}
	if err := runner.acquire(manifest); err != nil {
		t.Fatal(err)
	}
	reader, err := openLockReader(manifest.LockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	other := Runner{}
	if err := other.acquire(manifest); !errors.Is(err, errSessionBusy) {
		t.Fatalf("concurrent owner acquired: %v", err)
	}
	if err := runner.release(manifest.LockPath); err != nil {
		t.Fatalf("waiting reader blocked ownership release: %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	manifest.SessionID = "second"
	if err := runner.acquire(manifest); err != nil {
		t.Fatalf("next session could not acquire released ownership: %v", err)
	}
	runner.release(manifest.LockPath)
}
