package lastfm

import (
	"fmt"
	"testing"
	"time"

	"playlistmaker/charm/internal/library"
)

type firstPick struct{}

func (firstPick) Intn(int) int { return 0 }

func presetTrack(id string) library.Track {
	return library.Track{ID: id, Artist: id, Variants: []library.Variant{{ID: id + "-video", TrackID: id, Category: library.BandLive}}}
}

func TestFamiliarUnseenRequiresHistoryAndStrictlyUnseenEligibleVideo(t *testing.T) {
	now := time.Now()
	tracks := []library.Track{presetTrack("familiar"), presetTrack("watched"), presetTrack("unfamiliar"), presetTrack("filtered")}
	tracks[0].Variants = append(tracks[0].Variants, library.Variant{ID: "watched-favourite", TrackID: "familiar", Category: library.BandLive, History: library.History{PlayedCount: 99}})
	tracks[1].Variants[0].History.PlayedCount = 1
	tracks[3].Variants[0].Category = library.Category("excluded")
	service := Service{Random: firstPick{}, index: Index{TrackPlays: map[string][]time.Time{}}}
	for _, tr := range tracks[:2] {
		service.index.TrackPlays[tr.ID] = []time.Time{now, now, now}
	}
	service.index.TrackPlays["filtered"] = []time.Time{now, now, now}
	request := MixRequest{Preset: FamiliarUnseen, Count: 20, Tracks: tracks, Query: library.Query{Enabled: map[library.Category]bool{library.BandLive: true}}, SelectionStrategy: library.FavouriteSelection}
	result, err := service.BuildMix(request)
	if err != nil || result.Created != 1 || result.Variants[0].ID != "familiar-video" {
		t.Fatalf("strict unseen: %+v %v", result, err)
	}
	request.Action = AppendQueue
	request.QueuedTrackIDs = map[string]bool{"familiar": true}
	result, err = service.BuildMix(request)
	if err != nil || result.Created != 0 {
		t.Fatalf("append repeated existing track: %+v %v", result, err)
	}
}

func TestBalancedRotationAllocatesBucketsWithoutDuplicates(t *testing.T) {
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	service := Service{Random: firstPick{}, index: Index{TrackPlays: map[string][]time.Time{}}}
	tracks := []library.Track{}
	for bucket := 0; bucket < 3; bucket++ {
		for i := 0; i < 10; i++ {
			id := fmt.Sprintf("%d-%d", bucket, i)
			tracks = append(tracks, presetTrack(id))
			if bucket == 0 {
				service.index.TrackPlays[id] = []time.Time{now, now}
			}
			if bucket == 1 {
				old := now.AddDate(-1, 0, 0)
				service.index.TrackPlays[id] = []time.Time{old, old, old}
			}
		}
	}
	request := MixRequest{Preset: BalancedRotation, Count: 10, Tracks: tracks, Now: now, Query: library.Query{Enabled: map[library.Category]bool{library.BandLive: true}}}
	result, err := service.BuildMix(request)
	if err != nil || result.Created != 10 {
		t.Fatalf("result: %+v %v", result, err)
	}
	counts := [3]int{}
	seen := map[string]bool{}
	for _, v := range result.Variants {
		if seen[v.TrackID] {
			t.Fatal("duplicate")
		}
		seen[v.TrackID] = true
		counts[int(v.TrackID[0]-'0')]++
	}
	if counts != [3]int{4, 4, 2} {
		t.Fatalf("proportions: %v", counts)
	}
	request.Count = 40
	result, err = service.BuildMix(request)
	if err != nil || result.Created != 30 {
		t.Fatalf("bucket exhaustion: %+v %v", result, err)
	}
}
