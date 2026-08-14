package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"playlistmaker/charm/internal/backend"
	"playlistmaker/charm/internal/bridge"
	"playlistmaker/charm/internal/library"
	"playlistmaker/charm/internal/ui"
)

func main() {
	trackCount := flag.Int("tracks", 1337, "number of synthetic tracks")
	variantCount := flag.Int("variants", 6420, "number of synthetic video variants")
	bridgePath := flag.String("bridge", "", "path to PlaylistMaker.Bridge executable")
	configPath := flag.String("config", "config.yaml", "path to PlaylistMaker config")
	disableHistory := flag.Bool("disable-history", false, "disable new playback-history sessions")
	check := flag.Bool("check", false, "load the selected library and exit")
	flag.Parse()

	if *trackCount < 1 || *variantCount < *trackCount {
		fmt.Fprintln(os.Stderr, "tracks must be positive and variants must be at least tracks")
		os.Exit(2)
	}

	tracks := library.Generate(*trackCount, *variantCount)
	var playback backend.PlaybackService
	var bridgeClient *bridge.Client
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
