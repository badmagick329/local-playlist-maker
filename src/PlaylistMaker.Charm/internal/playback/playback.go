// Package playback plans and starts paired native audio/video playback.
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
)

type Item struct{ VideoPath, AudioPath, Artist, Title string }
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
	Random         *rand.Rand
	History        *history.Service
	HistoryEnabled bool
	Now            func() time.Time
}

func (s Service) Launch(ctx context.Context, request backend.PlaybackRequest) (backend.PlaybackResult, error) {
	items, err := s.Plan(request.VideoIDs, request.Options)
	if err != nil {
		return backend.PlaybackResult{UserSafeError: err.Error()}, nil
	}
	if len(items) == 0 {
		return backend.PlaybackResult{UserSafeError: "The queue is empty."}, nil
	}
	for _, item := range items {
		if _, err := os.Stat(item.VideoPath); err != nil {
			return backend.PlaybackResult{UserSafeError: "A queued video is unavailable."}, nil
		}
		if _, err := os.Stat(item.AudioPath); err != nil {
			return backend.PlaybackResult{UserSafeError: "A queued audio file is unavailable."}, nil
		}
	}
	var session history.Session
	if s.HistoryEnabled && s.History != nil {
		entries := make([]history.SessionEntry, len(items))
		for index, item := range items {
			entries[index] = history.SessionEntry{VideoPath: item.VideoPath, AudioPath: item.AudioPath, Artist: item.Artist, Title: item.Title}
		}
		session, err = s.History.Create(entries)
		if err != nil {
			return backend.PlaybackResult{UserSafeError: "Could not create playback history session."}, nil
		}
	}
	audioProgram, audioArguments, err := s.commandFor(items, false, nil)
	if err != nil {
		return backend.PlaybackResult{UserSafeError: err.Error()}, nil
	}
	starter := s.Starter
	if starter == nil {
		starter = OSStarter{}
	}
	if _, err := starter.Start(ctx, audioProgram, audioArguments); err != nil {
		return backend.PlaybackResult{UserSafeError: "Could not start audio player."}, nil
	}
	var historyArguments []string
	if s.HistoryEnabled && s.History != nil {
		historyArguments = s.History.MpvArguments(session)
	}
	videoProgram, videoArguments, err := s.commandFor(items, true, historyArguments)
	if err != nil {
		return backend.PlaybackResult{UserSafeError: err.Error()}, nil
	}
	pid, err := starter.Start(ctx, videoProgram, videoArguments)
	if err != nil {
		return backend.PlaybackResult{UserSafeError: "Audio started, but video player failed to start."}, nil
	}
	if s.HistoryEnabled && s.History != nil {
		if err := s.History.RecordPID(&session, pid); err != nil {
			return backend.PlaybackResult{Succeeded: true, PlannedVideoCount: len(items), UserSafeError: "Players started, but history session PID was not recorded."}, nil
		}
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
	index := map[string]library.Variant{}
	for _, track := range s.Tracks {
		for _, variant := range track.Variants {
			index[pathid.ComparisonKey(variant.ID)] = variant
		}
	}
	queued := make([]library.Variant, 0, len(ids))
	for _, id := range ids {
		variant, ok := index[pathid.ComparisonKey(id)]
		if !ok {
			return nil, fmt.Errorf("Queued video is no longer in the library")
		}
		if variant.VideoPath == "" || variant.AudioPath == "" {
			return nil, fmt.Errorf("Queued video has no paired audio path")
		}
		queued = append(queued, variant)
	}
	if options.OneVideoPerTrack {
		groups := map[string][]library.Variant{}
		order := []string{}
		for _, variant := range queued {
			key := pathid.ComparisonKey(variant.AudioPath)
			if _, ok := groups[key]; !ok {
				order = append(order, key)
			}
			groups[key] = append(groups[key], variant)
		}
		queued = queued[:0]
		for _, key := range order {
			values := groups[key]
			if options.SelectionStrategy == library.DefaultSelection {
				random := s.Random
				if random == nil {
					random = rand.New(rand.NewSource(time.Now().UnixNano()))
				}
				queued = append(queued, values[random.Intn(len(values))])
			} else if selected, ok := library.SelectVariant(values, options.SelectionStrategy); ok {
				queued = append(queued, selected)
			}
		}
	}
	planned := make([]Item, 0, len(queued)*options.RepeatEach)
	for _, variant := range queued {
		for range options.RepeatEach {
			planned = append(planned, Item{VideoPath: variant.VideoPath, AudioPath: variant.AudioPath, Artist: "", Title: ""})
		}
	}
	if options.Shuffle {
		random := s.Random
		if random == nil {
			random = rand.New(rand.NewSource(time.Now().UnixNano()))
		}
		random.Shuffle(len(planned), func(i, j int) { planned[i], planned[j] = planned[j], planned[i] })
	}
	if options.MaximumItems > 0 && len(planned) > options.MaximumItems {
		planned = planned[:options.MaximumItems]
	}
	return planned, nil
}

func (s Service) commandFor(items []Item, video bool, additional []string) (string, []string, error) {
	if len(items) == 0 {
		return "", nil, fmt.Errorf("The queue is empty.")
	}
	playlistCommand, singleCommand, suffix := s.Config.AudioPlaylistCommand, s.Config.AudioSingleFileCommand, s.Config.AudioPlaylistSuffix
	pathFor := func(item Item) string { return item.AudioPath }
	if video {
		playlistCommand, singleCommand, suffix = s.Config.VideoPlaylistCommand, s.Config.VideoSingleFileCommand, s.Config.VideoPlaylistSuffix
		pathFor = func(item Item) string { return item.VideoPath }
	}
	if len(items) == 1 && len(singleCommand) > 0 {
		return singleCommand[0], append(append([]string{}, singleCommand[1:]...), append(additional, pathFor(items[0]))...), nil
	}
	if len(playlistCommand) == 0 {
		return "", nil, fmt.Errorf("player command is not configured")
	}
	paths := make([]string, len(items))
	for index, item := range items {
		paths[index] = pathFor(item)
	}
	playlist, err := writePlaylist(s.Config.DataDirectory, suffix, paths)
	if err != nil {
		return "", nil, err
	}
	arguments := make([]string, 0, len(playlistCommand)+len(additional))
	for _, argument := range playlistCommand[1:] {
		arguments = append(arguments, strings.ReplaceAll(argument, s.Config.PlaylistTemplate, playlist))
	}
	arguments = append(arguments, additional...)
	return playlistCommand[0], arguments, nil
}

func writePlaylist(directory, suffix string, paths []string) (string, error) {
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return "", err
	}
	return writePlaylistFile(directory, suffix, paths)
}
func writePlaylistFile(directory, suffix string, paths []string) (string, error) {
	file, err := os.CreateTemp(directory, "playlist-*")
	if err != nil {
		return "", err
	}
	name := file.Name()
	target := name + suffix
	defer func() { _ = file.Close() }()
	if _, err := file.WriteString(strings.Join(paths, "\n") + "\n"); err != nil {
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
