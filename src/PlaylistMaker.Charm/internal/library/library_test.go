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

func TestExplicitSortsRemainPrimaryDuringSearch(t *testing.T) {
	enabled := map[Category]bool{MusicVideo: true}
	tracks := []Track{
		{ID: "later", Artist: "match", Title: "match", ReleaseDate: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), Variants: []Variant{{ID: "later-video", Category: MusicVideo, Date: time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC), ModifiedAt: time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)}}, BaseSearchText: "match", SearchTextByCategory: map[Category]string{}},
		{ID: "earlier", Artist: "match match", Title: "match", ReleaseDate: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC), Variants: []Variant{{ID: "earlier-video", Category: MusicVideo, Date: time.Date(2025, 2, 2, 0, 0, 0, 0, time.UTC), ModifiedAt: time.Date(2025, 2, 2, 0, 0, 0, 0, time.UTC)}}, BaseSearchText: "match match", SearchTextByCategory: map[Category]string{}},
	}
	for _, test := range []struct {
		sort Sort
		want []string
	}{
		{ReleaseOldest, []string{"earlier", "later"}},
		{ReleaseNewest, []string{"later", "earlier"}},
		{VideoOldest, []string{"earlier", "later"}},
		{ModifiedNewest, []string{"later", "earlier"}},
	} {
		result := FilterAndSort(tracks, Query{SearchText: "match", Enabled: enabled, Sort: test.sort})
		if len(result) != 2 || result[0].ID != test.want[0] || result[1].ID != test.want[1] {
			t.Fatalf("%s with search = %#v, want %v", test.sort, result, test.want)
		}
	}
}

func TestRelevanceSortRequiresExplicitSelection(t *testing.T) {
	enabled := map[Category]bool{MusicVideo: true}
	tracks := []Track{
		{ID: "newer", Title: "match", Variants: []Variant{{ID: "newer-video", Category: MusicVideo, ModifiedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)}}, BaseSearchText: "prefix match", SearchTextByCategory: map[Category]string{}},
		{ID: "stronger", Title: "match", Variants: []Variant{{ID: "stronger-video", Category: MusicVideo, ModifiedAt: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)}}, BaseSearchText: "match", SearchTextByCategory: map[Category]string{}},
	}
	if got := FilterAndSort(tracks, Query{SearchText: "match", Enabled: enabled, Sort: ModifiedNewest}); got[0].ID != "newer" {
		t.Fatalf("modified order = %q", got[0].ID)
	}
	if got := FilterAndSort(tracks, Query{SearchText: "match", Enabled: enabled, Sort: Relevance}); got[0].ID != "stronger" {
		t.Fatalf("relevance order = %q", got[0].ID)
	}
	if got := FilterAndSort(tracks, Query{Enabled: enabled, Sort: Relevance}); got[0].ID != "newer" {
		t.Fatalf("empty relevance order = %q", got[0].ID)
	}
}

func TestTrackReleaseAndVideoDateFiltersCombine(t *testing.T) {
	enabled := map[Category]bool{MusicVideo: true}
	trackRange, _ := ParseDateRange("2025")
	videoRange, _ := ParseDateRange("2026")
	tracks := []Track{
		{ID: "both", ReleaseDate: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Variants: []Variant{{ID: "both-video", Category: MusicVideo, Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}}, SearchTextByCategory: map[Category]string{}},
		{ID: "track-only", ReleaseDate: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Variants: []Variant{{ID: "track-video", Category: MusicVideo, Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}}, SearchTextByCategory: map[Category]string{}},
		{ID: "video-only", ReleaseDate: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Variants: []Variant{{ID: "video-video", Category: MusicVideo, Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}}, SearchTextByCategory: map[Category]string{}},
	}
	if got := FilterAndSort(tracks, Query{Enabled: enabled, TrackRelease: trackRange, Sort: ModifiedNewest}); len(got) != 2 {
		t.Fatalf("track release filter got %d", len(got))
	}
	if got := FilterAndSort(tracks, Query{Enabled: enabled, VideoDate: videoRange, Sort: ModifiedNewest}); len(got) != 2 {
		t.Fatalf("video date filter got %d", len(got))
	}
	if got := FilterAndSort(tracks, Query{Enabled: enabled, TrackRelease: trackRange, VideoDate: videoRange, Sort: ModifiedNewest}); len(got) != 1 || got[0].ID != "both" {
		t.Fatalf("combined filters = %#v", got)
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
