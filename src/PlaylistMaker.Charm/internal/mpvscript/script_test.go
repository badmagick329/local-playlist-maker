package mpvscript

import (
	"os"
	"testing"
)

func TestEnsureInstallsAndReplacesTheEmbeddedScript(t *testing.T) {
	directory := t.TempDir()
	path, err := Ensure(directory)
	if err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != string(source) {
		t.Fatalf("installed script mismatch: %v", err)
	}
	if err := os.WriteFile(path, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	path, err = Ensure(directory)
	if err != nil {
		t.Fatal(err)
	}
	contents, err = os.ReadFile(path)
	if err != nil || string(contents) != string(source) {
		t.Fatalf("replaced script mismatch: %v", err)
	}
}
