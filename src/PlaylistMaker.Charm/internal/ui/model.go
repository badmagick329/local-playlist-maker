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

type playbackResultMsg struct {
	count      int
	err        error
	queued     bool
	queueOrder []string
}

type Model struct {
	all             []library.Track
	filtered        []library.Track
	rows            []row
	expanded        map[string]bool
	queued          map[string]library.Variant
	queueOrder      []string
	enabled         map[library.Category]bool
	query           string
	sort            library.Sort
	trackDate       *library.DateRange
	videoDate       *library.DateRange
	mode            mode
	cursor          int
	overlayCursor   int
	waitingForG     bool
	width           int
	height          int
	status          string
	theme           theme
	stats           *latencyStats
	playback        PlaybackLauncher
	playbackOptions backend.PlaybackOptions
	draftOptions    backend.PlaybackOptions
	filterDraft     [2]string
	optionEdit      string
	optionEditField int
	optionError     string
	helpOffset      int
	detailsOffset   int
	launching       bool
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

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	started := time.Now()
	defer func() { m.stats.recordUpdate(time.Since(started)) }()

	switch message := message.(type) {
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
		m.helpOffset = min(m.helpOffset, m.helpMaxOffset())
		return m, nil
	case tea.KeyPressMsg:
		if message.String() == "ctrl+q" {
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
		variant, found = library.DefaultVariant(track, m.currentQuery())
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

func (m Model) handleNavigationKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.String() != "g" {
		m.waitingForG = false
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
	case "a":
		m.queueAll(false)
	case "A":
		m.queueAll(true)
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
	case "esc":
		m.status = "Esc closes modes; Ctrl+Q quits"
	}
	return m, nil
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

func (m Model) handleOptionsKey(key tea.KeyPressMsg) Model {
	switch key.String() {
	case "esc":
		m.mode = modeNavigate
		return m
	case "j", "down", "ctrl+j":
		if !m.commitOptionEdit() {
			return m
		}
		m.overlayCursor = min(m.overlayCursor+1, 3)
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
		return m
	case "h", "left":
		m.clearOptionEdit()
		if m.overlayCursor == 2 {
			m.draftOptions.RepeatEach = max(1, m.draftOptions.RepeatEach-1)
		} else if m.overlayCursor == 3 {
			m.draftOptions.MaximumItems = max(0, m.draftOptions.MaximumItems-1)
		}
		return m
	case "l", "right":
		m.clearOptionEdit()
		if m.overlayCursor == 2 {
			m.draftOptions.RepeatEach = min(10, m.draftOptions.RepeatEach+1)
		} else if m.overlayCursor == 3 {
			m.draftOptions.MaximumItems++
		}
		return m
	case "backspace":
		if m.overlayCursor >= 2 {
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
		if m.overlayCursor >= 2 {
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
	case "esc":
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
			m.status = "Track date: " + err.Error()
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

func (m *Model) queueAll(allVariants bool) {
	added, skipped := 0, 0
	for _, track := range m.filtered {
		variants := library.EligibleVariants(track, m.currentQuery())
		if !allVariants {
			if selected, ok := library.DefaultVariant(track, m.currentQuery()); ok {
				variants = []library.Variant{selected}
			}
		}
		for _, variant := range variants {
			if m.queuedID(variant.ID) != "" {
				skipped++
				continue
			}
			m.queued[variant.ID] = variant
			m.queueOrder = append(m.queueOrder, variant.ID)
			added++
		}
	}
	if len(m.filtered) == 0 {
		m.status = "No matching tracks"
	} else if skipped > 0 {
		m.status = fmt.Sprintf("Queued %d; %d already queued", added, skipped)
	} else {
		m.status = fmt.Sprintf("Queued %d video(s)", added)
	}
}

func (m Model) handleSearchKey(key tea.KeyPressMsg) Model {
	switch key.String() {
	case "esc", "enter":
		m.mode = modeNavigate
		m.status = "Search retained"
	case "ctrl+j", "down":
		m.moveCursor(1)
	case "ctrl+k", "up":
		m.moveCursor(-1)
	case "ctrl+u":
		m.query = ""
		m.refreshResults()
	case "backspace":
		if m.query != "" {
			_, size := utf8.DecodeLastRuneInString(m.query)
			m.query = m.query[:len(m.query)-size]
			m.refreshResults()
		}
	case "space":
		m.query += " "
		m.refreshResults()
	default:
		if key.Text != "" && !key.Mod.Contains(tea.ModCtrl) && !key.Mod.Contains(tea.ModAlt) {
			m.query += key.Text
			m.refreshResults()
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
	if current, ok := m.currentRow(); ok {
		selectedTrackID = m.filtered[current.trackIndex].ID
	}
	m.filtered = library.FilterAndSort(m.all, m.currentQuery())
	m.rebuildRows()
	if selectedTrackID != "" {
		for index, current := range m.rows {
			if m.filtered[current.trackIndex].ID == selectedTrackID {
				m.cursor = index
				break
			}
		}
	}
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
		for variantIndex, variant := range track.Variants {
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
		defaultVariant, ok := library.DefaultVariant(track, m.currentQuery())
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

	if m.mode == modeCategories || m.mode == modeSort || m.mode == modeQueue || m.mode == modePlaybackOptions || m.mode == modeFilters || m.mode == modeHelp || m.mode == modeDetails {
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
		labels = append(labels, "track "+m.trackDate.Label)
	}
	if m.videoDate != nil {
		labels = append(labels, "video "+m.videoDate.Label)
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
	right := m.theme.muted.Render(fmt.Sprintf("%s  %d", track.ReleaseDateLabel, eligibleCount))
	line := joinAligned(left, right, width)
	if selected {
		return m.theme.selected.Width(width).Render(stripStyles(line))
	}
	return line
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
			lines = append(lines, "", "j/k move  •  shift+j/k reorder  •  delete remove  •  q/esc close")
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
			m.plannedPreview(),
		}
		if m.optionError != "" {
			lines = append(lines, m.theme.warning.Render(m.optionError))
		}
		lines = append(lines, "", "j/k move • digits edit • h/l adjust • r reset • enter save • esc cancel")
	case modeFilters:
		title = "Filters"
		lines = []string{fmt.Sprintf("%s Track: %s", cursorMark(m.overlayCursor, 0), emptyAny(m.filterDraft[0])), fmt.Sprintf("%s Video: %s", cursorMark(m.overlayCursor, 1), emptyAny(m.filterDraft[1])), fmt.Sprintf("%s Apply", cursorMark(m.overlayCursor, 2)), fmt.Sprintf("%s Reset all", cursorMark(m.overlayCursor, 3)), "", "YYYY or START..END • enter apply • esc cancel"}
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
		plain := []rune(stripStyles(baseLines[row]))
		for len(plain) < width {
			plain = append(plain, ' ')
		}
		left := string(plain[:min(x, len(plain))])
		rightStart := min(x+overlayWidth, len(plain))
		right := string(plain[rightStart:])
		baseLines[row] = left + overlayLine + right
	}
	return strings.Join(baseLines[:height], "\n")
}

func milliseconds(duration time.Duration) float64 {
	return float64(duration) / float64(time.Millisecond)
}
