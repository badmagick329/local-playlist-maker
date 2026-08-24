// Package playback plans mpv video playback and starts its tracking helper.
package playback

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"playlistmaker/charm/internal/backend"
	"playlistmaker/charm/internal/config"
	"playlistmaker/charm/internal/history"
	"playlistmaker/charm/internal/library"
	"playlistmaker/charm/internal/pathid"
	"playlistmaker/charm/internal/tracking"
	"playlistmaker/charm/internal/tracksession"
)

type Item struct {
	VideoPath string
	Track     tracking.Track
}

type Starter interface {
	Start(context.Context, string, []string) (int, error)
}

type OSStarter struct{}

func (OSStarter) Start(ctx context.Context, program string, arguments []string) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	command := exec.Command(program, arguments...)
	command.Stdin, command.Stdout, command.Stderr = nil, io.Discard, io.Discard
	if err := command.Start(); err != nil {
		return 0, err
	}
	pid := command.Process.Pid
	go func() { _ = command.Wait() }()
	return pid, nil
}

type Service struct {
	Tracks         []library.Track
	Config         config.Config
	Starter        Starter
	HelperStarter  tracksession.Starter
	Random         *rand.Rand
	History        *history.Service
	HistoryEnabled bool
	AllowUntracked bool
}

func (s Service) Launch(ctx context.Context, request backend.PlaybackRequest) (backend.PlaybackResult, error) {
	items, err := s.Plan(request.VideoIDs, request.Options)
	if err != nil {
		return backend.PlaybackResult{UserSafeError: err.Error()}, nil
	}
	for _, item := range items {
		if _, err := os.Stat(item.VideoPath); err != nil {
			return backend.PlaybackResult{UserSafeError: "A queued video is unavailable."}, nil
		}
	}
	entries := make([]tracksession.Entry, len(items))
	for index, item := range items {
		entries[index] = tracksession.Entry{VideoPath: item.VideoPath, Track: item.Track}
	}
	historyPath, minimum := "", 50
	if s.History != nil {
		historyPath = s.History.HistoryPath()
		minimum = s.History.MinimumWatchedPercent
	}
	manifestPath, manifest, err := tracksession.Create(s.Config.DataDirectory, entries, s.AllowUntracked, s.HistoryEnabled, historyPath, minimum)
	if err != nil {
		return backend.PlaybackResult{UserSafeError: "Could not create playback session."}, nil
	}
	manifest.ConfigPath = s.Config.ConfigPath
	if err := tracksession.WriteManifest(manifestPath, manifest); err != nil {
		tracksession.Cleanup(manifestPath, manifest)
		return backend.PlaybackResult{UserSafeError: "Could not prepare playback session."}, nil
	}
	helperStarter := s.HelperStarter
	if helperStarter == nil {
		helperStarter = tracksession.OSStarter{}
	}
	helper, err := helperStarter.Start(ctx, manifestPath, manifest)
	if err != nil {
		tracksession.Cleanup(manifestPath, manifest)
		return backend.PlaybackResult{UserSafeError: err.Error()}, nil
	}
	program, arguments, err := s.videoCommand(items, manifestPath, manifest)
	if err != nil {
		_ = helper.Cancel()
		tracksession.Cleanup(manifestPath, manifest)
		return backend.PlaybackResult{UserSafeError: err.Error()}, nil
	}
	starter := s.Starter
	if starter == nil {
		starter = OSStarter{}
	}
	mpvPID, err := starter.Start(ctx, program, arguments)
	if err != nil {
		_ = helper.Cancel()
		tracksession.Cleanup(manifestPath, manifest)
		return backend.PlaybackResult{UserSafeError: "Could not start video player."}, nil
	}
	manifest.MPVProcessID = mpvPID
	if err := tracksession.WriteManifest(manifestPath, manifest); err != nil {
		_ = helper.Cancel()
		return backend.PlaybackResult{Succeeded: true, PlannedVideoCount: len(items), UserSafeError: "Video started, but its tracking session was not updated."}, nil
	}
	return backend.PlaybackResult{Succeeded: true, PlannedVideoCount: len(items)}, nil
}

func (s Service) Plan(ids []string, options backend.PlaybackOptions) ([]Item, error) {
	if options.RepeatEach < 1 || options.RepeatEach > 10 || options.MaximumItems < 0 {
		return nil, fmt.Errorf("invalid playback options")
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("The queue is empty.")
	}
	type indexed struct {
		track   library.Track
		variant library.Variant
	}
	index := map[string]indexed{}
	for _, track := range s.Tracks {
		for _, variant := range track.Variants {
			index[pathid.ComparisonKey(variant.ID)] = indexed{track: track, variant: variant}
		}
	}
	queued := make([]indexed, 0, len(ids))
	for _, id := range ids {
		value, ok := index[pathid.ComparisonKey(id)]
		if !ok {
			return nil, fmt.Errorf("Queued video is no longer in the library")
		}
		if value.variant.VideoPath == "" || value.variant.TrackID == "" || value.variant.TrackID != value.track.ID {
			return nil, fmt.Errorf("Queued video has no catalogue track")
		}
		queued = append(queued, value)
	}
	if options.OneVideoPerTrack {
		groups := map[string][]indexed{}
		order := []string{}
		for _, value := range queued {
			if _, ok := groups[value.track.ID]; !ok {
				order = append(order, value.track.ID)
			}
			groups[value.track.ID] = append(groups[value.track.ID], value)
		}
		queued = queued[:0]
		for _, trackID := range order {
			values := groups[trackID]
			if options.Shuffle {
				queued = append(queued, values[s.random().Intn(len(values))])
				continue
			}
			variants := make([]library.Variant, len(values))
			for index := range values {
				variants[index] = values[index].variant
			}
			if selected, ok := library.SelectVariant(variants, options.SelectionStrategy); ok {
				for _, value := range values {
					if value.variant.ID == selected.ID {
						queued = append(queued, value)
						break
					}
				}
			}
		}
	}
	expanded := make([]indexed, 0, len(queued)*options.RepeatEach)
	for _, value := range queued {
		for range options.RepeatEach {
			expanded = append(expanded, value)
		}
	}
	if options.Shuffle {
		s.random().Shuffle(len(expanded), func(i, j int) { expanded[i], expanded[j] = expanded[j], expanded[i] })
	}
	if options.MaximumItems > 0 && len(expanded) > options.MaximumItems {
		if options.Shuffle {
			expanded = expanded[:options.MaximumItems]
		} else {
			variants := make([]library.Variant, len(expanded))
			for index := range expanded {
				variants[index] = expanded[index].variant
			}
			keep := make([]bool, len(expanded))
			for _, index := range library.RankVariantIndexes(variants, options.SelectionStrategy)[:options.MaximumItems] {
				keep[index] = true
			}
			retained := expanded[:0]
			for index, value := range expanded {
				if keep[index] {
					retained = append(retained, value)
				}
			}
			expanded = retained
		}
	}
	planned := make([]Item, len(expanded))
	for index, value := range expanded {
		localPath := value.track.LocalAudioPath
		if localPath != "" {
			if _, err := os.Stat(localPath); err != nil {
				localPath = ""
			}
		}
		if value.track.SpotifyURI == "" && localPath == "" && !s.AllowUntracked {
			return nil, fmt.Errorf("%s - %s has no available tracking source; restart with --allow-untracked-playback to play it", value.track.Artist, value.track.Title)
		}
		planned[index] = Item{VideoPath: value.variant.VideoPath, Track: tracking.Track{TrackID: value.track.ID, Artist: value.track.Artist, Title: value.track.Title, LocalAudioPath: localPath, SpotifyURI: value.track.SpotifyURI}}
	}
	return planned, nil
}

func (s Service) random() *rand.Rand {
	if s.Random != nil {
		return s.Random
	}
	return rand.New(rand.NewSource(time.Now().UnixNano()))
}

func (s Service) videoCommand(items []Item, manifestPath string, manifest tracksession.Manifest) (string, []string, error) {
	if len(items) == 0 {
		return "", nil, fmt.Errorf("The queue is empty.")
	}
	additional := []string{
		"--script-opt=playlistmaker_history-manifest_path=" + manifestPath,
		"--script-opt=playlistmaker_history-event_path=" + manifest.EventPath,
		fmt.Sprintf("--script-opt=playlistmaker_history-minimum_watched_percent=%d", manifest.MinimumWatchedPercent),
	}
	if manifest.HistoryEnabled {
		additional = append(additional, "--script-opt=playlistmaker_history-history_path="+manifest.HistoryPath)
	}
	if len(items) == 1 && len(s.Config.VideoSingleFileCommand) > 0 {
		arguments := append([]string{}, s.Config.VideoSingleFileCommand[1:]...)
		arguments = append(arguments, additional...)
		arguments = append(arguments, items[0].VideoPath)
		return s.Config.VideoSingleFileCommand[0], arguments, nil
	}
	if len(s.Config.VideoPlaylistCommand) == 0 {
		return "", nil, fmt.Errorf("video player command is not configured")
	}
	paths := make([]string, len(items))
	for index, item := range items {
		paths[index] = item.VideoPath
	}
	playlist, err := writePlaylist(s.Config.DataDirectory, s.Config.VideoPlaylistSuffix, paths)
	if err != nil {
		return "", nil, err
	}
	arguments := make([]string, 0, len(s.Config.VideoPlaylistCommand)+len(additional))
	for _, argument := range s.Config.VideoPlaylistCommand[1:] {
		arguments = append(arguments, strings.ReplaceAll(argument, s.Config.PlaylistTemplate, playlist))
	}
	arguments = append(arguments, additional...)
	return s.Config.VideoPlaylistCommand[0], arguments, nil
}

func writePlaylist(directory, suffix string, paths []string) (string, error) {
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return "", err
	}
	file, err := os.CreateTemp(directory, "playlist-*")
	if err != nil {
		return "", err
	}
	name := file.Name()
	target := name + suffix
	if _, err := file.WriteString(strings.Join(paths, "\n") + "\n"); err != nil {
		_ = file.Close()
		_ = os.Remove(name)
		return "", err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	if err := os.Rename(name, target); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	return filepath.Clean(target), nil
}
