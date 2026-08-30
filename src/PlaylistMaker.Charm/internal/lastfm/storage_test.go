package lastfm

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicWriteRetriesTransientRenameFailure(t *testing.T) {
	original := renameFile
	t.Cleanup(func() { renameFile = original })
	attempts := 0
	renameFile = func(source, destination string) error {
		attempts++
		if attempts < 3 {
			return errors.New("sharing violation")
		}
		return os.Rename(source, destination)
	}
	path := filepath.Join(t.TempDir(), "checkpoint.json")
	if err := atomicWrite(path, func(file *os.File) error { _, err := file.WriteString("saved"); return err }); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "saved" || attempts != 3 {
		t.Fatalf("contents=%q attempts=%d", contents, attempts)
	}
}
