package pathid

import (
	"path/filepath"
	"testing"
)

func TestComparisonKeyNormalizesWindowsLibraryIdentities(t *testing.T) {
	tests := []struct {
		name  string
		left  string
		right string
	}{
		{"case and separators", `C:\Music\Library\Track.flac`, `c:/music/Library/track.flac`},
		{"trailing separator", `C:\Music\Album\\`, `c:/music/album`},
		{"unicode", `C:\Music\나연\Pop!.flac`, `c:/music/나연/pop!.flac`},
		{"legacy library replacement", `C:\Music\Library\Track.flac`, `C:\Music/Library/Track.flac`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if left, right := ComparisonKey(test.left), ComparisonKey(test.right); left != right {
				t.Fatalf("comparison keys differ: %q != %q", left, right)
			}
		})
	}
}

func TestResolveCleansDotsWithoutLowercasingDisplayPath(t *testing.T) {
	base := filepath.Join(t.TempDir(), "Folder")
	got := Resolve(base, filepath.Join("child", "..", "나연", "Pop!.flac"))
	want := filepath.Join(base, "나연", "Pop!.flac")
	if got != want {
		t.Fatalf("resolved path = %q, want %q", got, want)
	}
}
