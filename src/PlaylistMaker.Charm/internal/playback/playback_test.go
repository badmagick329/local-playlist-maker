package playback

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"playlistmaker/charm/internal/backend"
	"playlistmaker/charm/internal/config"
	"playlistmaker/charm/internal/library"
)

type fakeStarter struct {
	calls []struct {
		program string
		args    []string
	}
	failAt int
}

func (f *fakeStarter) Start(_ context.Context, program string, args []string) (int, error) {
	f.calls = append(f.calls, struct {
		program string
		args    []string
	}{program, append([]string(nil), args...)})
	if f.failAt == len(f.calls) {
		return 0, os.ErrPermission
	}
	return 100 + len(f.calls), nil
}

func TestPlanAppliesPairingRepeatAndLimit(t *testing.T) {
	tracks := testTracks(t)
	service := Service{Tracks: tracks, Random: rand.New(rand.NewSource(1))}
	items, err := service.Plan([]string{"one", "two"}, backend.PlaybackOptions{RepeatEach: 2, MaximumItems: 3})
	if err != nil || len(items) != 3 || items[0].AudioPath != items[0].VideoPath+".flac" {
		t.Fatalf("plan = %#v, %v", items, err)
	}
	service.Tracks[0].Variants[1].AudioPath = service.Tracks[0].Variants[0].AudioPath
	items, err = service.Plan([]string{"one", "two"}, backend.PlaybackOptions{RepeatEach: 1, OneVideoPerTrack: true})
	if err != nil || len(items) != 1 {
		t.Fatalf("one-per-track = %#v, %v", items, err)
	}
	if _, err := service.Plan([]string{"missing"}, backend.DefaultPlaybackOptions()); err == nil {
		t.Fatal("missing ID was accepted")
	}
}

func TestLaunchWritesPairedPlaylistsAndStartsAudioBeforeVideo(t *testing.T) {
	directory := t.TempDir()
	tracks := testTracks(t)
	starter := &fakeStarter{}
	service := Service{Tracks: tracks, Starter: starter, Config: config.Config{DataDirectory: directory, PlaylistTemplate: "{playlistPath}", AudioPlaylistCommand: []string{"audio", "{playlistPath}"}, VideoPlaylistCommand: []string{"video", "--playlist={playlistPath}"}, AudioPlaylistSuffix: ".audio", VideoPlaylistSuffix: ".video", AudioSingleFileCommand: []string{"audio-one"}, VideoSingleFileCommand: []string{"video-one"}}}
	result, err := service.Launch(context.Background(), backend.PlaybackRequest{VideoIDs: []string{"one", "two"}, Options: backend.DefaultPlaybackOptions()})
	if err != nil || !result.Succeeded || len(starter.calls) != 2 || starter.calls[0].program != "audio" || starter.calls[1].program != "video" {
		t.Fatalf("launch = %#v, %v, %#v", result, err, starter.calls)
	}
	audioList := strings.TrimPrefix(starter.calls[0].args[0], "")
	videoList := strings.TrimPrefix(starter.calls[1].args[0], "--playlist=")
	audio, _ := os.ReadFile(audioList)
	video, _ := os.ReadFile(videoList)
	if !strings.Contains(string(audio), "one.mkv.flac") || !strings.Contains(string(video), "two.mkv") {
		t.Fatalf("paired playlists = %q / %q", audio, video)
	}
	starter = &fakeStarter{failAt: 2}
	service.Starter = starter
	result, _ = service.Launch(context.Background(), backend.PlaybackRequest{VideoIDs: []string{"one"}, Options: backend.DefaultPlaybackOptions()})
	if result.Succeeded || len(starter.calls) != 2 || !strings.Contains(result.UserSafeError, "Audio started") {
		t.Fatalf("partial launch = %#v", result)
	}
}

func TestOnePerTrackUsesNonDefaultSelectionWithinQueuedCandidates(t *testing.T) {
	tracks := testTracks(t)
	tracks[0].Variants[1].AudioPath = tracks[0].Variants[0].AudioPath
	tracks[0].Variants[0].History.SkippedCount = 5
	tracks[0].Variants[1].History.CompletedCount = 2
	tracks[0].Variants[1].History.PlayedCount = 2
	service := Service{Tracks: tracks, Random: rand.New(rand.NewSource(1))}
	items, err := service.Plan([]string{"one", "two"}, backend.PlaybackOptions{RepeatEach: 1, OneVideoPerTrack: true, SelectionStrategy: library.FavouriteSelection})
	if err != nil || len(items) != 1 || items[0].VideoPath != tracks[0].Variants[1].VideoPath {
		t.Fatalf("history selection = %#v, %v", items, err)
	}
}

func TestOnePerTrackUsesLatestSelectionWithinQueuedCandidates(t *testing.T) {
	tracks := testTracks(t)
	tracks[0].Variants[1].AudioPath = tracks[0].Variants[0].AudioPath
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tracks[0].Variants[0].Category = library.MusicVideo
	tracks[0].Variants[0].ModifiedAt = base
	tracks[0].Variants[0].Date = base.AddDate(0, 0, 2)
	tracks[0].Variants[0].History.CompletedCount = 20
	tracks[0].Variants[1].Category = library.Performance
	tracks[0].Variants[1].ModifiedAt = base.AddDate(0, 0, 1)
	tracks[0].Variants[1].Date = base
	tracks[0].Variants[1].History.SkippedCount = 20
	service := Service{Tracks: tracks, Random: rand.New(rand.NewSource(1))}
	items, err := service.Plan([]string{"one", "two"}, backend.PlaybackOptions{RepeatEach: 1, OneVideoPerTrack: true, SelectionStrategy: library.LatestSelection})
	if err != nil || len(items) != 1 || items[0].VideoPath != tracks[0].Variants[1].VideoPath {
		t.Fatalf("latest selection = %#v, %v", items, err)
	}
}

func TestPlanPreservesQueueOrderWithoutMaximumForEveryStrategy(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	queue := []string{"variant-3", "variant-5", "variant-1", "variant-4", "variant-2"}
	strategies := []library.SelectionStrategy{
		library.DefaultSelection,
		library.FavouriteSelection,
		library.FreshSelection,
		library.UnseenSelection,
		library.LatestSelection,
	}
	for _, strategy := range strategies {
		t.Run(strategy.String(), func(t *testing.T) {
			tracks := testVariantTracks(t, 5)
			for index := range tracks[0].Variants {
				attempted := base.Add(time.Duration(index) * time.Hour)
				tracks[0].Variants[index].Date = attempted
				tracks[0].Variants[index].ModifiedAt = attempted
				tracks[0].Variants[index].History = library.History{
					PlayedCount:        index + 1,
					CompletedCount:     index + 1,
					LastAttemptedAtUTC: &attempted,
				}
			}
			items, err := (Service{Tracks: tracks}).Plan(queue, backend.PlaybackOptions{RepeatEach: 1, SelectionStrategy: strategy})
			if err != nil {
				t.Fatal(err)
			}
			if got := itemIDs(items); !sameStrings(got, queue) {
				t.Fatalf("%s queue order = %v, want %v", strategy, got, queue)
			}
		})
	}
}

func TestPlanLatestSelectsNewestAndPreservesQueueOrder(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tracks := testVariantTracks(t, 5)
	for index := range tracks[0].Variants {
		tracks[0].Variants[index].ModifiedAt = base.Add(time.Duration(index) * time.Hour)
	}
	queue := []string{"variant-4", "variant-1", "variant-5", "variant-2", "variant-3"}
	items, err := (Service{Tracks: tracks}).Plan(queue, backend.PlaybackOptions{MaximumItems: 3, RepeatEach: 1, SelectionStrategy: library.LatestSelection})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := itemIDs(items), []string{"variant-4", "variant-5", "variant-3"}; !sameStrings(got, want) {
		t.Fatalf("latest selection = %v, want %v", got, want)
	}
}

func TestPlanFreshSelectsFreshestAndPreservesQueueOrder(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tracks := testVariantTracks(t, 5)
	for index := range tracks[0].Variants {
		attempted := base.Add(time.Duration(index) * time.Hour)
		tracks[0].Variants[index].History.LastAttemptedAtUTC = &attempted
	}
	queue := []string{"variant-3", "variant-5", "variant-1", "variant-4", "variant-2"}
	items, err := (Service{Tracks: tracks}).Plan(queue, backend.PlaybackOptions{MaximumItems: 3, RepeatEach: 1, SelectionStrategy: library.FreshSelection})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := itemIDs(items), []string{"variant-3", "variant-1", "variant-2"}; !sameStrings(got, want) {
		t.Fatalf("fresh selection = %v, want %v", got, want)
	}
}

func TestPlanOtherStrategiesSelectSubsetAndPreserveQueueOrder(t *testing.T) {
	tests := []struct {
		name     string
		strategy library.SelectionStrategy
		prepare  func([]library.Variant)
		queue    []string
		want     []string
	}{
		{
			name:     "favourite",
			strategy: library.FavouriteSelection,
			prepare: func(values []library.Variant) {
				values[0].History = library.History{PlayedCount: 1, CompletedCount: 1}
				values[1].History = library.History{PlayedCount: 4, CompletedCount: 4}
				values[2].History = library.History{PlayedCount: 1, SkippedCount: 1}
				values[3].History = library.History{}
				values[4].History = library.History{SkippedCount: 3}
			},
			queue: []string{"variant-3", "variant-5", "variant-1", "variant-4", "variant-2"},
			want:  []string{"variant-3", "variant-1", "variant-2"},
		},
		{
			name:     "unseen",
			strategy: library.UnseenSelection,
			prepare: func(values []library.Variant) {
				values[0].History = library.History{}
				values[1].History = library.History{SkippedCount: 1}
				values[2].History = library.History{StoppedCount: 1}
				values[3].History = library.History{AbandonedCount: 1}
				values[4].History = library.History{PlayedCount: 1}
			},
			queue: []string{"variant-3", "variant-5", "variant-1", "variant-2", "variant-4"},
			want:  []string{"variant-3", "variant-1", "variant-4"},
		},
		{
			name:     "default",
			strategy: library.DefaultSelection,
			prepare: func(values []library.Variant) {
				for index := range values {
					values[index].Date = time.Date(2026, 1, 1+index, 0, 0, 0, 0, time.UTC)
				}
			},
			queue: []string{"variant-4", "variant-1", "variant-5", "variant-2", "variant-3"},
			want:  []string{"variant-4", "variant-5", "variant-3"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tracks := testVariantTracks(t, 5)
			test.prepare(tracks[0].Variants)
			items, err := (Service{Tracks: tracks}).Plan(test.queue, backend.PlaybackOptions{MaximumItems: 3, RepeatEach: 1, SelectionStrategy: test.strategy})
			if err != nil {
				t.Fatal(err)
			}
			if got := itemIDs(items); !sameStrings(got, test.want) {
				t.Fatalf("%s ranking = %v, want %v", test.name, got, test.want)
			}
		})
	}
}

func TestPlanStableTiesPreserveFirstQueuedOccurrence(t *testing.T) {
	tracks := testVariantTracks(t, 2)
	queue := []string{"variant-2", "variant-1", "variant-2"}
	items, err := (Service{Tracks: tracks}).Plan(queue, backend.PlaybackOptions{MaximumItems: 2, RepeatEach: 1, SelectionStrategy: library.DefaultSelection})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := itemIDs(items), []string{"variant-2", "variant-1"}; !sameStrings(got, want) {
		t.Fatalf("stable occurrence selection = %v, want %v", got, want)
	}
}

func TestPlanShuffleCapsAfterShufflingInsteadOfRanking(t *testing.T) {
	tracks := testVariantTracks(t, 5)
	for index := range tracks[0].Variants {
		tracks[0].Variants[index].ModifiedAt = time.Date(2026, 1, 1+index, 0, 0, 0, 0, time.UTC)
	}
	service := Service{Tracks: tracks, Random: rand.New(rand.NewSource(7))}
	items, err := service.Plan(variantIDs(tracks[0]), backend.PlaybackOptions{Shuffle: true, MaximumItems: 3, RepeatEach: 1, SelectionStrategy: library.LatestSelection})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := itemIDs(items), []string{"variant-3", "variant-2", "variant-4"}; !sameStrings(got, want) {
		t.Fatalf("shuffled ranking = %v, want %v", got, want)
	}
}

func TestOnePerTrackUsesStrategyAndPreservesFirstTrackOrder(t *testing.T) {
	tracks := testVariantTracks(t, 5)
	groupA, groupB, groupC := "group-a.flac", "group-b.flac", "group-c.flac"
	tracks[0].Variants[0].AudioPath = groupA
	tracks[0].Variants[1].AudioPath = groupB
	tracks[0].Variants[2].AudioPath = groupA
	tracks[0].Variants[3].AudioPath = groupC
	tracks[0].Variants[4].AudioPath = groupB
	for index := range tracks[0].Variants {
		tracks[0].Variants[index].ModifiedAt = time.Date(2026, 1, 1+index, 0, 0, 0, 0, time.UTC)
	}
	tracks[0].Variants[2].ModifiedAt = time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC)
	tracks[0].Variants[3].ModifiedAt = time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	queue := []string{"variant-1", "variant-2", "variant-4", "variant-3", "variant-5"}
	service := Service{Tracks: tracks}
	items, err := service.Plan(queue, backend.PlaybackOptions{OneVideoPerTrack: true, RepeatEach: 1, SelectionStrategy: library.LatestSelection})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := itemIDs(items), []string{"variant-3", "variant-5", "variant-4"}; !sameStrings(got, want) {
		t.Fatalf("representative order = %v, want %v", got, want)
	}
	items, err = service.Plan(queue, backend.PlaybackOptions{OneVideoPerTrack: true, MaximumItems: 2, RepeatEach: 1, SelectionStrategy: library.LatestSelection})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := itemIDs(items), []string{"variant-3", "variant-5"}; !sameStrings(got, want) {
		t.Fatalf("capped representative order = %v, want %v", got, want)
	}
}

func TestOnePerTrackShuffleUsesInjectedRandomRepresentative(t *testing.T) {
	tracks := testVariantTracks(t, 5)
	for index := range tracks[0].Variants {
		tracks[0].Variants[index].AudioPath = tracks[0].Variants[0].AudioPath
		tracks[0].Variants[index].ModifiedAt = time.Date(2026, 1, 1+index, 0, 0, 0, 0, time.UTC)
	}
	service := Service{Tracks: tracks, Random: rand.New(rand.NewSource(7))}
	items, err := service.Plan(variantIDs(tracks[0]), backend.PlaybackOptions{OneVideoPerTrack: true, Shuffle: true, MaximumItems: 1, RepeatEach: 1, SelectionStrategy: library.LatestSelection})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := itemIDs(items), []string{"variant-2"}; !sameStrings(got, want) {
		t.Fatalf("random representative = %v, want %v", got, want)
	}
}

func TestPlanRepeatEachAndPartialMaximumPreserveExpandedOrder(t *testing.T) {
	tracks := testVariantTracks(t, 3)
	for index := range tracks[0].Variants {
		tracks[0].Variants[index].ModifiedAt = time.Date(2026, 1, 1+index, 0, 0, 0, 0, time.UTC)
	}
	service := Service{Tracks: tracks}
	items, err := service.Plan([]string{"variant-2", "variant-3", "variant-1"}, backend.PlaybackOptions{MaximumItems: 3, RepeatEach: 2, SelectionStrategy: library.LatestSelection})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := itemIDs(items), []string{"variant-2", "variant-3", "variant-3"}; !sameStrings(got, want) {
		t.Fatalf("repeat and cap = %v, want %v", got, want)
	}
}

func testVariantTracks(t *testing.T, count int) []library.Track {
	t.Helper()
	directory := t.TempDir()
	values := make([]library.Variant, count)
	for index := range values {
		id := fmt.Sprintf("variant-%d", index+1)
		video := filepath.Join(directory, id+".mkv")
		audio := video + ".flac"
		if err := os.WriteFile(video, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(audio, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		values[index] = library.Variant{ID: id, VideoPath: video, AudioPath: audio}
	}
	return []library.Track{{ID: "track", Variants: values}}
}

func variantIDs(track library.Track) []string {
	ids := make([]string, len(track.Variants))
	for index, variant := range track.Variants {
		ids[index] = variant.ID
	}
	return ids
}

func itemIDs(items []Item) []string {
	ids := make([]string, len(items))
	for index, item := range items {
		ids[index] = strings.TrimSuffix(filepath.Base(item.VideoPath), filepath.Ext(item.VideoPath))
	}
	return ids
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func testTracks(t *testing.T) []library.Track {
	t.Helper()
	directory := t.TempDir()
	values := []library.Variant{}
	for _, id := range []string{"one", "two"} {
		video := filepath.Join(directory, id+".mkv")
		audio := video + ".flac"
		if err := os.WriteFile(video, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(audio, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		values = append(values, library.Variant{ID: id, VideoPath: video, AudioPath: audio})
	}
	return []library.Track{{ID: "track", Variants: values}}
}
