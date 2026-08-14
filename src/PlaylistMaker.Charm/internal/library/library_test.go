package library

import (
	"testing"
	"time"
)

func TestGenerateUsesRequestedScale(t *testing.T) {
	tracks := Generate(1337, 6420)
	if len(tracks) != 1337 {
		t.Fatalf("got %d tracks", len(tracks))
	}
	variants := 0
	for _, track := range tracks {
		variants += len(track.Variants)
	}
	if variants != 6420 {
		t.Fatalf("got %d variants", variants)
	}
}

func TestFilterSearchesVariantFilenames(t *testing.T) {
	tracks := Generate(100, 400)
	enabled := map[Category]bool{}
	for _, category := range Categories {
		enabled[category] = true
	}
	result := FilterAndSort(tracks, Query{SearchText: "performance 02", Enabled: enabled, Sort: ModifiedNewest})
	if len(result) == 0 {
		t.Fatal("expected a variant filename match")
	}
}

func TestParseDateRangeIncludesRequestedPrecision(t *testing.T) {
	tests := []struct {
		value    string
		contains time.Time
	}{
		{"2026", time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC)},
		{"2026-03", time.Date(2026, 3, 31, 23, 59, 59, 0, time.UTC)},
		{"2026-03-14", time.Date(2026, 3, 14, 23, 59, 59, 0, time.UTC)},
		{"2026-03..2026-04", time.Date(2026, 4, 30, 23, 59, 59, 0, time.UTC)},
		{"2026-03-14..2026-03-15", time.Date(2026, 3, 15, 23, 59, 59, 0, time.UTC)},
	}
	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			rangeValue, err := ParseDateRange(test.value)
			if err != nil || !rangeValue.Contains(test.contains) {
				t.Fatalf("ParseDateRange(%q) = %#v, %v", test.value, rangeValue, err)
			}
		})
	}
}

func BenchmarkFilterAndSortParityScale(b *testing.B) {
	tracks := Generate(1337, 6420)
	enabled := map[Category]bool{}
	for _, category := range Categories {
		enabled[category] = true
	}
	b.ResetTimer()
	for range b.N {
		FilterAndSort(tracks, Query{SearchText: "aes kiss", Enabled: enabled, Sort: ModifiedNewest})
	}
}
