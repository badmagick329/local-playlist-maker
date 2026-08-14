package main

import (
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"playlistmaker/charm/internal/bridge"
	"playlistmaker/charm/internal/library"
	"playlistmaker/charm/internal/ui"
)

func main() {
	trackCount := flag.Int("tracks", 1337, "number of synthetic tracks")
	variantCount := flag.Int("variants", 6420, "number of synthetic video variants")
	bridgePath := flag.String("bridge", "", "path to PlaylistMaker.Bridge executable")
	configPath := flag.String("config", "config.yaml", "path to PlaylistMaker config")
	check := flag.Bool("check", false, "load the selected library and exit")
	flag.Parse()

	if *trackCount < 1 || *variantCount < *trackCount {
		fmt.Fprintln(os.Stderr, "tracks must be positive and variants must be at least tracks")
		os.Exit(2)
	}

	tracks := library.Generate(*trackCount, *variantCount)
	var playback ui.PlaybackLauncher
	var bridgeClient *bridge.Client
	if *bridgePath != "" {
		var err error
		bridgeClient, tracks, err = bridge.Start(*bridgePath, *configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "PlaylistMaker bridge failed: %v\n", err)
			os.Exit(1)
		}
		defer bridgeClient.Close()
		playback = bridgeClient
	}
	if *check {
		variants := 0
		for _, track := range tracks {
			variants += len(track.Variants)
		}
		fmt.Printf("Tracks: %d\nVideos: %d\nPlayback connected: %t\n", len(tracks), variants, playback != nil)
		return
	}

	program := tea.NewProgram(ui.New(tracks, playback))
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "PlaylistMaker Charm spike failed: %v\n", err)
		os.Exit(1)
	}
}
