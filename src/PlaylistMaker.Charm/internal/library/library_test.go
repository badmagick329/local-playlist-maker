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

func TestSearchRelevanceIsPrimaryForEverySort(t *testing.T) {
	enabled := map[Category]bool{MusicVideo: true}
	tracks := []Track{
		{ID: "delulu", Artist: "older", Title: "Delulu", ReleaseDate: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC), Variants: []Variant{{ID: "delulu-video", Category: MusicVideo, Date: time.Date(2025, 2, 2, 0, 0, 0, 0, time.UTC), ModifiedAt: time.Date(2025, 2, 2, 0, 0, 0, 0, time.UTC)}}, BaseSearchText: "delulu", SearchTextByCategory: map[Category]string{}},
		{ID: "newer", Artist: "newer", Title: "Loose match", ReleaseDate: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), Variants: []Variant{{ID: "newer-video", Category: MusicVideo, Date: time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC), ModifiedAt: time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)}}, BaseSearchText: "distant elephant lunar umbrella", SearchTextByCategory: map[Category]string{}},
	}
	for _, selected := range Sorts {
		result := FilterAndSort(tracks, Query{SearchText: "delu", Enabled: enabled, Sort: selected})
		if len(result) != 2 || result[0].ID != "delulu" {
			t.Fatalf("%s with search = %#v", selected, result)
		}
	}
}

func TestSelectedSortOrdersEqualScoreSearchMatches(t *testing.T) {
	enabled := map[Category]bool{MusicVideo: true}
	tracks := []Track{
		{ID: "later", ReleaseDate: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), Variants: []Variant{{ID: "later-video", Category: MusicVideo, ModifiedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)}}, BaseSearchText: "delu", SearchTextByCategory: map[Category]string{}},
		{ID: "earlier", ReleaseDate: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC), Variants: []Variant{{ID: "earlier-video", Category: MusicVideo, ModifiedAt: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)}}, BaseSearchText: "delu", SearchTextByCategory: map[Category]string{}},
	}
	if got := FilterAndSort(tracks, Query{SearchText: "delu", Enabled: enabled, Sort: ReleaseOldest}); got[0].ID != "earlier" {
		t.Fatalf("equal-score release order = %q", got[0].ID)
	}
	if got := FilterAndSort(tracks, Query{Enabled: enabled, Sort: ReleaseNewest}); got[0].ID != "later" {
		t.Fatalf("empty-search release order = %q", got[0].ID)
	}
	if got := FilterAndSort(tracks, Query{Enabled: enabled, Sort: Relevance}); got[0].ID != "later" {
		t.Fatalf("empty relevance order = %q", got[0].ID)
	}
	ties := []Track{
		{ID: "z", Variants: []Variant{{ID: "z-video", Category: MusicVideo}}, BaseSearchText: "delu", SearchTextByCategory: map[Category]string{}},
		{ID: "a", Variants: []Variant{{ID: "a-video", Category: MusicVideo}}, BaseSearchText: "delu", SearchTextByCategory: map[Category]string{}},
	}
	if got := FilterAndSort(ties, Query{SearchText: "delu", Enabled: enabled, Sort: ModifiedNewest}); got[0].ID != "a" {
		t.Fatalf("stable tie order = %q", got[0].ID)
	}
}

func TestSearchRespectsCategoryAndVideoDateEligibility(t *testing.T) {
	enabled := map[Category]bool{MusicVideo: true}
	date, _ := ParseDateRange("2025")
	tracks := []Track{
		{ID: "eligible", BaseSearchText: "delulu", Variants: []Variant{{ID: "eligible-video", Category: MusicVideo, Date: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}}, SearchTextByCategory: map[Category]string{}},
		{ID: "wrong-category", BaseSearchText: "delulu", Variants: []Variant{{ID: "wrong-category-video", Category: Performance, Date: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}}, SearchTextByCategory: map[Category]string{}},
		{ID: "wrong-date", BaseSearchText: "delulu", Variants: []Variant{{ID: "wrong-date-video", Category: MusicVideo, Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}}, SearchTextByCategory: map[Category]string{}},
	}
	if got := FilterAndSort(tracks, Query{SearchText: "delu", Enabled: enabled, VideoDate: date, Sort: ModifiedNewest}); len(got) != 1 || got[0].ID != "eligible" {
		t.Fatalf("eligible search results = %#v", got)
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
