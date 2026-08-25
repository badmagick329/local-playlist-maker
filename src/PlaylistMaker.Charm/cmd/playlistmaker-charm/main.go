package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"

	"playlistmaker/charm/internal/backend"
	"playlistmaker/charm/internal/bridge"
	"playlistmaker/charm/internal/config"
	"playlistmaker/charm/internal/history"
	"playlistmaker/charm/internal/historywatch"
	"playlistmaker/charm/internal/library"
	"playlistmaker/charm/internal/migration"
	"playlistmaker/charm/internal/native"
	nativeplayback "playlistmaker/charm/internal/playback"
	"playlistmaker/charm/internal/snapshotcmp"
	"playlistmaker/charm/internal/spotify"
	"playlistmaker/charm/internal/spotifylink"
	"playlistmaker/charm/internal/tracking"
	"playlistmaker/charm/internal/tracksession"
	"playlistmaker/charm/internal/ui"
	"playlistmaker/charm/internal/updater"
)

type historySource struct {
	path   string
	tracks []library.Track
}

type mappingUpdater struct {
	service        updater.Service
	spotifyService spotifylink.Service
	config         config.Config
	historyService history.Service
	historyEnabled bool
	allowUntracked bool
}

func (u mappingUpdater) Scan(ctx context.Context) ([]updater.Item, error) { return u.service.Scan(ctx) }
func (u mappingUpdater) Ignored(ctx context.Context) ([]updater.Item, error) {
	return u.service.Ignored(ctx)
}
func (u mappingUpdater) Search(ctx context.Context, query string) ([]updater.Audio, error) {
	return u.service.Search(ctx, query)
}
func (u mappingUpdater) Confirm(videoPath, audioPath string) error {
	return u.service.Confirm(videoPath, audioPath)
}
func (u mappingUpdater) Create(videoPath, artist, title string) error {
	return u.service.Create(videoPath, artist, title)
}
func (u mappingUpdater) SpotifyScan(ctx context.Context, report func(spotifylink.ScanProgress)) (spotifylink.ScanResult, error) {
	return u.spotifyService.ScanWithProgress(ctx, report)
}
func (u mappingUpdater) SpotifySearch(ctx context.Context, query string) ([]spotifylink.Candidate, error) {
	return u.spotifyService.Search(ctx, query)
}
func (u mappingUpdater) SpotifyValidate(ctx context.Context, value string) (spotifylink.Candidate, error) {
	return u.spotifyService.Validate(ctx, value)
}
func (u mappingUpdater) SpotifyConfirm(ctx context.Context, trackID, uri string) error {
	validated, err := u.spotifyService.Validate(ctx, uri)
	if err != nil {
		return err
	}
	return u.spotifyService.Confirm(trackID, validated.URI)
}
func (u mappingUpdater) SpotifyIgnore(trackID string) error {
	return u.spotifyService.Ignore(trackID)
}
func (u mappingUpdater) Ignore(videoPath string) error  { return u.service.Ignore(videoPath) }
func (u mappingUpdater) Restore(videoPath string) error { return u.service.Restore(videoPath) }
func (u mappingUpdater) Reload(ctx context.Context) ([]library.Track, ui.PlaybackLauncher, error) {
	snapshot, err := (native.Loader{Config: u.config}).Load(ctx)
	if err != nil {
		return nil, nil, err
	}
	index, err := history.Read(u.historyService.HistoryPath())
	if err != nil {
		return nil, nil, err
	}
	tracks := history.Attach(snapshot.Tracks, index)
	return tracks, nativeplayback.Service{Tracks: tracks, Config: u.config, History: &u.historyService, HistoryEnabled: u.historyEnabled, AllowUntracked: u.allowUntracked}, nil
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
	configPath := flag.String("config", "", "path to PlaylistMaker config")
	disableHistory := flag.Bool("disable-history", false, "disable new playback-history sessions")
	allowUntracked := flag.Bool("allow-untracked-playback", false, "allow mpv playback without Spotify or local tracking")
	trackSession := flag.String("track-session", "", "run the hidden tracking helper for a playback manifest")
	migrateMapping := flag.String("migrate-mapping", "", "one-time path to the legacy video-to-audio mapping")
	check := flag.Bool("check", false, "load the selected library and exit")
	backendMode := flag.String("backend", "go", "backend mode: go, bridge, go-library, or compare")
	flag.Parse()
	if *trackSession != "" {
		manifest, err := tracksession.ReadManifest(*trackSession)
		if err != nil {
			fmt.Fprintf(os.Stderr, "PlaylistMaker tracking manifest failed: %v\n", err)
			os.Exit(1)
		}
		goConfig, err := config.Load(manifest.ConfigPath)
		if err != nil {
			_ = tracksession.WriteReady(manifest.ReadyPath, tracksession.Ready{Error: err.Error()})
			os.Exit(1)
		}
		runtime := &tracksession.Runtime{}
		if len(goConfig.LocalTrackingStartCommand) > 0 && len(goConfig.LocalTrackingStopCommand) > 0 {
			runtime.Local = tracking.Local{StartCommand: goConfig.LocalTrackingStartCommand, StopCommand: goConfig.LocalTrackingStopCommand}
		}
		if goConfig.SpotifyClientID != "" && !*check {
			auth := &spotify.Auth{ClientID: goConfig.SpotifyClientID, RedirectURI: goConfig.SpotifyRedirectURI, TokenPath: filepath.Join(goConfig.DataDirectory, "spotify-auth.json")}
			client := &spotify.Client{Auth: auth}
			runtime.Spotify = &spotify.Player{Client: client, StatePath: manifest.SpotifyStatePath, SessionID: manifest.SessionID, HelperPID: os.Getpid()}
		}
		if err := (tracksession.Runner{Runtime: runtime, DeviceName: goConfig.SpotifyDeviceName}).Run(context.Background(), *trackSession); err != nil {
			fmt.Fprintf(os.Stderr, "PlaylistMaker tracking helper failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if *trackCount < 1 || *variantCount < *trackCount {
		fmt.Fprintln(os.Stderr, "tracks must be positive and variants must be at least tracks")
		os.Exit(2)
	}

	tracks := library.Generate(*trackCount, *variantCount)
	var playback backend.PlaybackService
	var categoryPresets []config.CategoryPreset
	var updates *mappingUpdater
	historyPath := ""
	var bridgeClient *bridge.Client
	if *backendMode != "go" && *backendMode != "bridge" && *backendMode != "go-library" && *backendMode != "compare" {
		fmt.Fprintf(os.Stderr, "unsupported backend mode %q\n", *backendMode)
		os.Exit(2)
	}
	if *backendMode == "go" {
		executablePath, err := os.Executable()
		if err != nil {
			fmt.Fprintf(os.Stderr, "PlaylistMaker config discovery failed: %v\n", err)
			os.Exit(1)
		}
		resolvedConfigPath, err := config.Discover(*configPath, executablePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "PlaylistMaker config discovery failed: %v\n", err)
			os.Exit(1)
		}
		goConfig, err := config.Load(resolvedConfigPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "PlaylistMaker Go config failed: %v\n", err)
			os.Exit(1)
		}
		categoryPresets = goConfig.CategoryPresets
		if goConfig.SpotifyClientID != "" {
			auth := &spotify.Auth{ClientID: goConfig.SpotifyClientID, RedirectURI: goConfig.SpotifyRedirectURI, TokenPath: filepath.Join(goConfig.DataDirectory, "spotify-auth.json")}
			if err := spotify.Recover(context.Background(), &spotify.Client{Auth: auth}, filepath.Join(goConfig.DataDirectory, "spotify-active-session.json")); err != nil {
				fmt.Fprintf(os.Stderr, "PlaylistMaker Spotify recovery failed: %v\n", err)
			}
		}
		if *migrateMapping != "" {
			historyPath := filepath.Join(goConfig.DataDirectory, history.HistoryFileName)
			report, err := migration.Run(*migrateMapping, goConfig.MediaCatalogFile, historyPath, goConfig.FlacCacheFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "PlaylistMaker catalogue migration failed: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Migrated %d tracks and %d videos; updated %d history events; unresolved history entries: %d\n", report.Tracks, report.Videos, report.HistoryEventsUpdated, report.UnresolvedHistoryEntries)
			return
		}
		loggingEnabled := goConfig.PlaybackHistoryEnabled && !*disableHistory
		historyService := history.Service{DataDirectory: goConfig.DataDirectory, MinimumWatchedPercent: goConfig.PlaybackHistoryMinimumWatchedPercent}
		historyPath = historyService.HistoryPath()
		if !*check {
			if err := tracksession.RecoverStale(context.Background(), goConfig.DataDirectory, nil); err != nil {
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
		playback = nativeplayback.Service{Tracks: tracks, Config: goConfig, History: &historyService, HistoryEnabled: loggingEnabled, AllowUntracked: *allowUntracked}
		spotifyAuth := &spotify.Auth{ClientID: goConfig.SpotifyClientID, RedirectURI: goConfig.SpotifyRedirectURI, TokenPath: filepath.Join(goConfig.DataDirectory, "spotify-auth.json")}
		spotifyClient := &spotify.Client{Auth: spotifyAuth}
		updates = &mappingUpdater{service: updater.Service{Config: goConfig}, spotifyService: spotifylink.Service{CatalogPath: goConfig.MediaCatalogFile, CachePath: goConfig.FlacCacheFile, Auth: spotifyAuth, Client: spotifyClient}, config: goConfig, historyService: historyService, historyEnabled: loggingEnabled, allowUntracked: *allowUntracked}
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

	model := ui.New(tracks, playback).WithCategoryPresets(categoryPresets)
	if *backendMode == "go" {
		model = model.WithMappingUpdater(updates)
		model = model.WithSpotifyUpdater(updates)
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
