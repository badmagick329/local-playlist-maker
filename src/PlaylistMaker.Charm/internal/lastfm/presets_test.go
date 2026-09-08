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

func TestForgottenFavouritesCooldownAndEstablishedListening(t *testing.T) {
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	old := now.AddDate(-1, 0, 0)
	plays := []time.Time{}
	for i := 0; i < 10; i++ {
		plays = append(plays, old.AddDate(0, 0, i/2))
	}
	for _, tc := range []struct {
		name   string
		change func(*library.Track, *[]time.Time)
		want   bool
	}{
		{"established", func(*library.Track, *[]time.Time) {}, true},
		{"too few plays", func(_ *library.Track, p *[]time.Time) { *p = (*p)[:9] }, false},
		{"single binge", func(_ *library.Track, p *[]time.Time) {
			for i := range *p {
				(*p)[i] = old
			}
		}, false},
		{"recent scrobble", func(_ *library.Track, p *[]time.Time) { *p = append(*p, now.AddDate(0, -6, 0)) }, false},
		{"recent skip", func(t *library.Track, _ *[]time.Time) {
			at := now.AddDate(0, 0, -29)
			t.History.LastAttemptedAtUTC = &at
		}, false},
		{"cooldown boundary", func(t *library.Track, _ *[]time.Time) {
			at := now.AddDate(0, 0, -30)
			t.History.LastAttemptedAtUTC = &at
		}, false},
		{"cooldown expired", func(t *library.Track, _ *[]time.Time) {
			at := now.AddDate(0, 0, -30).Add(-time.Second)
			t.History.LastAttemptedAtUTC = &at
		}, true},
		{"other video skipped", func(t *library.Track, _ *[]time.Time) {
			at := now
			t.Variants = append(t.Variants, library.Variant{Category: library.Category("filtered"), History: library.History{LastAttemptedAtUTC: &at}})
		}, false},
		{"local listening absent from cache", func(t *library.Track, _ *[]time.Time) { at := now.AddDate(0, -2, 0); t.History.LastPlayedAtUTC = &at }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			track := presetTrack("song")
			history := append([]time.Time(nil), plays...)
			tc.change(&track, &history)
			s := Service{Random: firstPick{}, index: Index{TrackPlays: map[string][]time.Time{"song": history}}}
			r, err := s.BuildMix(MixRequest{Preset: ForgottenFavourites, Now: now, Count: 10, Tracks: []library.Track{track}, Query: library.Query{Enabled: map[library.Category]bool{library.BandLive: true}}})
			if err != nil || (r.Created == 1) != tc.want {
				t.Fatalf("created=%d err=%v", r.Created, err)
			}
		})
	}
	if weight := forgottenWeight(presetTrack("song"), plays, now); weight != 4 {
		t.Fatalf("moderate familiarity weight=%d", weight)
	}
}

func TestCurrentObsessionsRanksGrowthAgainstCacheDate(t *testing.T) {
	anchor := time.Date(2025, 1, 31, 12, 0, 0, 0, time.UTC)
	s := Service{Random: firstPick{}, index: Index{Scrobbles: []Scrobble{{PlayedAtUTC: anchor}}, TrackPlays: map[string][]time.Time{}}}
	tracks := []library.Track{}
	for _, id := range []string{"growing", "new", "declining", "binge"} {
		tracks = append(tracks, presetTrack(id))
	}
	for _, id := range []string{"growing", "new", "declining"} {
		s.index.TrackPlays[id] = []time.Time{anchor.AddDate(0, 0, -1), anchor}
	}
	for i := 0; i < 6; i++ {
		s.index.TrackPlays["growing"] = append(s.index.TrackPlays["growing"], anchor.AddDate(0, 0, -2))
	}
	for i := 0; i < 3; i++ {
		s.index.TrackPlays["growing"] = append(s.index.TrackPlays["growing"], anchor.AddDate(0, 0, -20))
	}
	for i := 0; i < 10; i++ {
		s.index.TrackPlays["declining"] = append(s.index.TrackPlays["declining"], anchor.AddDate(0, 0, -20))
		s.index.TrackPlays["binge"] = append(s.index.TrackPlays["binge"], anchor)
	}
	request := MixRequest{Preset: CurrentObsessions, Now: anchor.AddDate(1, 0, 0), Count: 10, Tracks: tracks, Query: library.Query{Enabled: map[library.Category]bool{library.BandLive: true}}}
	r, err := s.BuildMix(request)
	if err != nil || r.Created != 2 || r.Variants[0].TrackID != "growing" || r.Variants[1].TrackID != "new" {
		t.Fatalf("growth mix=%+v %v", r, err)
	}
	request.Action = AppendQueue
	request.QueuedTrackIDs = map[string]bool{"growing": true}
	r, err = s.BuildMix(request)
	if err != nil || r.Created != 1 || r.Variants[0].TrackID != "new" {
		t.Fatalf("append=%+v %v", r, err)
	}
}

func TestObsessionWindowBoundaries(t *testing.T) {
	end := time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)
	start := end.AddDate(0, 0, -14)
	previous := start.AddDate(0, 0, -30)
	plays := []time.Time{start, end.Add(-time.Second), previous, start.Add(-time.Second), previous.Add(-time.Second), end}
	if got := obsessionGrowth(plays, end); got != 32 {
		t.Fatalf("window score=%d, want 2*30-2*14", got)
	}
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
