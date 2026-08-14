package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"

	"playlistmaker/charm/internal/backend"
	"playlistmaker/charm/internal/bridge"
	"playlistmaker/charm/internal/config"
	"playlistmaker/charm/internal/history"
	"playlistmaker/charm/internal/historywatch"
	"playlistmaker/charm/internal/library"
	"playlistmaker/charm/internal/native"
	nativeplayback "playlistmaker/charm/internal/playback"
	"playlistmaker/charm/internal/snapshotcmp"
	"playlistmaker/charm/internal/ui"
)

type historySource struct {
	path   string
	tracks []library.Track
}

func (s historySource) Refresh(ctx context.Context) ([]library.Track, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	index, err := history.Read(s.path)
	if err != nil {
		return nil, err
	}
	tracks := make([]library.Track, len(s.tracks))
	copy(tracks, s.tracks)
	for index := range tracks {
		tracks[index].Variants = append([]library.Variant(nil), tracks[index].Variants...)
	}
	return history.Attach(tracks, index), nil
}

func main() {
	trackCount := flag.Int("tracks", 1337, "number of synthetic tracks")
	variantCount := flag.Int("variants", 6420, "number of synthetic video variants")
	bridgePath := flag.String("bridge", "", "path to PlaylistMaker.Bridge executable")
	configPath := flag.String("config", "config.yaml", "path to PlaylistMaker config")
	disableHistory := flag.Bool("disable-history", false, "disable new playback-history sessions")
	check := flag.Bool("check", false, "load the selected library and exit")
	backendMode := flag.String("backend", "go", "backend mode: go, bridge, go-library, or compare")
	flag.Parse()

	if *trackCount < 1 || *variantCount < *trackCount {
		fmt.Fprintln(os.Stderr, "tracks must be positive and variants must be at least tracks")
		os.Exit(2)
	}

	tracks := library.Generate(*trackCount, *variantCount)
	var playback backend.PlaybackService
	historyPath := ""
	var bridgeClient *bridge.Client
	if *backendMode != "go" && *backendMode != "bridge" && *backendMode != "go-library" && *backendMode != "compare" {
		fmt.Fprintf(os.Stderr, "unsupported backend mode %q\n", *backendMode)
		os.Exit(2)
	}
	if *backendMode == "go" {
		goConfig, err := config.Load(*configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "PlaylistMaker Go config failed: %v\n", err)
			os.Exit(1)
		}
		loggingEnabled := goConfig.PlaybackHistoryEnabled && !*disableHistory
		historyService := history.Service{DataDirectory: goConfig.DataDirectory, MinimumWatchedPercent: goConfig.PlaybackHistoryMinimumWatchedPercent}
		historyPath = historyService.HistoryPath()
		if loggingEnabled && !*check {
			if err := historyService.Recover(); err != nil {
				fmt.Fprintf(os.Stderr, "PlaylistMaker history recovery failed: %v\n", err)
			}
		}
		nativeSnapshot, err := (native.Loader{Config: goConfig, ReadOnly: *check}).Load(context.Background())
		if err != nil {
			fmt.Fprintf(os.Stderr, "PlaylistMaker Go library failed: %v\n", err)
			os.Exit(1)
		}
		index, err := history.Read(historyService.HistoryPath())
		if err != nil {
			fmt.Fprintf(os.Stderr, "PlaylistMaker history read failed: %v\n", err)
			os.Exit(1)
		}
		tracks = history.Attach(nativeSnapshot.Tracks, index)
		playback = nativeplayback.Service{Tracks: tracks, Config: goConfig, History: &historyService, HistoryEnabled: loggingEnabled}
	} else if *bridgePath != "" {
		var err error
		bridgeClient, err = bridge.Start(*bridgePath, *configPath, *disableHistory)
		if err != nil {
			fmt.Fprintf(os.Stderr, "PlaylistMaker bridge failed: %v\n", err)
			os.Exit(1)
		}
		defer bridgeClient.Close()
		snapshot, err := bridgeClient.Load(context.Background())
		if err != nil {
			fmt.Fprintf(os.Stderr, "PlaylistMaker library load failed: %v\n", err)
			os.Exit(1)
		}
		tracks = snapshot.Tracks
		playback = bridgeClient
		if *backendMode == "go-library" || *backendMode == "compare" {
			goConfig, err := config.Load(*configPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "PlaylistMaker Go config failed: %v\n", err)
				os.Exit(1)
			}
			nativeSnapshot, err := (native.Loader{Config: goConfig}).Load(context.Background())
			if err != nil {
				fmt.Fprintf(os.Stderr, "PlaylistMaker Go library failed: %v\n", err)
				os.Exit(1)
			}
			comparison := snapshotcmp.Compare(snapshot, nativeSnapshot)
			if *backendMode == "compare" {
				fmt.Printf("Compared bridge and Go libraries: %d difference(s)\n", comparison.Total)
				for _, difference := range comparison.Differences {
					fmt.Println("- " + difference)
				}
				if !comparison.Equal() {
					os.Exit(1)
				}
				return
			}
			tracks = snapshotcmp.OverlayHistory(nativeSnapshot, snapshot).Tracks
		}
	} else {
		fmt.Fprintln(os.Stderr, "bridge, go-library, and compare modes require --bridge for temporary diagnostics")
		os.Exit(2)
	}
	if *check {
		variants := 0
		for _, track := range tracks {
			variants += len(track.Variants)
		}
		fmt.Printf(
			"Tracks: %d\nVideos: %d\nPlayback connected: %t\nHistory logging enabled: %t\n",
			len(tracks),
			variants,
			playback != nil,
			*backendMode == "go" && !*disableHistory,
		)
		return
	}

	model := ui.New(tracks, playback)
	if *backendMode == "go" {
		watcher, err := historywatch.New(historyPath, 250*time.Millisecond)
		if err != nil {
			fmt.Fprintf(os.Stderr, "PlaylistMaker history watch unavailable: %v\n", err)
			model = model.WithHistorySource(historySource{path: historyPath, tracks: tracks})
		} else {
			defer watcher.Close()
			model = model.WithHistorySource(historySource{path: historyPath, tracks: tracks}, watcher)
		}
	}
	program := tea.NewProgram(model)
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "PlaylistMaker Charm spike failed: %v\n", err)
		os.Exit(1)
	}
}
