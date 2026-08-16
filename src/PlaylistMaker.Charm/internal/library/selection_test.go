package library

import (
	"testing"
	"time"
)

func TestSelectionStrategiesUseHistoryDeterministically(t *testing.T) {
	now := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	old := now.AddDate(0, 0, -2)
	variants := []Variant{
		{ID: "skipped", History: History{SkippedCount: 5, LastAttemptedAtUTC: &now}},
		{ID: "completed", History: History{PlayedCount: 2, CompletedCount: 2, LastPlayedAtUTC: &now, LastAttemptedAtUTC: &now}},
		{ID: "fresh", History: History{}},
		{ID: "old", History: History{StoppedCount: 1, LastAttemptedAtUTC: &old}},
	}
	if got, _ := SelectVariant(variants, FavouriteSelection); got.ID != "completed" {
		t.Fatalf("favourite = %s", got.ID)
	}
	if got, _ := SelectVariant(variants, FreshSelection); got.ID != "fresh" {
		t.Fatalf("fresh = %s", got.ID)
	}
	if got, _ := SelectVariant(variants, UnseenSelection); got.ID != "fresh" {
		t.Fatalf("unseen = %s", got.ID)
	}
	played := []Variant{{ID: "later", History: History{PlayedCount: 1, LastAttemptedAtUTC: &now}}, {ID: "earlier", History: History{PlayedCount: 1, LastAttemptedAtUTC: &old}}}
	if got, _ := SelectVariant(played, UnseenSelection); got.ID != "earlier" {
		t.Fatalf("unseen fallback = %s", got.ID)
	}
}

func TestSelectionNeverEscapesCandidateSetAndUsesStableTieBreak(t *testing.T) {
	variants := []Variant{{ID: "z", Category: Performance}, {ID: "a", Category: MusicVideo}}
	if got, _ := SelectVariant(variants, DefaultSelection); got.ID != "a" {
		t.Fatalf("default = %s", got.ID)
	}
	if got, _ := SelectVariant(variants[:1], FavouriteSelection); got.ID != "z" {
		t.Fatalf("candidate set escaped: %s", got.ID)
	}
}

func TestDefaultSelectionPrefersOriginalLanguageBeforeRecency(t *testing.T) {
	old := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := old.AddDate(1, 0, 0)
	for _, title := range []string{"Lovey Dovey", "Roly-Poly"} {
		variants := []Variant{
			{ID: "original", Filename: "Artist - " + title + ".mkv", Category: MusicVideo, Date: old, ModifiedAt: old},
			{ID: "japanese", Filename: "Artist - " + title + " Japanese version.mkv", Category: MusicVideo, Date: newer, ModifiedAt: newer},
		}
		if got, _ := SelectVariant(variants, DefaultSelection); got.ID != "original" {
			t.Fatalf("%s default = %s", title, got.ID)
		}
	}
}

func TestDefaultSelectionLanguageCategoryAndDateRanking(t *testing.T) {
	old := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := old.AddDate(1, 0, 0)
	if got, _ := SelectVariant([]Variant{{ID: "korean", Filename: "Artist - Song Korean.mkv", Category: MusicVideo, Date: old}, {ID: "japanese", Filename: "Artist - Song (Japanese ver.).mkv", Category: MusicVideo, Date: newer}}, DefaultSelection); got.ID != "korean" {
		t.Fatalf("Korean should beat Japanese, got %s", got.ID)
	}
	if got, _ := SelectVariant([]Variant{{ID: "only", Filename: "Artist - Song Japanese.mkv", Category: MusicVideo, Date: newer}}, DefaultSelection); got.ID != "only" {
		t.Fatalf("only Japanese = %s", got.ID)
	}
	if got, _ := SelectVariant([]Variant{{ID: "old", Filename: "Artist - Song.mkv", Category: MusicVideo, Date: old}, {ID: "new", Filename: "Artist - Song Korean version.mkv", Category: MusicVideo, Date: newer}}, DefaultSelection); got.ID != "new" {
		t.Fatalf("newest preferred = %s", got.ID)
	}
	if got, _ := SelectVariant([]Variant{{ID: "mv", Filename: "Artist - Song Japanese.mkv", Category: MusicVideo, Date: old}, {ID: "performance", Filename: "Artist - Song.mkv", Category: Performance, Date: newer}}, DefaultSelection); got.ID != "mv" {
		t.Fatalf("music video priority = %s", got.ID)
	}
}

func TestQueryAwareDefaultSelectionUsesTheSameRanking(t *testing.T) {
	old := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	track := Track{Variants: []Variant{{ID: "original", Filename: "Artist - Song.mkv", Category: MusicVideo, Date: old}, {ID: "japanese", Filename: "Artist - Song Japanese.mkv", Category: MusicVideo, Date: old.AddDate(1, 0, 0)}}}
	query := Query{Enabled: map[Category]bool{MusicVideo: true}}
	if got, _ := DefaultVariant(track, query); got.ID != "original" {
		t.Fatalf("query default = %s", got.ID)
	}
}
