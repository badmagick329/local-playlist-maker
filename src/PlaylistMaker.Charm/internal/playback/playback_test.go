package playback

import (
	"context"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
