package ui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"playlistmaker/charm/internal/backend"
	"playlistmaker/charm/internal/config"
	"playlistmaker/charm/internal/lastfm"
	"playlistmaker/charm/internal/library"
	"playlistmaker/charm/internal/spotifylink"
	"playlistmaker/charm/internal/updater"
)

type mode int

const (
	modeNavigate mode = iota
	modeSearch
	modeCategories
	modeSort
	modeQueue
	modePlaybackOptions
	modeFilters
	modeHelp
	modeDetails
	modeMappingUpdate
	modeMappingPicker
	modeSpotifyUpdate
	modeSpotifySearch
	modeLastFM
)

func (m mode) String() string {
	switch m {
	case modeSearch:
		return "SEARCH"
	case modeCategories:
		return "CATEGORIES"
	case modeSort:
		return "SORT"
	case modeQueue:
		return "QUEUE"
	case modePlaybackOptions:
		return "OPTIONS"
	case modeFilters:
		return "FILTERS"
	case modeHelp:
		return "HELP"
	case modeDetails:
		return "DETAILS"
	case modeMappingUpdate:
		return "UPDATE MAPPINGS"
	case modeMappingPicker:
		return "CHOOSE TRACK"
	case modeSpotifyUpdate:
		return "SPOTIFY LINKS"
	case modeSpotifySearch:
		return "SPOTIFY SEARCH"
	case modeLastFM:
		return "LAST.FM"
	default:
		return "NAV"
	}
}

// Model owns the UI state; service results are applied only through the event loop.
type Model struct {
	trackingErrorPath  string
	trackingErrorSince time.Time
	trackingError      string
	all                []library.Track
	filtered           []library.Track
	rows               []row
	expanded           map[string]bool
	queued             map[string]library.Variant
	queueOrder         []string
	enabled            map[library.Category]bool
	categoryPresets    []config.CategoryPreset
	query              string
	sort               library.Sort
	trackDate          *library.DateRange
	videoDate          *library.DateRange
	mode               mode
	cursor             int
	overlayCursor      int
	waitingForG        bool
	width              int
	height             int
	status             string
	theme              theme
	stats              *latencyStats
	playback           PlaybackLauncher
	historySource      HistorySource
	historyWatcher     HistoryWatcher
	historyRefreshing  bool
	historyPending     bool
	playbackMix        int
	draftMix           int
	mixTrackCount      int
	draftTrackCount    int
	savedPeriod        [3]string
	periodDraft        [3]string
	savedMethod        lastfm.MixMethod
	draftMethod        lastfm.MixMethod
	playbackOptions    backend.PlaybackOptions
	draftOptions       backend.PlaybackOptions
	filterDraft        [2]string
	optionEdit         string
	optionEditField    int
	optionError        string
	helpOffset         int
	detailsOffset      int
	launching          bool
	mappingUpdater     MappingUpdater
	mappingItems       []updater.Item
	mappingIndex       int
	mappingScanning    bool
	mappingIgnored     bool
	mappingQuery       string
	mappingCandidates  []updater.Audio
	mappingCursor      int
	mappingShowUnused  bool
	mappingDuplicate   *updater.ExistingTracksError
	mappingSession     int
	mappingRevision    int
	mappingPool        []updater.Audio
	mappingLoading     bool
	mappingPending     bool
	mappingRelink      bool
	mappingSaving      bool
	mappingDirty       bool
	spotifyUpdater     SpotifyUpdater
	spotifyItems       []spotifylink.Item
	spotifyIndex       int
	spotifyCandidate   int
	spotifyScanning    bool
	spotifyCancelling  bool
	spotifyProgress    spotifylink.ScanProgress
	spotifyScanError   string
	spotifyScan        *spotifyScanRunner
	spotifyScanCancel  context.CancelFunc
	spotifyQuery       string
	spotifyDirty       bool
	lastfm             LastFMService
	lastfmStatus       lastfm.Status
	lastfmProgress     lastfm.SyncProgress
	lastfmRunning      bool
	lastfmCancelling   bool
	lastfmRunner       *lastfmSyncRunner
	lastfmCancel       context.CancelFunc
	lastfmResetArmed   bool
}

func New(tracks []library.Track, playback ...PlaybackLauncher) Model {
	enabled := make(map[library.Category]bool, len(library.Categories))
	for _, category := range library.Categories {
		enabled[category] = category == library.MusicVideo
	}
	m := Model{
		all:             tracks,
		expanded:        make(map[string]bool),
		queued:          make(map[string]library.Variant),
		enabled:         enabled,
		sort:            library.ModifiedNewest,
		mode:            modeNavigate,
		width:           120,
		height:          36,
		theme:           newTheme(),
		stats:           &latencyStats{},
		playbackOptions: backend.DefaultPlaybackOptions(),
		optionEditField: -1,
	}
	if len(playback) > 0 {
		m.playback = playback[0]
	}
	m.refreshResults()
	return m
}

func (m Model) Init() tea.Cmd {
	if m.historySource == nil {
		return m.trackingErrorCmd()
	}
	commands := []tea.Cmd{m.startHistoryRefreshCmd(false), m.trackingErrorCmd()}
	if m.historyWatcher != nil {
		commands = append(commands, m.waitForHistoryChangeCmd())
	}
	return tea.Batch(commands...)
}

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	started := time.Now()
	defer func() { m.stats.recordUpdate(time.Since(started)) }()

	switch message := message.(type) {
	case trackingErrorTick:
		return m.handleTrackingErrorTick()
	case lastfmProgressMsg:
		return m.handleLastFMProgress(message)
	case lastfmSyncMsg:
		return m.handleLastFMSync(message)
	case lastfmActionMsg:
		return m.handleLastFMAction(message)
	case spotifyScanMsg:
		return m.handleSpotifyScan(message)
	case spotifyScanProgressMsg:
		return m.handleSpotifyScanProgress(message)
	case spotifySearchMsg:
		return m.handleSpotifySearch(message)
	case spotifyValidateMsg:
		return m.handleSpotifyValidate(message)
	case spotifySaveMsg:
		return m.handleSpotifySave(message)
	case mappingScanMsg:
		return m.handleMappingScan(message)
	case mappingIgnoredMsg:
		return m.handleMappingIgnored(message)
	case mappingDebounceMsg:
		return m.handleMappingDebounce(message)
	case mappingSearchMsg:
		return m.handleMappingSearch(message)
	case mappingConfirmMsg:
		return m.handleMappingConfirm(message)
	case mappingIgnoreMsg:
		return m.handleMappingIgnore(message)
	case libraryReloadMsg:
		return m.handleLibraryReload(message)
	case historyRefreshMsg:
		return m.handleHistoryRefresh(message)
	case historyWatchChangedMsg:
		return m.handleHistoryWatchChanged()
	case historyWatchClosedMsg:
		return m, nil
	case playbackResultMsg:
		return m.handlePlaybackResult(message)
	case tea.WindowSizeMsg:
		m.width = max(message.Width, 40)
		m.height = max(message.Height, 12)
		m.keepCursorVisible()
		m.clampOverlayState()
		return m, nil
	case tea.KeyPressMsg:
		if message.String() == "ctrl+q" {
			if m.spotifyScanCancel != nil {
				m.spotifyScanCancel()
			}
			if m.lastfmCancel != nil {
				m.lastfmCancel()
			}
			m.closeHistoryWatcher()
			return m, tea.Quit
		}
		return m.handleKey(message)
	}
	return m, nil
}
