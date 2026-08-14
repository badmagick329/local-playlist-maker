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
