package playback

import (
	"context"
	"errors"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"playlistmaker/charm/internal/backend"
	"playlistmaker/charm/internal/config"
	"playlistmaker/charm/internal/library"
	"playlistmaker/charm/internal/tracksession"
)

func TestPlanPreservesQueueOrderRepeatAndMaximum(t *testing.T) {
	tracks := testTracks(t, 3, 1)
	service := Service{Tracks: tracks}
	queue := []string{tracks[2].Variants[0].ID, tracks[0].Variants[0].ID, tracks[1].Variants[0].ID}
	items, err := service.Plan(queue, backend.PlaybackOptions{RepeatEach: 1, SelectionStrategy: library.DefaultSelection})
	if err != nil {
		t.Fatal(err)
	}
	got := []string{items[0].Track.TrackID, items[1].Track.TrackID, items[2].Track.TrackID}
	want := []string{tracks[2].ID, tracks[0].ID, tracks[1].ID}
	if !slices.Equal(got, want) {
		t.Fatalf("planned track order = %v, want %v", got, want)
	}
}

func TestOneVideoPerTrackUsesStableTrackID(t *testing.T) {
	tracks := testTracks(t, 2, 2)
	queue := []string{tracks[0].Variants[0].ID, tracks[1].Variants[0].ID, tracks[0].Variants[1].ID, tracks[1].Variants[1].ID}
	items, err := (Service{Tracks: tracks}).Plan(queue, backend.PlaybackOptions{OneVideoPerTrack: true, RepeatEach: 1, SelectionStrategy: library.LatestSelection})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Track.TrackID != tracks[0].ID || items[1].Track.TrackID != tracks[1].ID {
		t.Fatalf("one per stable track = %#v", items)
	}
}

func TestShuffleUsesInjectedRandomSource(t *testing.T) {
	tracks := testTracks(t, 4, 1)
	queue := variantIDs(tracks)
	items, err := (Service{Tracks: tracks, Random: rand.New(rand.NewSource(7))}).Plan(queue, backend.PlaybackOptions{Shuffle: true, MaximumItems: 3, RepeatEach: 1})
	if err != nil || len(items) != 3 {
		t.Fatalf("shuffle = %#v, %v", items, err)
	}
	if slices.Equal([]string{items[0].Track.TrackID, items[1].Track.TrackID, items[2].Track.TrackID}, []string{tracks[0].ID, tracks[1].ID, tracks[2].ID}) {
		t.Fatal("injected shuffle kept the original order")
	}
}

func TestPlanAcceptsEveryTrackingSourceCombination(t *testing.T) {
	tracks := testTracks(t, 4, 1)
	tracks[0].SpotifyURI = "spotify:track:spotify-only"
	tracks[0].LocalAudioPath = ""
	tracks[1].SpotifyURI = ""
	tracks[2].SpotifyURI = "spotify:track:dual"
	tracks[3].SpotifyURI, tracks[3].LocalAudioPath = "", ""
	queue := variantIDs(tracks)
	if _, err := (Service{Tracks: tracks}).Plan(queue, backend.PlaybackOptions{RepeatEach: 1}); err == nil {
		t.Fatal("untracked item was accepted without the startup flag")
	}
	items, err := (Service{Tracks: tracks, AllowUntracked: true}).Plan(queue, backend.PlaybackOptions{RepeatEach: 1})
	if err != nil || len(items) != 4 || items[0].Track.LocalAudioPath != "" || items[1].Track.LocalAudioPath == "" || items[2].Track.SpotifyURI == "" {
		t.Fatalf("source combinations = %#v, %v", items, err)
	}
}

func TestLaunchStartsReadyHelperBeforeMPVAndCancelsOnMPVFailure(t *testing.T) {
	tracks := testTracks(t, 1, 1)
	order := []string{}
	helper := &fakeHelperStarter{order: &order}
	video := &fakeVideoStarter{order: &order, err: errors.New("mpv failed")}
	service := Service{
		Tracks: tracks, HelperStarter: helper, Starter: video,
		Config: config.Config{DataDirectory: t.TempDir(), PlaylistTemplate: "{playlistPath}", VideoSingleFileCommand: []string{"mpv"}, VideoPlaylistCommand: []string{"mpv", "--playlist={playlistPath}"}, VideoPlaylistSuffix: ".m3u"},
	}
	result, err := service.Launch(context.Background(), backend.PlaybackRequest{VideoIDs: variantIDs(tracks), Options: backend.PlaybackOptions{RepeatEach: 1}})
	if err != nil || result.Succeeded || !slices.Equal(order, []string{"helper", "mpv", "cancel"}) {
		t.Fatalf("launch order = %v, result %#v, err %v", order, result, err)
	}
}

type fakeVideoStarter struct {
	order *[]string
	err   error
}

func (f *fakeVideoStarter) Start(context.Context, string, []string) (int, error) {
	*f.order = append(*f.order, "mpv")
	return 42, f.err
}

type fakeHelperStarter struct{ order *[]string }
type fakeHelperHandle struct{ order *[]string }

func (f *fakeHelperStarter) Start(context.Context, string, tracksession.Manifest) (tracksession.Handle, error) {
	*f.order = append(*f.order, "helper")
	return fakeHelperHandle{order: f.order}, nil
}
func (f fakeHelperHandle) PID() int { return 7 }
func (f fakeHelperHandle) Cancel() error {
	*f.order = append(*f.order, "cancel")
	return nil
}

func testTracks(t *testing.T, trackCount, variantsPerTrack int) []library.Track {
	t.Helper()
	directory := t.TempDir()
	tracks := make([]library.Track, trackCount)
	for trackIndex := range tracks {
		trackID := "trk_test_" + string(rune('a'+trackIndex))
		audio := filepath.Join(directory, trackID+".flac")
		if err := os.WriteFile(audio, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		tracks[trackIndex] = library.Track{ID: trackID, Artist: "Artist", Title: "Title", LocalAudioPath: audio}
		for variantIndex := range variantsPerTrack {
			video := filepath.Join(directory, trackID+"-"+string(rune('a'+variantIndex))+".mkv")
			if err := os.WriteFile(video, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			tracks[trackIndex].Variants = append(tracks[trackIndex].Variants, library.Variant{ID: video, TrackID: trackID, VideoPath: video})
		}
	}
	return tracks
}

func variantIDs(tracks []library.Track) []string {
	result := []string{}
	for _, track := range tracks {
		for _, variant := range track.Variants {
			result = append(result, variant.ID)
		}
	}
	return result
}
