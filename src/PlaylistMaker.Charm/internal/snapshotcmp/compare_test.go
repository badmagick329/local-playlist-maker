package snapshotcmp

import (
	"strings"
	"testing"
	"time"

	"playlistmaker/charm/internal/backend"
	"playlistmaker/charm/internal/library"
)

func TestCompareReportsBoundedStructuralDifferences(t *testing.T) {
	base := snapshot(25)
	changed := snapshot(25)
	for index := range changed.Tracks {
		changed.Tracks[index].Title = "different"
	}
	result := Compare(base, changed)
	if result.Total != 25 || len(result.Differences) != maxReportedDifferences || !strings.Contains(result.Differences[0], "title") {
		t.Fatalf("comparison = %#v", result)
	}
	if !Compare(base, base).Equal() {
		t.Fatal("identical snapshots differed")
	}
}

func TestCompareFindsVariantOrderAndFields(t *testing.T) {
	left, right := snapshot(1), snapshot(1)
	right.Tracks[0].Variants[0].Filename = "changed.mkv"
	result := Compare(left, right)
	if result.Total != 1 || !strings.Contains(result.Differences[0], "filename") {
		t.Fatalf("comparison = %#v", result)
	}
}

func snapshot(count int) backend.LibrarySnapshot {
	tracks := make([]library.Track, count)
	for index := range tracks {
		id := string(rune('a' + index%26))
		tracks[index] = library.Track{ID: id, Artist: "artist", Title: "title", ReleaseDateLabel: "2024", Variants: []library.Variant{{ID: id + "-video", VideoPath: id + ".mkv", AudioPath: id + ".flac", Filename: id + ".mkv", Category: library.MusicVideo, DateLabel: "2024-01-01", Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}}}
	}
	return backend.LibrarySnapshot{Tracks: tracks}
}
