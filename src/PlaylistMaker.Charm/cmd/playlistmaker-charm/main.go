package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"playlistmaker/charm/internal/backend"
	"playlistmaker/charm/internal/bridge"
	"playlistmaker/charm/internal/config"
	"playlistmaker/charm/internal/library"
	"playlistmaker/charm/internal/native"
	"playlistmaker/charm/internal/snapshotcmp"
	"playlistmaker/charm/internal/ui"
)

func main() {
	trackCount := flag.Int("tracks", 1337, "number of synthetic tracks")
	variantCount := flag.Int("variants", 6420, "number of synthetic video variants")
	bridgePath := flag.String("bridge", "", "path to PlaylistMaker.Bridge executable")
	configPath := flag.String("config", "config.yaml", "path to PlaylistMaker config")
	disableHistory := flag.Bool("disable-history", false, "disable new playback-history sessions")
	check := flag.Bool("check", false, "load the selected library and exit")
	backendMode := flag.String("backend", "bridge", "backend mode: bridge, go-library, or compare")
	flag.Parse()

	if *trackCount < 1 || *variantCount < *trackCount {
		fmt.Fprintln(os.Stderr, "tracks must be positive and variants must be at least tracks")
		os.Exit(2)
	}

	tracks := library.Generate(*trackCount, *variantCount)
	var playback backend.PlaybackService
	var bridgeClient *bridge.Client
	if *backendMode != "bridge" && *backendMode != "go-library" && *backendMode != "compare" {
		fmt.Fprintf(os.Stderr, "unsupported backend mode %q\n", *backendMode)
		os.Exit(2)
	}
	if *bridgePath != "" {
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
	} else if *backendMode != "bridge" {
		fmt.Fprintln(os.Stderr, "go-library and compare modes require --bridge for temporary playback/history support")
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
			playback != nil && !*disableHistory,
		)
		return
	}

	program := tea.NewProgram(ui.New(tracks, playback))
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "PlaylistMaker Charm spike failed: %v\n", err)
		os.Exit(1)
	}
}
