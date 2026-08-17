package ui

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"playlistmaker/charm/internal/backend"
	"playlistmaker/charm/internal/library"
	"playlistmaker/charm/internal/pathid"
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
		return "CHOOSE AUDIO"
	default:
		return "NAV"
	}
}

type row struct {
	trackIndex   int
	variantIndex int
}

func (r row) isVariant() bool { return r.variantIndex >= 0 }

type PlaybackLauncher = backend.PlaybackService

type HistorySource interface {
	Refresh(context.Context) ([]library.Track, error)
}

// HistoryWatcher emits debounced notifications for changes to the history file.
// It is separate from HistorySource so tests and alternate backends can inject it.
type HistoryWatcher interface {
	Changes() <-chan struct{}
	Close() error
}

type MappingUpdater interface {
	Scan(context.Context) ([]updater.Item, error)
	Ignored(context.Context) ([]updater.Item, error)
	Search(context.Context, string) ([]updater.Audio, error)
	Confirm(string, string) error
	Ignore(string) error
	Restore(string) error
	Reload(context.Context) ([]library.Track, PlaybackLauncher, error)
}

type playbackResultMsg struct {
	count      int
	err        error
	queued     bool
	queueOrder []string
}

type historyRefreshMsg struct {
	tracks []library.Track
	err    error
	manual bool
}

type historyWatchChangedMsg struct{}
type historyWatchClosedMsg struct{}
type mappingScanMsg struct {
	items []updater.Item
	err   error
}
type mappingIgnoredMsg struct {
	items []updater.Item
	err   error
}
type mappingSearchMsg struct {
	items []updater.Audio
	err   error
}
type mappingConfirmMsg struct{ err error }
type mappingIgnoreMsg struct {
	restored bool
	err      error
}
type mappingReloadMsg struct {
	tracks   []library.Track
	playback PlaybackLauncher
	err      error
}

type Model struct {
	all               []library.Track
	filtered          []library.Track
	rows              []row
	expanded          map[string]bool
	queued            map[string]library.Variant
	queueOrder        []string
	enabled           map[library.Category]bool
	query             string
	sort              library.Sort
	trackDate         *library.DateRange
	videoDate         *library.DateRange
	mode              mode
	cursor            int
	overlayCursor     int
	waitingForG       bool
	width             int
	height            int
	status            string
	theme             theme
	stats             *latencyStats
	playback          PlaybackLauncher
	historySource     HistorySource
	historyWatcher    HistoryWatcher
	historyRefreshing bool
	historyPending    bool
	playbackOptions   backend.PlaybackOptions
	draftOptions      backend.PlaybackOptions
	filterDraft       [2]string
	optionEdit        string
	optionEditField   int
	optionError       string
	helpOffset        int
	detailsOffset     int
	launching         bool
	mappingUpdater    MappingUpdater
	mappingItems      []updater.Item
	mappingIndex      int
	mappingScanning   bool
	mappingIgnored    bool
	mappingQuery      string
	mappingCandidates []updater.Audio
	mappingCursor     int
	mappingDirty      bool
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
		return nil
	}
	commands := []tea.Cmd{m.startHistoryRefreshCmd(false)}
	if m.historyWatcher != nil {
		commands = append(commands, m.waitForHistoryChangeCmd())
	}
	return tea.Batch(commands...)
}

func (m Model) WithHistorySource(source HistorySource, watcher ...HistoryWatcher) Model {
	m.historySource = source
	// Init immediately starts the first asynchronous refresh. Mark it in flight
	// now so a filesystem notification arriving during startup is coalesced.
	m.historyRefreshing = source != nil
	if len(watcher) > 0 {
		m.historyWatcher = watcher[0]
	}
	return m
}

func (m Model) WithMappingUpdater(updater MappingUpdater) Model {
	m.mappingUpdater = updater
	return m
}

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	started := time.Now()
	defer func() { m.stats.recordUpdate(time.Since(started)) }()

	switch message := message.(type) {
	case mappingScanMsg:
		m.mappingScanning = false
		if message.err != nil {
			m.status = "Mapping scan failed: " + message.err.Error()
		} else {
			m.mappingItems, m.mappingIndex, m.mappingIgnored = message.items, 0, false
			m.status = mappingSummary(message.items)
		}
		return m, nil
	case mappingIgnoredMsg:
		m.mappingScanning = false
		if message.err != nil {
			m.status = "Ignored videos failed: " + message.err.Error()
		} else {
			m.mappingItems, m.mappingIndex, m.mappingIgnored = message.items, 0, true
			m.status = fmt.Sprintf("%d ignored videos", len(message.items))
		}
		return m, nil
	case mappingSearchMsg:
		if message.err != nil {
			m.status = "Audio search failed: " + message.err.Error()
		} else {
			m.mappingCandidates, m.mappingCursor = message.items, 0
		}
		return m, nil
	case mappingConfirmMsg:
		if message.err != nil {
			m.status = "Mapping save failed: " + message.err.Error()
			return m, nil
		}
		m.mappingDirty, m.mappingIndex = true, min(m.mappingIndex+1, len(m.mappingItems))
		m.status = "Mapping saved"
		return m, nil
	case mappingIgnoreMsg:
		if message.err != nil {
			m.status = "Ignored video save failed: " + message.err.Error()
			return m, nil
		}
		m.mappingDirty = true
		m.removeCurrentMappingItem()
		if message.restored {
			m.status = "Video restored"
		} else {
			m.status = "Video ignored"
		}
		return m, nil
	case mappingReloadMsg:
		if message.err != nil {
			m.status = "Library reload failed: " + message.err.Error()
			return m, nil
		}
		m.all, m.playback = message.tracks, message.playback
		valid := m.variantIndex()
		m.queueOrder = slices.DeleteFunc(m.queueOrder, func(id string) bool { _, ok := valid[id]; return !ok })
		for id := range m.queued {
			if _, ok := valid[id]; !ok {
				delete(m.queued, id)
			}
		}
		m.refreshResults()
		m.status = "Library reloaded"
		return m, nil
	case historyRefreshMsg:
		m.historyRefreshing = false
		if message.err != nil {
			m.status = "History refresh failed: " + message.err.Error()
		} else {
			m.applyHistory(message.tracks)
			if message.manual {
				m.status = "History refreshed"
			}
		}
		if m.historyPending {
			m.historyPending = false
			m.historyRefreshing = true
			return m, m.startHistoryRefreshCmd(false)
		}
		return m, nil
	case historyWatchChangedMsg:
		if m.historySource == nil {
			return m, nil
		}
		wait := m.waitForHistoryChangeCmd()
		if m.historyRefreshing {
			m.historyPending = true
			return m, wait
		}
		m.historyRefreshing = true
		return m, tea.Batch(wait, m.startHistoryRefreshCmd(false))
	case historyWatchClosedMsg:
		return m, nil
	case playbackResultMsg:
		m.launching = false
		if message.err != nil {
			m.status = "Playback failed: " + message.err.Error()
			return m, nil
		}
		if message.queued && slices.Equal(m.queueOrder, message.queueOrder) {
			m.queued = make(map[string]library.Variant)
			m.queueOrder = nil
		}
		m.status = fmt.Sprintf("Launched %d video(s)", message.count)
		return m, nil
	case tea.WindowSizeMsg:
		m.width = max(message.Width, 40)
		m.height = max(message.Height, 12)
		m.keepCursorVisible()
		m.clampOverlayState()
		return m, nil
	case tea.KeyPressMsg:
		if message.String() == "ctrl+q" {
			m.closeHistoryWatcher()
			return m, tea.Quit
		}
		return m.handleKey(message)
	}
	return m, nil
}

func (m Model) handleKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeSearch:
		return m.handleSearchKey(key), nil
	case modeCategories:
		return m.handleCategoryKey(key), nil
	case modeSort:
		return m.handleSortKey(key), nil
	case modeQueue:
		return m.handleQueueKey(key), nil
	case modePlaybackOptions:
		return m.handleOptionsKey(key), nil
	case modeFilters:
		return m.handleFiltersKey(key), nil
	case modeHelp:
		return m.handleHelpKey(key), nil
	case modeDetails:
		return m.handleDetailsKey(key), nil
	case modeMappingUpdate:
		return m.handleMappingUpdateKey(key)
	case modeMappingPicker:
		return m.handleMappingPickerKey(key)
	default:
		return m.handleNavigationKey(key)
	}
}

func (m Model) launchQueue() (tea.Model, tea.Cmd) {
	if len(m.queueOrder) == 0 {
		return m.launchHighlighted()
	}
	return m.launchIDs(append([]string(nil), m.queueOrder...), true)
}

func (m Model) launchHighlighted() (tea.Model, tea.Cmd) {
	current, ok := m.currentRow()
	if !ok {
		m.status = "No selected media"
		return m, nil
	}
	track := m.filtered[current.trackIndex]
	variant := library.Variant{}
	if current.isVariant() {
		variant = track.Variants[current.variantIndex]
	} else {
		var found bool
		variant, found = m.selectVariant(library.EligibleVariants(track, m.currentQuery()))
		if !found {
			m.status = "No eligible media"
			return m, nil
		}
	}
	return m.launchIDs([]string{variant.ID}, false)
}

func (m Model) launchIDs(ids []string, queued bool) (tea.Model, tea.Cmd) {
	if m.launching {
		m.status = "Playback launch already in progress"
		return m, nil
	}
	if len(ids) == 0 {
		m.status = "Queue is empty"
		return m, nil
	}
	if m.playback == nil {
		m.status = "Playback is unavailable in synthetic spike mode"
		return m, nil
	}
	m.launching = true
	variants := m.queued
	if !queued {
		variants = m.variantIndex()
	}
	m.status = fmt.Sprintf("Launching %d planned video(s)…", plannedCount(ids, variants, m.playbackOptions))
	return m, func() tea.Msg {
		result, err := m.playback.Launch(context.Background(), backend.PlaybackRequest{
			VideoIDs: ids,
			Options:  m.playbackOptions,
		})
		if err == nil && !result.Succeeded {
			err = fmt.Errorf("%s", result.UserSafeError)
		}
		return playbackResultMsg{count: result.PlannedVideoCount, err: err, queued: queued, queueOrder: append([]string(nil), ids...)}
	}
}

func (m Model) variantIndex() map[string]library.Variant {
	result := make(map[string]library.Variant)
	for _, track := range m.all {
		for _, variant := range track.Variants {
			result[variant.ID] = variant
		}
	}
	return result
}

func (m Model) selectVariant(candidates []library.Variant) (library.Variant, bool) {
	return library.SelectVariant(candidates, m.playbackOptions.SelectionStrategy)
}

func (m Model) handleNavigationKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.String() != "g" {
		m.waitingForG = false
	}
	if key.Text == "R" {
		return m.requestHistoryRefresh()
	}

	switch key.String() {
	case "j", "down", "ctrl+j":
		m.moveCursor(1)
	case "k", "up", "ctrl+k":
		m.moveCursor(-1)
	case "ctrl+d", "pgdown":
		m.moveCursor(m.pageSize())
	case "ctrl+u", "pgup":
		m.moveCursor(-m.pageSize())
	case "g":
		if m.waitingForG {
			m.cursor = 0
			m.waitingForG = false
		} else {
			m.waitingForG = true
		}
	case "G":
		if len(m.rows) > 0 {
			m.cursor = len(m.rows) - 1
		}
	case "h", "left":
		m.collapseCurrent()
	case "l", "right", "enter":
		m.toggleExpanded()
	case "space":
		m.toggleQueue()
		m.moveCursor(1)
	case "a":
		m.queueCurrentTrack()
	case "A":
		m.queueFilteredTracks()
	case "/":
		m.mode = modeSearch
		m.status = "Search mode: type freely; Enter or Esc returns to navigation"
	case "c":
		m.mode = modeCategories
		m.overlayCursor = 0
	case "s":
		m.mode = modeSort
		m.overlayCursor = slices.Index(library.Sorts, m.sort)
	case "o":
		return m.launchQueue()
	case "p":
		m.mode, m.overlayCursor, m.draftOptions = modePlaybackOptions, 0, m.playbackOptions
		m.optionEdit, m.optionEditField, m.optionError = "", -1, ""
	case "R", "shift+r":
		return m.requestHistoryRefresh()
	case "f":
		m.mode, m.overlayCursor = modeFilters, 0
		m.filterDraft = [2]string{}
		if m.trackDate != nil {
			m.filterDraft[0] = m.trackDate.Label
		}
		if m.videoDate != nil {
			m.filterDraft[1] = m.videoDate.Label
		}
	case "q":
		m.mode = modeQueue
		m.overlayCursor = min(m.overlayCursor, max(len(m.queueOrder)-1, 0))
	case "?":
		m.mode, m.helpOffset = modeHelp, 0
	case "d":
		m.mode, m.detailsOffset = modeDetails, 0
	case "u":
		if m.mappingUpdater == nil {
			m.status = "Mapping updates are unavailable"
			return m, nil
		}
		m.mode, m.mappingScanning, m.mappingItems, m.mappingIndex, m.mappingIgnored = modeMappingUpdate, true, nil, 0, false
		m.status = "Scanning video and audio folders…"
		return m, m.mappingScanCmd()
	case "esc":
		m.status = "Esc closes modes; Ctrl+Q quits"
	}
	return m, nil
}

func (m Model) requestHistoryRefresh() (tea.Model, tea.Cmd) {
	if m.historySource == nil {
		m.status = "History refresh is unavailable"
		return m, nil
	}
	if m.historyRefreshing {
		m.status = "History refresh already in progress"
		return m, nil
	}
	m.status, m.historyRefreshing = "Refreshing history…", true
	return m, m.startHistoryRefreshCmd(true)
}

func (m Model) startHistoryRefreshCmd(manual bool) tea.Cmd {
	if m.historySource == nil {
		return nil
	}
	source := m.historySource
	return func() tea.Msg {
		tracks, err := source.Refresh(context.Background())
		return historyRefreshMsg{tracks: tracks, err: err, manual: manual}
	}
}

func (m Model) waitForHistoryChangeCmd() tea.Cmd {
	if m.historyWatcher == nil {
		return nil
	}
	changes := m.historyWatcher.Changes()
	return func() tea.Msg {
		if _, ok := <-changes; !ok {
			return historyWatchClosedMsg{}
		}
		return historyWatchChangedMsg{}
	}
}

func (m *Model) closeHistoryWatcher() {
	if m.historyWatcher == nil {
		return
	}
	_ = m.historyWatcher.Close()
	m.historyWatcher = nil
}

func (m *Model) applyHistory(updated []library.Track) {
	byTrack := make(map[string]library.Track, len(updated))
	for _, track := range updated {
		byTrack[track.ID] = track
	}
	for trackIndex := range m.all {
		fresh, ok := byTrack[m.all[trackIndex].ID]
		if !ok {
			continue
		}
		m.all[trackIndex].History = fresh.History
		byVariant := make(map[string]library.History, len(fresh.Variants))
		for _, variant := range fresh.Variants {
			byVariant[variant.ID] = variant.History
		}
		for variantIndex := range m.all[trackIndex].Variants {
			if value, ok := byVariant[m.all[trackIndex].Variants[variantIndex].ID]; ok {
				m.all[trackIndex].Variants[variantIndex].History = value
			}
		}
	}
	for id, variant := range m.queued {
		for _, track := range m.all {
			for _, fresh := range track.Variants {
				if fresh.ID == id {
					variant.History = fresh.History
					m.queued[id] = variant
				}
			}
		}
	}
	m.refreshResults()
}

func (m Model) handleDetailsKey(key tea.KeyPressMsg) Model {
	maxOffset := max(len(m.detailsLines())-overlayListCapacity(m.height), 0)
	switch key.String() {
	case "esc", "d":
		m.mode = modeNavigate
	case "j", "down", "ctrl+j":
		m.detailsOffset = min(m.detailsOffset+1, maxOffset)
	case "k", "up", "ctrl+k":
		m.detailsOffset = max(m.detailsOffset-1, 0)
	case "ctrl+d", "pgdown":
		m.detailsOffset = min(m.detailsOffset+overlayListCapacity(m.height), maxOffset)
	case "ctrl+u", "pgup":
		m.detailsOffset = max(m.detailsOffset-overlayListCapacity(m.height), 0)
	case "g":
		m.detailsOffset = 0
	case "G":
		m.detailsOffset = maxOffset
	}
	return m
}

func (m Model) handleMappingUpdateKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.mappingScanning {
		return m, nil
	}
	switch key.String() {
	case "u", "esc":
		m.mode = modeNavigate
		if m.mappingDirty {
			return m, m.mappingReloadCmd()
		}
		return m, nil
	case "r":
		m.mappingIgnored = false
		m.mappingScanning, m.mappingItems, m.mappingIndex = true, nil, 0
		m.status = "Scanning video and audio folders…"
		return m, m.mappingScanCmd()
	case "s":
		if m.mappingIgnored {
			return m, nil
		}
		m.mappingIndex = min(m.mappingIndex+1, len(m.mappingItems))
		m.status = "Mapping skipped"
		return m, nil
	case "/":
		if m.mappingIgnored {
			return m, nil
		}
		m.mode, m.mappingQuery, m.mappingCandidates, m.mappingCursor = modeMappingPicker, "", nil, 0
		return m, nil
	case "i":
		item, ok := m.currentMappingItem()
		if !ok {
			return m, nil
		}
		if m.mappingIgnored {
			return m, m.mappingIgnoreCmd(item.VideoPath, true)
		}
		return m, m.mappingIgnoreCmd(item.VideoPath, false)
	case "I":
		m.mappingItems, m.mappingIndex, m.mappingScanning = nil, 0, true
		if m.mappingIgnored {
			m.mappingIgnored = false
			m.status = "Scanning video and audio folders…"
			return m, m.mappingScanCmd()
		}
		m.status = "Loading ignored videos…"
		return m, m.mappingIgnoredCmd()
	case "enter":
		if m.mappingIgnored {
			return m, nil
		}
		item, ok := m.currentMappingItem()
		if !ok || item.AudioPath == "" {
			m.status = "Choose an audio track first"
			return m, nil
		}
		return m, m.mappingConfirmCmd(item.VideoPath, item.AudioPath)
	}
	return m, nil
}

func (m Model) handleMappingPickerKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc", "/":
		m.mode = modeMappingUpdate
		return m, nil
	case "j", "down":
		m.mappingCursor = min(m.mappingCursor+1, max(len(m.mappingCandidates)-1, 0))
	case "k", "up":
		m.mappingCursor = max(m.mappingCursor-1, 0)
	case "enter":
		item, ok := m.currentMappingItem()
		if !ok || m.mappingCursor >= len(m.mappingCandidates) {
			return m, nil
		}
		item.AudioPath = m.mappingCandidates[m.mappingCursor].Path
		item.AudioArtist = m.mappingCandidates[m.mappingCursor].Artist
		item.AudioTitle = m.mappingCandidates[m.mappingCursor].Title
		item.Reason = "Selected manually"
		m.mappingItems[m.mappingIndex] = item
		m.mode = modeMappingUpdate
	case "backspace":
		if m.mappingQuery != "" {
			_, size := utf8.DecodeLastRuneInString(m.mappingQuery)
			m.mappingQuery = m.mappingQuery[:len(m.mappingQuery)-size]
			return m, m.mappingSearchCmd()
		}
	default:
		if key.Text != "" && !key.Mod.Contains(tea.ModCtrl) && !key.Mod.Contains(tea.ModAlt) {
			m.mappingQuery += key.Text
			return m, m.mappingSearchCmd()
		}
	}
	return m, nil
}

func (m Model) currentMappingItem() (updater.Item, bool) {
	if m.mappingIndex < 0 || m.mappingIndex >= len(m.mappingItems) {
		return updater.Item{}, false
	}
	return m.mappingItems[m.mappingIndex], true
}

func (m Model) mappingScanCmd() tea.Cmd {
	service := m.mappingUpdater
	return func() tea.Msg {
		items, err := service.Scan(context.Background())
		return mappingScanMsg{items: items, err: err}
	}
}
func (m Model) mappingIgnoredCmd() tea.Cmd {
	service := m.mappingUpdater
	return func() tea.Msg {
		items, err := service.Ignored(context.Background())
		return mappingIgnoredMsg{items: items, err: err}
	}
}
func (m Model) mappingSearchCmd() tea.Cmd {
	service, query := m.mappingUpdater, m.mappingQuery
	return func() tea.Msg {
		items, err := service.Search(context.Background(), query)
		return mappingSearchMsg{items: items, err: err}
	}
}
func (m Model) mappingConfirmCmd(video, audio string) tea.Cmd {
	service := m.mappingUpdater
	return func() tea.Msg { return mappingConfirmMsg{err: service.Confirm(video, audio)} }
}
func (m Model) mappingIgnoreCmd(video string, restored bool) tea.Cmd {
	service := m.mappingUpdater
	return func() tea.Msg {
		if restored {
			return mappingIgnoreMsg{restored: true, err: service.Restore(video)}
		}
		return mappingIgnoreMsg{err: service.Ignore(video)}
	}
}
func (m Model) mappingReloadCmd() tea.Cmd {
	service := m.mappingUpdater
	return func() tea.Msg {
		tracks, playback, err := service.Reload(context.Background())
		return mappingReloadMsg{tracks: tracks, playback: playback, err: err}
	}
}

func (m *Model) removeCurrentMappingItem() {
	if m.mappingIndex < 0 || m.mappingIndex >= len(m.mappingItems) {
		return
	}
	m.mappingItems = slices.Delete(m.mappingItems, m.mappingIndex, m.mappingIndex+1)
	m.mappingIndex = min(m.mappingIndex, max(len(m.mappingItems)-1, 0))
}

func mappingSummary(items []updater.Item) string {
	suggestions := 0
	for _, item := range items {
		if item.AudioPath != "" {
			suggestions++
		}
	}
	return fmt.Sprintf("%d unmapped videos • %d suggestions", len(items), suggestions)
}

func (m Model) handleHelpKey(key tea.KeyPressMsg) Model {
	maxOffset := m.helpMaxOffset()
	switch key.String() {
	case "esc", "?":
		m.mode = modeNavigate
	case "j", "down", "ctrl+j":
		m.helpOffset = min(m.helpOffset+1, maxOffset)
	case "k", "up", "ctrl+k":
		m.helpOffset = max(m.helpOffset-1, 0)
	case "ctrl+d", "pgdown":
		m.helpOffset = min(m.helpOffset+max(m.height-8, 1), maxOffset)
	case "ctrl+u", "pgup":
		m.helpOffset = max(m.helpOffset-max(m.height-8, 1), 0)
	case "g":
		m.helpOffset = 0
	case "G":
		m.helpOffset = maxOffset
	}
	return m
}

func (m Model) helpMaxOffset() int {
	return max(len(helpLines())-overlayListCapacity(m.height), 0)
}

func (m *Model) clampOverlayState() {
	m.helpOffset = min(max(m.helpOffset, 0), m.helpMaxOffset())
	switch m.mode {
	case modeCategories:
		m.overlayCursor = min(max(m.overlayCursor, 0), len(library.Categories)-1)
	case modeSort:
		m.overlayCursor = min(max(m.overlayCursor, 0), len(library.Sorts)-1)
	case modeQueue:
		m.overlayCursor = min(max(m.overlayCursor, 0), max(len(m.queueOrder)-1, 0))
	case modePlaybackOptions:
		m.overlayCursor = min(max(m.overlayCursor, 0), 4)
	case modeFilters:
		m.overlayCursor = min(max(m.overlayCursor, 0), 3)
	case modeDetails:
		maxOffset := max(len(m.detailsLines())-overlayListCapacity(m.height), 0)
		m.detailsOffset = min(max(m.detailsOffset, 0), maxOffset)
	}
}

func (m Model) handleOptionsKey(key tea.KeyPressMsg) Model {
	switch key.String() {
	case "esc":
		m.mode = modeNavigate
		return m
	case "p":
		m.mode = modeNavigate
		return m
	case "j", "down", "ctrl+j":
		if !m.commitOptionEdit() {
			return m
		}
		m.overlayCursor = min(m.overlayCursor+1, 4)
		return m
	case "k", "up", "ctrl+k":
		if !m.commitOptionEdit() {
			return m
		}
		m.overlayCursor = max(m.overlayCursor-1, 0)
		return m
	case "r":
		m.draftOptions = backend.DefaultPlaybackOptions()
		m.clearOptionEdit()
		return m
	case "enter":
		if !m.commitOptionEdit() {
			return m
		}
		m.playbackOptions = m.draftOptions
		m.mode = modeNavigate
		m.status = "Playback options saved"
		return m
	case "space":
		if m.overlayCursor == 0 {
			m.draftOptions.Shuffle = !m.draftOptions.Shuffle
		}
		if m.overlayCursor == 1 {
			m.draftOptions.OneVideoPerTrack = !m.draftOptions.OneVideoPerTrack
		}
		if m.overlayCursor == 4 {
			m.draftOptions.SelectionStrategy = m.draftOptions.SelectionStrategy.Next(1)
		}
		return m
	case "h", "left":
		m.clearOptionEdit()
		if m.overlayCursor == 2 {
			m.draftOptions.RepeatEach = max(1, m.draftOptions.RepeatEach-1)
		} else if m.overlayCursor == 3 {
			m.draftOptions.MaximumItems = max(0, m.draftOptions.MaximumItems-1)
		} else if m.overlayCursor == 4 {
			m.draftOptions.SelectionStrategy = m.draftOptions.SelectionStrategy.Next(-1)
		}
		return m
	case "l", "right":
		m.clearOptionEdit()
		if m.overlayCursor == 2 {
			m.draftOptions.RepeatEach = min(10, m.draftOptions.RepeatEach+1)
		} else if m.overlayCursor == 3 {
			m.draftOptions.MaximumItems++
		} else if m.overlayCursor == 4 {
			m.draftOptions.SelectionStrategy = m.draftOptions.SelectionStrategy.Next(1)
		}
		return m
	case "backspace":
		if m.overlayCursor >= 2 && m.overlayCursor <= 3 {
			if m.optionEditField != m.overlayCursor {
				m.optionEditField = m.overlayCursor
				m.optionEdit = m.optionValue()
			}
			if m.optionEdit != "" {
				m.optionEdit = m.optionEdit[:len(m.optionEdit)-1]
			}
			m.applyOptionEdit()
		}
		return m
	}
	if key.Text >= "0" && key.Text <= "9" {
		if m.overlayCursor >= 2 && m.overlayCursor <= 3 {
			if m.optionEditField != m.overlayCursor {
				m.optionEditField, m.optionEdit = m.overlayCursor, ""
			}
			m.optionEdit += key.Text
			m.applyOptionEdit()
		}
	}
	return m
}

func (m *Model) clearOptionEdit() {
	m.optionEdit, m.optionError, m.optionEditField = "", "", -1
}

func (m Model) optionValue() string {
	if m.overlayCursor == 2 {
		return strconv.Itoa(m.draftOptions.RepeatEach)
	}
	return strconv.Itoa(m.draftOptions.MaximumItems)
}

func (m *Model) applyOptionEdit() bool {
	if m.optionEdit == "" {
		m.optionError = "A number is required"
		return false
	}
	value, err := strconv.Atoi(m.optionEdit)
	if err != nil {
		m.optionError = "Number is too large"
		return false
	}
	if m.optionEditField == 2 {
		if value < 1 || value > 10 {
			m.optionError = "Repeat must be 1 through 10"
			return false
		}
		m.draftOptions.RepeatEach = value
	} else if m.optionEditField == 3 {
		if value < 0 {
			m.optionError = "Maximum must be zero or greater"
			return false
		}
		m.draftOptions.MaximumItems = value
	}
	m.optionError = ""
	return true
}

func (m *Model) commitOptionEdit() bool {
	if m.optionEditField < 0 {
		return true
	}
	if !m.applyOptionEdit() {
		return false
	}
	m.clearOptionEdit()
	return true
}

func (m Model) handleFiltersKey(key tea.KeyPressMsg) Model {
	switch key.String() {
	case "esc", "f":
		m.mode = modeNavigate
		return m
	case "j", "down", "ctrl+j":
		m.overlayCursor = min(m.overlayCursor+1, 3)
		return m
	case "k", "up", "ctrl+k":
		m.overlayCursor = max(m.overlayCursor-1, 0)
		return m
	case "ctrl+u":
		if m.overlayCursor < 2 {
			m.filterDraft[m.overlayCursor] = ""
		}
		return m
	case "backspace":
		if m.overlayCursor < 2 && len(m.filterDraft[m.overlayCursor]) > 0 {
			_, s := utf8.DecodeLastRuneInString(m.filterDraft[m.overlayCursor])
			m.filterDraft[m.overlayCursor] = m.filterDraft[m.overlayCursor][:len(m.filterDraft[m.overlayCursor])-s]
		}
		return m
	case "r":
		m.resetFilters()
		return m
	case "enter":
		if m.overlayCursor == 3 {
			m.resetFilters()
			return m
		}
		track, err := library.ParseDateRange(m.filterDraft[0])
		if err != nil {
			m.status = "Track release: " + err.Error()
			return m
		}
		video, err := library.ParseDateRange(m.filterDraft[1])
		if err != nil {
			m.status = "Video date: " + err.Error()
			return m
		}
		m.trackDate, m.videoDate, m.mode = track, video, modeNavigate
		m.refreshResults()
		return m
	}
	if m.overlayCursor < 2 && key.Text != "" {
		m.filterDraft[m.overlayCursor] += key.Text
	}
	return m
}
func (m *Model) resetFilters() {
	m.query = ""
	m.trackDate = nil
	m.videoDate = nil
	for _, c := range library.Categories {
		m.enabled[c] = c == library.MusicVideo
	}
	m.mode = modeNavigate
	m.cursor = 0
	m.refreshResults()
}

func (m *Model) queueCurrentTrack() {
	current, ok := m.currentRow()
	if !ok {
		m.status = "No selected media"
		return
	}
	track := m.filtered[current.trackIndex]
	added, skipped := 0, 0
	for _, variant := range library.EligibleVariants(track, m.currentQuery()) {
		if m.queuedID(variant.ID) != "" {
			skipped++
			continue
		}
		m.queued[variant.ID] = variant
		m.queueOrder = append(m.queueOrder, variant.ID)
		added++
	}
	if skipped > 0 {
		m.status = fmt.Sprintf("Queued %d videos from this track; %d already queued", added, skipped)
	} else {
		m.status = fmt.Sprintf("Queued %d videos from this track", added)
	}
}

func (m *Model) queueFilteredTracks() {
	added, skipped := 0, 0
	for _, track := range m.filtered {
		variant, ok := m.selectVariant(library.EligibleVariants(track, m.currentQuery()))
		if !ok {
			continue
		}
		if m.queuedID(variant.ID) != "" {
			skipped++
			continue
		}
		m.queued[variant.ID] = variant
		m.queueOrder = append(m.queueOrder, variant.ID)
		added++
	}
	if len(m.filtered) == 0 {
		m.status = "No matching tracks"
	} else if skipped > 0 {
		m.status = fmt.Sprintf("Queued %d tracks; %d already queued", added, skipped)
	} else {
		m.status = fmt.Sprintf("Queued %d tracks", added)
	}
}

func (m Model) handleSearchKey(key tea.KeyPressMsg) Model {
	switch key.String() {
	case "esc", "enter", "/":
		m.mode = modeNavigate
		m.status = "Search retained"
	case "ctrl+j", "down":
		m.moveCursor(1)
	case "ctrl+k", "up":
		m.moveCursor(-1)
	case "ctrl+u":
		m.query = ""
		m.refreshResults()
		m.cursor = 0
	case "backspace":
		if m.query != "" {
			_, size := utf8.DecodeLastRuneInString(m.query)
			m.query = m.query[:len(m.query)-size]
			m.refreshResults()
			m.cursor = 0
		}
	case "space":
		m.query += " "
		m.refreshResults()
		m.cursor = 0
	default:
		if key.Text != "" && !key.Mod.Contains(tea.ModCtrl) && !key.Mod.Contains(tea.ModAlt) {
			m.query += key.Text
			m.refreshResults()
			m.cursor = 0
		}
	}
	return m
}

func (m Model) handleCategoryKey(key tea.KeyPressMsg) Model {
	switch key.String() {
	case "j", "down", "ctrl+j":
		m.overlayCursor = min(m.overlayCursor+1, len(library.Categories)-1)
	case "k", "up", "ctrl+k":
		m.overlayCursor = max(m.overlayCursor-1, 0)
	case "space", "enter":
		category := library.Categories[m.overlayCursor]
		m.enabled[category] = !m.enabled[category]
		m.refreshResults()
	case "c", "esc":
		m.mode = modeNavigate
	}
	return m
}

func (m Model) handleSortKey(key tea.KeyPressMsg) Model {
	switch key.String() {
	case "j", "down", "ctrl+j":
		m.overlayCursor = min(m.overlayCursor+1, len(library.Sorts)-1)
	case "k", "up", "ctrl+k":
		m.overlayCursor = max(m.overlayCursor-1, 0)
	case "enter", "space":
		m.sort = library.Sorts[m.overlayCursor]
		m.refreshResults()
		m.cursor = 0
		m.mode = modeNavigate
		m.status = "Sorted by " + m.sort.String()
	case "esc", "s":
		m.mode = modeNavigate
	}
	return m
}

func (m Model) handleQueueKey(key tea.KeyPressMsg) Model {
	if key.Text == "C" {
		m.queued = make(map[string]library.Variant)
		m.queueOrder = nil
		m.overlayCursor = 0
		m.status = "Queue cleared"
		return m
	}
	switch key.String() {
	case "j", "down", "ctrl+j":
		m.overlayCursor = min(m.overlayCursor+1, max(len(m.queueOrder)-1, 0))
	case "k", "up", "ctrl+k":
		m.overlayCursor = max(m.overlayCursor-1, 0)
	case "delete", "backspace", "space":
		m.removeQueueAt(m.overlayCursor)
	case "shift+j":
		m.moveQueue(1)
	case "shift+k":
		m.moveQueue(-1)
	case "q", "esc":
		m.mode = modeNavigate
	}
	return m
}

func (m *Model) refreshResults() {
	selectedTrackID := ""
	selectedVariantID := ""
	if current, ok := m.currentRow(); ok {
		selectedTrackID = m.filtered[current.trackIndex].ID
		if current.isVariant() {
			selectedVariantID = m.filtered[current.trackIndex].Variants[current.variantIndex].ID
		}
	}
	m.filtered = library.FilterAndSort(m.all, m.currentQuery())
	m.rebuildRows()
	if selectedVariantID != "" {
		for index, current := range m.rows {
			if current.isVariant() && m.filtered[current.trackIndex].Variants[current.variantIndex].ID == selectedVariantID {
				m.cursor = index
				m.clampOverlayState()
				m.keepCursorVisible()
				return
			}
		}
	}
	if selectedTrackID != "" {
		for index, current := range m.rows {
			if !current.isVariant() && m.filtered[current.trackIndex].ID == selectedTrackID {
				m.cursor = index
				break
			}
		}
	}
	m.clampOverlayState()
	m.keepCursorVisible()
}

func (m Model) currentQuery() library.Query {
	return library.Query{SearchText: m.query, Enabled: m.enabled, TrackRelease: m.trackDate, VideoDate: m.videoDate, Sort: m.sort}
}

func (m Model) isEligible(variant library.Variant) bool {
	return m.enabled[variant.Category] && (m.videoDate == nil || m.videoDate.Contains(variant.Date))
}

func (m *Model) rebuildRows() {
	rows := make([]row, 0, len(m.filtered)+64)
	for trackIndex, track := range m.filtered {
		rows = append(rows, row{trackIndex: trackIndex, variantIndex: -1})
		if !m.expanded[track.ID] {
			continue
		}
		variantOrder := make([]int, 0, len(track.Variants))
		if selected, ok := m.selectVariant(library.EligibleVariants(track, m.currentQuery())); ok {
			for variantIndex, variant := range track.Variants {
				if variant.ID == selected.ID {
					variantOrder = append(variantOrder, variantIndex)
					break
				}
			}
		}
		for variantIndex, variant := range track.Variants {
			if len(variantOrder) == 0 || variant.ID != track.Variants[variantOrder[0]].ID {
				variantOrder = append(variantOrder, variantIndex)
			}
		}
		for _, variantIndex := range variantOrder {
			variant := track.Variants[variantIndex]
			if m.isEligible(variant) {
				rows = append(rows, row{trackIndex: trackIndex, variantIndex: variantIndex})
			}
		}
	}
	m.rows = rows
}

func (m *Model) moveCursor(delta int) {
	if len(m.rows) == 0 {
		m.cursor = 0
		return
	}
	m.cursor = min(max(m.cursor+delta, 0), len(m.rows)-1)
}

func (m Model) pageSize() int {
	return max(m.height-4, 1)
}

func (m *Model) toggleExpanded() {
	current, ok := m.currentRow()
	if !ok {
		return
	}
	track := m.filtered[current.trackIndex]
	if current.isVariant() {
		m.toggleQueue()
		return
	}
	m.expanded[track.ID] = !m.expanded[track.ID]
	m.rebuildRows()
}

func (m *Model) collapseCurrent() {
	current, ok := m.currentRow()
	if !ok {
		return
	}
	track := m.filtered[current.trackIndex]
	if current.isVariant() {
		for index := m.cursor; index >= 0; index-- {
			if !m.rows[index].isVariant() {
				m.cursor = index
				break
			}
		}
		return
	}
	if m.expanded[track.ID] {
		m.expanded[track.ID] = false
		m.rebuildRows()
	}
}

func (m *Model) toggleQueue() {
	current, ok := m.currentRow()
	if !ok {
		return
	}
	track := m.filtered[current.trackIndex]
	variant := track.Variants[0]
	if current.isVariant() {
		variant = track.Variants[current.variantIndex]
	} else {
		defaultVariant, ok := m.selectVariant(library.EligibleVariants(track, m.currentQuery()))
		if !ok {
			return
		}
		variant = defaultVariant
	}
	if queuedID := m.queuedID(variant.ID); queuedID != "" {
		delete(m.queued, queuedID)
		m.queueOrder = slices.DeleteFunc(m.queueOrder, func(id string) bool { return id == queuedID })
		m.status = "Removed from queue"
		return
	}
	m.queued[variant.ID] = variant
	m.queueOrder = append(m.queueOrder, variant.ID)
	m.status = "Queued " + variant.Filename
}

func (m Model) queuedID(id string) string {
	identity := pathid.ComparisonKey(id)
	for _, queuedID := range m.queueOrder {
		if pathid.ComparisonKey(queuedID) == identity {
			return queuedID
		}
	}
	return ""
}

func (m *Model) removeQueueAt(index int) {
	if index < 0 || index >= len(m.queueOrder) {
		return
	}
	id := m.queueOrder[index]
	delete(m.queued, id)
	m.queueOrder = slices.Delete(m.queueOrder, index, index+1)
	m.overlayCursor = min(index, max(len(m.queueOrder)-1, 0))
}

func (m *Model) moveQueue(delta int) {
	if len(m.queueOrder) < 2 {
		return
	}
	target := min(max(m.overlayCursor+delta, 0), len(m.queueOrder)-1)
	if target == m.overlayCursor {
		return
	}
	m.queueOrder[m.overlayCursor], m.queueOrder[target] = m.queueOrder[target], m.queueOrder[m.overlayCursor]
	m.overlayCursor = target
}

func (m Model) currentRow() (row, bool) {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return row{}, false
	}
	return m.rows[m.cursor], true
}

func (m *Model) keepCursorVisible() {
	if len(m.rows) == 0 {
		m.cursor = 0
		return
	}
	m.cursor = min(max(m.cursor, 0), len(m.rows)-1)
}

func (m Model) View() tea.View {
	started := time.Now()
	content := m.render()
	m.stats.recordView(time.Since(started))
	view := tea.NewView(content)
	view.AltScreen = true
	view.WindowTitle = "PlaylistMaker Charm performance spike"
	return view
}

func (m Model) render() string {
	width := max(m.width, 40)
	height := max(m.height, 12)
	bodyHeight := max(height-4, 1)

	header := m.renderHeader(width)
	body := m.renderRows(width, bodyHeight)
	footer := m.renderFooter(width)
	base := strings.Join([]string{header, body, footer}, "\n")

	if m.mode == modeCategories || m.mode == modeSort || m.mode == modeQueue || m.mode == modePlaybackOptions || m.mode == modeFilters || m.mode == modeHelp || m.mode == modeDetails || m.mode == modeMappingUpdate || m.mode == modeMappingPicker {
		base = m.renderOverlay(base, width, height)
	}
	return base
}

func (m Model) renderHeader(width int) string {
	modeText := m.theme.accent.Render(" " + m.mode.String() + " ")
	query := m.query
	if query == "" {
		query = m.theme.muted.Render("press / to search")
	} else if m.mode == modeSearch {
		query += m.theme.accent.Render("▏")
	}
	left := modeText + "  " + query
	filters := m.activeFilterLabel()
	right := fmt.Sprintf("%d tracks  •  %d queued  •  %s%s", len(m.filtered), len(m.queueOrder), m.sort, filters)
	return joinAligned(left, m.theme.muted.Render(right), width)
}

func (m Model) activeFilterLabel() string {
	labels := make([]string, 0, 2)
	if m.trackDate != nil {
		labels = append(labels, "track release "+m.trackDate.Label)
	}
	if m.videoDate != nil {
		labels = append(labels, "video date "+m.videoDate.Label)
	}
	if len(labels) == 0 {
		return ""
	}
	return "  •  " + strings.Join(labels, ", ")
}

func (m Model) renderRows(width, height int) string {
	if len(m.rows) == 0 {
		return padLines(m.theme.warning.Render("No matching tracks"), height)
	}
	start := m.cursor - height/2
	start = min(max(start, 0), max(len(m.rows)-height, 0))
	end := min(start+height, len(m.rows))
	lines := make([]string, 0, height)
	for index := start; index < end; index++ {
		lines = append(lines, m.renderRow(m.rows[index], index == m.cursor, width))
	}
	return padLines(strings.Join(lines, "\n"), height)
}

func (m Model) renderRow(current row, selected bool, width int) string {
	track := m.filtered[current.trackIndex]
	if current.isVariant() {
		variant := track.Variants[current.variantIndex]
		mark := "  "
		if m.queuedID(variant.ID) != "" {
			mark = m.theme.queued.Render("● ")
		}
		left := "    " + mark + m.theme.variant.Render(variant.Filename)
		right := m.theme.muted.Render(fmt.Sprintf("%s  %s", variant.DateLabel, variant.Category))
		line := joinAligned(left, right, width)
		if selected {
			return m.theme.selected.Width(width).Render(stripStyles(line))
		}
		return line
	}

	expansion := "›"
	if m.expanded[track.ID] {
		expansion = "⌄"
	}
	queued := "  "
	for _, variant := range track.Variants {
		if m.queuedID(variant.ID) != "" {
			queued = m.theme.queued.Render("● ")
			break
		}
	}
	left := m.theme.muted.Render(expansion+" ") + queued + m.theme.accent.Render(track.Artist) + m.theme.muted.Render("  —  ") + m.theme.title.Render(track.Title)
	eligibleCount := len(library.EligibleVariants(track, m.currentQuery()))
	right := m.theme.muted.Render(fmt.Sprintf("%s  %d", m.parentRowDate(track), eligibleCount))
	line := joinAligned(left, right, width)
	if selected {
		return m.theme.selected.Width(width).Render(stripStyles(line))
	}
	return line
}

func (m Model) parentRowDate(track library.Track) string {
	modified, video, ok := library.LatestEligibleDates(track, m.currentQuery())
	if !ok {
		return track.ReleaseDateLabel
	}
	switch m.sort {
	case library.ModifiedNewest, library.ModifiedOldest:
		return modified.Format("2006-01-02")
	case library.VideoNewest, library.VideoOldest:
		return video.Format("2006-01-02")
	default:
		return track.ReleaseDateLabel
	}
}

func (m Model) renderFooter(width int) string {
	stats := m.stats.snapshot()
	left := footerHint(m.mode, width)
	right := ""
	if width >= 165 {
		right = fmt.Sprintf("update p95 %.2fms  view p95 %.2fms", milliseconds(stats.updateP95), milliseconds(stats.viewP95))
	}
	status := m.status
	if status == "" {
		status = "Synthetic parity-scale library; no files or players are touched"
	}
	return m.theme.bar.Width(width).Render(joinAligned(left, right, width)) + "\n" + m.theme.muted.Render(truncate(status, width))
}

func (m Model) renderOverlay(base string, width, height int) string {
	var title string
	var lines []string
	switch m.mode {
	case modeCategories:
		title = "Categories"
		items := make([]string, 0, len(library.Categories))
		for index, category := range library.Categories {
			check := "○"
			if m.enabled[category] {
				check = "●"
			}
			prefix := "  "
			if index == m.overlayCursor {
				prefix = "› "
			}
			items = append(items, fmt.Sprintf("%s%s  %s", prefix, check, category))
		}
		lines = append(overlayWindow(items, m.overlayCursor, height), "", "j/k move  •  space/enter toggle  •  c/esc close")
	case modeSort:
		title = "Sort tracks"
		items := make([]string, 0, len(library.Sorts))
		for index, option := range library.Sorts {
			prefix := "  "
			if index == m.overlayCursor {
				prefix = "› "
			}
			selected := " "
			if option == m.sort {
				selected = "●"
			}
			items = append(items, fmt.Sprintf("%s%s  %s", prefix, selected, option))
		}
		lines = append(overlayWindow(items, m.overlayCursor, height), "", "j/k move  •  enter/space apply  •  s/esc close")
	case modeQueue:
		queued, planned := len(m.queueOrder), plannedCount(m.queueOrder, m.queued, m.playbackOptions)
		title = fmt.Sprintf("Queue (%d)", queued)
		if planned != queued {
			title += fmt.Sprintf(" → %d plays", planned)
		}
		if len(m.queueOrder) == 0 {
			lines = []string{"Queue is empty", "", "q/esc close"}
		} else {
			available := min(overlayListCapacity(height), 14)
			start := min(max(m.overlayCursor-available/2, 0), max(len(m.queueOrder)-available, 0))
			end := min(start+available, len(m.queueOrder))
			for index := start; index < end; index++ {
				prefix := "  "
				if index == m.overlayCursor {
					prefix = "› "
				}
				lines = append(lines, fmt.Sprintf("%s%d. %s", prefix, index+1, m.queued[m.queueOrder[index]].Filename))
			}
			lines = append(lines, "", "j/k move  •  shift+j/k reorder  •  delete remove  •  C clear  •  q/esc close")
		}
	case modePlaybackOptions:
		title = "Playback options"
		maximum := "All"
		if m.draftOptions.MaximumItems > 0 || m.optionEditField == 3 && m.optionEdit != "" {
			maximum = m.optionDisplay(3)
		}
		lines = []string{
			fmt.Sprintf("%s Shuffle: %s", cursorMark(m.overlayCursor, 0), onOff(m.draftOptions.Shuffle)),
			fmt.Sprintf("%s One video per track: %s", cursorMark(m.overlayCursor, 1), onOff(m.draftOptions.OneVideoPerTrack)),
			fmt.Sprintf("%s Repeat: %s", cursorMark(m.overlayCursor, 2), m.optionDisplay(2)),
			fmt.Sprintf("%s Maximum: %s", cursorMark(m.overlayCursor, 3), maximum),
			fmt.Sprintf("%s Version choice: %s", cursorMark(m.overlayCursor, 4), m.draftOptions.SelectionStrategy),
			m.plannedPreview(),
		}
		if m.optionError != "" {
			lines = append(lines, m.theme.warning.Render(m.optionError))
		}
		lines = append(lines, "", "j/k move • digits edit • h/l adjust • r reset • enter save • p/esc cancel")
	case modeFilters:
		title = "Filters"
		lines = []string{fmt.Sprintf("%s Track release: %s", cursorMark(m.overlayCursor, 0), emptyAny(m.filterDraft[0])), fmt.Sprintf("%s Video date: %s", cursorMark(m.overlayCursor, 1), emptyAny(m.filterDraft[1])), fmt.Sprintf("%s Apply", cursorMark(m.overlayCursor, 2)), fmt.Sprintf("%s Reset all", cursorMark(m.overlayCursor, 3)), "", "YYYY or START..END • enter apply • f/esc cancel"}
	case modeHelp:
		title = "Keyboard shortcuts"
		all := helpLines()
		visible := overlayListCapacity(height)
		end := min(m.helpOffset+visible, len(all))
		lines = append(lines, all[m.helpOffset:end]...)
		lines = append(lines, "", "j/k scroll • ctrl+u/d page • gg/G ends • ?/esc close")
	case modeDetails:
		title = "Details"
		all := m.detailsLines()
		m.detailsOffset = min(m.detailsOffset, max(len(all)-overlayListCapacity(height), 0))
		end := min(m.detailsOffset+overlayListCapacity(height), len(all))
		lines = append(lines, all[m.detailsOffset:end]...)
		lines = append(lines, "", "j/k scroll • ctrl+u/d page • gg/G ends • d/esc close")
	case modeMappingUpdate:
		title = "Update mappings"
		if m.mappingIgnored {
			title = "Ignored videos"
		}
		if m.mappingScanning {
			lines = []string{"Scanning video and audio folders…", "", "u/esc close"}
			break
		}
		item, ok := m.currentMappingItem()
		if !ok {
			if m.mappingIgnored {
				lines = []string{"No ignored videos", "", "I unmapped list • u/esc close"}
			} else {
				lines = []string{"0 unmapped videos • 0 suggestions", "", "r rescan • I ignored list • u/esc close"}
			}
			break
		}
		if m.mappingIgnored {
			lines = []string{fmt.Sprintf("%d ignored videos • %d of %d", len(m.mappingItems), m.mappingIndex+1, len(m.mappingItems)), "Video: " + item.Filename, "Artist: " + emptyAny(item.Artist), "Title: " + emptyAny(item.Title), "", "i restore • I unmapped list • u/esc close"}
			break
		}
		lines = []string{mappingSummary(m.mappingItems), fmt.Sprintf("%d of %d", m.mappingIndex+1, len(m.mappingItems)), "Video: " + item.Filename, "Artist: " + emptyAny(item.Artist), "Title: " + emptyAny(item.Title)}
		if item.AudioPath == "" {
			lines = append(lines, "No automatic suggestion")
		} else {
			audio := item.AudioArtist + " — " + item.AudioTitle
			if item.AudioArtist == "" && item.AudioTitle == "" {
				audio = "Selected audio track"
			}
			lines = append(lines, "Audio: "+audio, item.Reason)
		}
		lines = append(lines, "", "enter confirm • / choose audio • s skip • i ignore • I ignored list • r rescan • u/esc close")
	case modeMappingPicker:
		title = "Choose audio"
		lines = append(lines, "Search: "+m.mappingQuery)
		for index, candidate := range m.mappingCandidates {
			prefix := "  "
			if index == m.mappingCursor {
				prefix = "› "
			}
			lines = append(lines, prefix+candidate.Artist+" — "+candidate.Title)
		}
		if len(m.mappingCandidates) == 0 {
			lines = append(lines, "No matching audio")
		}
		lines = append(lines, "", "type search • j/k move • enter choose • / or esc cancel")
	}

	overlayWidth := min(max(width*2/3, 28), min(88, width))
	contentWidth := max(overlayWidth-6, 1)
	title = truncate(title, contentWidth)
	for index, line := range lines {
		lines[index] = truncateANSI(line, contentWidth)
	}
	separator := "\n\n"
	if height < 16 {
		separator = "\n"
		lines = slices.DeleteFunc(lines, func(line string) bool { return line == "" })
	}
	content := m.theme.overlayTitle.Render(title) + separator + strings.Join(lines, "\n")
	overlay := m.theme.overlay.Width(overlayWidth).Render(content)
	return placeOverlay(base, overlay, width, height)
}

func overlayWindow(items []string, cursor, height int) []string {
	visible := overlayListCapacity(height)
	start := min(max(cursor-visible/2, 0), max(len(items)-visible, 0))
	end := min(start+visible, len(items))
	return items[start:end]
}

func overlayListCapacity(height int) int {
	if height < 16 {
		return max(height-6, 1)
	}
	return max(height-8, 1)
}

func (m Model) optionDisplay(field int) string {
	if m.optionEditField == field {
		if m.optionEdit == "" {
			return "_"
		}
		return m.optionEdit + "_"
	}
	if field == 2 {
		return strconv.Itoa(m.draftOptions.RepeatEach)
	}
	return strconv.Itoa(m.draftOptions.MaximumItems)
}

func (m Model) plannedPreview() string {
	queued := len(m.queueOrder)
	if queued == 0 {
		return "Queue is empty"
	}
	planned := plannedCount(m.queueOrder, m.queued, m.draftOptions)
	if planned == queued {
		return fmt.Sprintf("%d queued", queued)
	}
	return fmt.Sprintf("%d queued → %d plays", queued, planned)
}

func onOff(value bool) string {
	if value {
		return "On"
	}
	return "Off"
}

func (m Model) detailsLines() []string {
	current, ok := m.currentRow()
	if !ok {
		return []string{"No selected media"}
	}
	track := m.filtered[current.trackIndex]
	if current.isVariant() {
		variant := track.Variants[current.variantIndex]
		return append([]string{"Video: " + variant.Filename, "Video path: " + variant.VideoPath, "Audio path: " + variant.AudioPath, "Category: " + string(variant.Category), "Video date: " + variant.DateLabel, "Modified: " + variant.ModifiedAt.UTC().Format(time.RFC3339), queueState(m.queuedID(variant.ID) != "")}, historyLines(variant.History)...)
	}
	eligible := len(library.EligibleVariants(track, m.currentQuery()))
	lines := []string{"Artist: " + track.Artist, "Title: " + track.Title, "Audio path: " + track.ID, "Release: " + track.ReleaseDateLabel, fmt.Sprintf("Variants: %d total • %d eligible", len(track.Variants), eligible), queueState(m.trackQueued(track))}
	return append(lines, historyLines(track.History)...)
}

func (m Model) trackQueued(track library.Track) bool {
	for _, variant := range track.Variants {
		if m.queuedID(variant.ID) != "" {
			return true
		}
	}
	return false
}
func queueState(queued bool) string {
	if queued {
		return "Queue: queued"
	}
	return "Queue: not queued"
}
func historyLines(value library.History) []string {
	lines := []string{fmt.Sprintf("History: %d played • %d completed • %d stopped • %d skipped", value.PlayedCount, value.CompletedCount, value.StoppedCount, value.SkippedCount), fmt.Sprintf("Recovery: %d not started • %d abandoned", value.NotStartedCount, value.AbandonedCount)}
	if value.LastPlayedAtUTC != nil {
		lines = append(lines, "Last played: "+value.LastPlayedAtUTC.UTC().Format(time.RFC3339))
	}
	if value.LastAttemptedAtUTC != nil {
		lines = append(lines, "Last attempted: "+value.LastAttemptedAtUTC.UTC().Format(time.RFC3339))
	}
	for _, event := range value.Recent {
		detail := event.Outcome + " • " + event.AtUTC.UTC().Format("2006-01-02 15:04")
		if event.Percent != nil {
			detail += fmt.Sprintf(" • %.0f%%", *event.Percent)
		}
		lines = append(lines, detail)
	}
	return lines
}

func cursorMark(cursor, index int) string {
	if cursor == index {
		return "›"
	}
	return " "
}
func emptyAny(value string) string {
	if value == "" {
		return "Any"
	}
	return value
}

func joinAligned(left, right string, width int) string {
	leftWidth := lipgloss.Width(left)
	rightWidth := lipgloss.Width(right)
	if rightWidth >= width-8 {
		return truncate(left, width)
	}
	availableLeft := max(width-rightWidth-2, 1)
	if leftWidth > availableLeft {
		left = truncateANSI(left, availableLeft)
		leftWidth = lipgloss.Width(left)
	}
	return left + strings.Repeat(" ", max(width-leftWidth-rightWidth, 1)) + right
}

func truncate(value string, width int) string {
	if width <= 0 {
		return ""
	}
	return ansi.Truncate(value, width, "…")
}

func truncateANSI(value string, width int) string {
	return ansi.Truncate(value, width, "…")
}

func stripStyles(value string) string {
	return ansi.Strip(value)
}

func padLines(value string, height int) string {
	lineCount := strings.Count(value, "\n") + 1
	if lineCount >= height {
		return value
	}
	return value + strings.Repeat("\n", height-lineCount)
}

func placeOverlay(base, overlay string, width, height int) string {
	baseLines := strings.Split(base, "\n")
	for len(baseLines) < height {
		baseLines = append(baseLines, "")
	}
	overlayLines := strings.Split(overlay, "\n")
	overlayWidth := 0
	for _, line := range overlayLines {
		overlayWidth = max(overlayWidth, lipgloss.Width(line))
	}
	x := max((width-overlayWidth)/2, 0)
	y := max((height-len(overlayLines))/2, 0)
	for index, overlayLine := range overlayLines {
		row := y + index
		if row >= len(baseLines) {
			break
		}
		background := ansi.Truncate(stripStyles(baseLines[row]), width, "")
		background += strings.Repeat(" ", max(width-ansi.StringWidth(background), 0))
		left := ansi.Cut(background, 0, x)
		left += strings.Repeat(" ", max(x-ansi.StringWidth(left), 0))
		right := ansi.Cut(background, x+overlayWidth, width)
		baseLines[row] = left + overlayLine + right
	}
	return strings.Join(baseLines[:height], "\n")
}

func milliseconds(duration time.Duration) float64 {
	return float64(duration) / float64(time.Millisecond)
}
