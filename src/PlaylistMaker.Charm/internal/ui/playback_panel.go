package ui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"playlistmaker/charm/internal/backend"
	"playlistmaker/charm/internal/lastfm"
	"playlistmaker/charm/internal/library"
)

var mixNames = []string{"Manual queue", "Familiar songs, unseen performances", "Balanced rotation", "Listening period", "Forgotten favourites", "Current obsessions"}

func (m *Model) openPlaybackPanel() {
	m.mode, m.overlayCursor = modePlaybackOptions, 5
	m.draftOptions = m.playbackOptions
	m.draftMix = m.playbackMix
	m.draftTrackCount = m.mixTrackCount
	if m.draftTrackCount == 0 {
		m.draftTrackCount = 20
	}
	m.periodDraft = m.savedPeriod
	if m.periodDraft[2] == "" {
		m.periodDraft[2] = "20"
	}
	m.draftMethod = m.savedMethod
	m.clearOptionEdit()
}

func (m Model) playbackRows() []int {
	rows := []int{5, 4, 3, 0, 1, 2}
	if m.draftMix == 3 {
		rows = append(rows, 6, 7)
		if strings.TrimSpace(m.periodDraft[1]) != "" {
			rows = append(rows, 8)
		}
		rows = append(rows, 9)
	}
	rows = append(rows, 10)
	if m.draftMix != 0 {
		rows = append(rows, 11)
	}
	return rows
}

func (m Model) handlePlaybackPanelKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := key.String()
	if k == "o" || k == "a" && m.draftMix != 0 {
		if !m.commitPanelEdit() {
			return m, nil
		}
		return m.runPlaybackPanel(k == "a")
	}
	// Closing keeps every change, like the Categories toggles. The drafts only
	// ever hold valid values, so a half-typed number falls back to the last
	// valid one rather than blocking the close.
	if k == "esc" || k == "p" {
		m.savePlaybackPanel()
		m.mode = modeNavigate
		return m, nil
	}
	if k == "j" || k == "down" || k == "k" || k == "up" {
		if !m.commitPanelEdit() {
			return m, nil
		}
		rows := m.playbackRows()
		pos := slices.Index(rows, m.overlayCursor)
		if k == "j" || k == "down" {
			pos = min(pos+1, len(rows)-1)
		} else {
			pos = max(pos-1, 0)
		}
		m.overlayCursor = rows[pos]
		return m, nil
	}
	if k == "enter" {
		if !m.commitPanelEdit() {
			return m, nil
		}
		if m.overlayCursor == 10 || m.overlayCursor == 11 {
			return m.runPlaybackPanel(m.overlayCursor == 11)
		}
		return m, nil
	}
	if k == "r" {
		m.draftOptions = backend.DefaultPlaybackOptions()
		m.draftMix = 0
		m.draftTrackCount = 20
		m.periodDraft = [3]string{"", "", "20"}
		m.draftMethod = lastfm.WeightedRandom
		m.clearOptionEdit()
		m.overlayCursor = 5
		return m, nil
	}
	// Row letters run before the period-field branch: those fields accept only
	// digits, '-' and '.', so a letter is never text there.
	if row, shifted, ok := playbackRowKeyPress(key); ok {
		return m.handlePlaybackRowLetter(row, shifted), nil
	}
	delta := 0
	if k == "left" || k == "h" {
		delta = -1
	}
	if k == "right" || k == "l" || k == "space" {
		delta = 1
	}
	if m.overlayCursor == 5 && delta != 0 {
		m.cycleMix(delta)
		return m, nil
	}
	if m.overlayCursor == 9 && delta != 0 {
		m.draftMethod = m.draftMethod.Next(delta)
		return m, nil
	}
	if m.overlayCursor >= 6 && m.overlayCursor <= 8 {
		field := m.overlayCursor - 6
		if k == "ctrl+u" {
			m.periodDraft[field] = ""
			return m, nil
		}
		if k == "backspace" && len(m.periodDraft[field]) > 0 {
			m.periodDraft[field] = m.periodDraft[field][:len(m.periodDraft[field])-1]
		}
		for _, r := range key.Text {
			if r >= '0' && r <= '9' || field < 2 && (r == '-' || r == '.') {
				m.periodDraft[field] += string(r)
			}
		}
		return m, nil
	}
	if m.lockedPlaybackRow(m.overlayCursor) {
		return m, nil
	}
	if m.overlayCursor == 3 && m.draftMix != 0 {
		if delta != 0 {
			m.draftTrackCount = max(1, m.draftTrackCount+delta)
			m.clearOptionEdit()
			return m, nil
		}
		if k == "backspace" || key.Text >= "0" && key.Text <= "9" {
			if m.optionEditField != 3 {
				m.optionEditField = 3
				m.optionEdit = ""
				if k == "backspace" {
					m.optionEdit = strconv.Itoa(m.draftTrackCount)
				}
			}
			if k == "backspace" {
				if len(m.optionEdit) > 0 {
					m.optionEdit = m.optionEdit[:len(m.optionEdit)-1]
				}
			} else {
				m.optionEdit += key.Text
			}
			m.validateTrackCount()
			return m, nil
		}
	}
	if m.overlayCursor <= 4 {
		if m.overlayCursor < 2 && delta != 0 {
			key = tea.KeyPressMsg{Code: tea.KeySpace}
		}
		return m.handleOptionsKey(key), nil
	}
	return m, nil
}

// playbackRowKeys is the Playback panel's letter for each row ID. Play and Add
// to queue show o and a, the keys that already run them from any row; the
// other letters avoid every key the panel handles (o a r p h j k l).
var playbackRowKeys = map[int]rune{5: 'm', 4: 'v', 3: 'n', 0: 's', 1: 'u', 2: 'x', 6: 'd', 7: 'e', 8: 'c', 9: 'w', 10: 'o', 11: 'a'}

// lockedPlaybackRow reports whether the draft mix fixes a row's value: a mix
// always plays one video per track, and the familiar/unseen mix always picks
// unseen performances.
func (m Model) lockedPlaybackRow(row int) bool {
	return row == 1 && m.draftMix != 0 || row == 4 && m.draftMix == 1
}

// playbackRowKeyPress reports which row a letter key names and whether Shift
// was held. Play and Add to queue are left to their o and a branches, so
// letters that name no row (h, l and the rest) keep their own meaning.
func playbackRowKeyPress(key tea.KeyPressMsg) (row int, shifted bool, ok bool) {
	letter, shifted, ok := letterKeyPress(key)
	if !ok {
		return 0, false, false
	}
	for id, bound := range playbackRowKeys {
		if bound == letter && id != 10 && id != 11 {
			return id, shifted, true
		}
	}
	return 0, false, false
}

// handlePlaybackRowLetter moves to a letter's row and acts on it: choice rows
// cycle (Shift cycles back), switches toggle, and number and period rows only
// take focus so digits edit them. A row without a reverse treats Shift as the
// plain letter, as Relevance does in the Sort view. A letter whose row is
// hidden does nothing; a locked row takes the cursor but keeps its value.
func (m Model) handlePlaybackRowLetter(row int, shifted bool) Model {
	if !slices.Contains(m.playbackRows(), row) {
		return m
	}
	// Leaving a half-typed number commits it, and an invalid one blocks the
	// move, exactly as j/k do.
	if !m.commitPanelEdit() {
		return m
	}
	m.overlayCursor = row
	if m.lockedPlaybackRow(row) {
		return m
	}
	delta := 1
	if shifted {
		delta = -1
	}
	switch row {
	case 5:
		m.cycleMix(delta)
	case 4:
		m.draftOptions.SelectionStrategy = m.draftOptions.SelectionStrategy.Next(delta)
	case 0:
		m.draftOptions.Shuffle = !m.draftOptions.Shuffle
	case 1:
		m.draftOptions.OneVideoPerTrack = !m.draftOptions.OneVideoPerTrack
	case 9:
		m.draftMethod = m.draftMethod.Next(delta)
	}
	return m
}

// cycleMix moves to the next or previous mix. A new mix starts in its own
// order, so Shuffle carried over from the previous mix is dropped.
func (m *Model) cycleMix(delta int) {
	m.draftMix = (m.draftMix + delta + len(mixNames)) % len(mixNames)
	m.draftOptions.Shuffle = false
	m.clearOptionEdit()
}

func (m *Model) validateTrackCount() bool {
	n, err := strconv.Atoi(m.optionEdit)
	if err != nil || n < 1 {
		m.optionError = "Track count must be a positive number"
		return false
	}
	m.draftTrackCount = n
	m.optionError = ""
	return true
}
func (m *Model) commitPanelEdit() bool {
	if m.draftMix != 0 && m.optionEditField == 3 {
		if !m.validateTrackCount() {
			return false
		}
		m.clearOptionEdit()
		return true
	}
	return m.commitOptionEdit()
}
func (m *Model) savePlaybackPanel() {
	m.playbackOptions = m.draftOptions
	m.playbackMix, m.mixTrackCount = m.draftMix, m.draftTrackCount
	m.savedPeriod, m.savedMethod = m.periodDraft, m.draftMethod
}

// launchOptions applies the mix's locks at launch rather than at save, so
// closing the panel on a mix keeps the manual queue's own play-first-N and
// one-video-per-track choices for when the owner switches back.
func (m Model) launchOptions() backend.PlaybackOptions {
	options := m.playbackOptions
	if m.playbackMix != 0 {
		options.MaximumItems = 0
		options.OneVideoPerTrack = false
	}
	return options
}

// A generated mix becomes a concrete queue once. Launching that queue must not
// select versions or apply the mix's track count a second time.
func (m Model) runPlaybackPanel(appendQueue bool) (tea.Model, tea.Cmd) {
	if m.draftMix == 0 {
		m.savePlaybackPanel()
		m.mode = modeNavigate
		return m.launchQueue()
	}
	if m.lastfm == nil {
		m.status = "Listening history is unavailable"
		return m, nil
	}
	request := lastfm.MixRequest{Tracks: m.filtered, Query: m.currentQuery(), Count: m.draftTrackCount, SelectionStrategy: m.draftOptions.SelectionStrategy, Method: m.draftMethod}
	switch m.draftMix {
	case 1:
		request.Preset = lastfm.FamiliarUnseen
	case 2:
		request.Preset = lastfm.BalancedRotation
	case 4:
		request.Preset = lastfm.ForgottenFavourites
	case 5:
		request.Preset = lastfm.CurrentObsessions
	case 3:
		var err error
		request.Primary, err = library.ParseDateRange(strings.TrimSpace(m.periodDraft[0]))
		if err != nil {
			m.status = "Primary period: " + err.Error()
			return m, nil
		}
		request.Secondary, err = library.ParseDateRange(strings.TrimSpace(m.periodDraft[1]))
		if err != nil {
			m.status = "Secondary period: " + err.Error()
			return m, nil
		}
		if request.Secondary != nil {
			request.SecondaryPercent, err = strconv.Atoi(m.periodDraft[2])
			if err != nil || request.SecondaryPercent < 0 || request.SecondaryPercent > 100 {
				m.status = "Secondary percentage must be 0 through 100"
				return m, nil
			}
		}
	}
	if appendQueue {
		request.Action = lastfm.AppendQueue
		request.QueuedTrackIDs = map[string]bool{}
		for _, v := range m.queued {
			request.QueuedTrackIDs[v.TrackID] = true
		}
	}
	result, err := m.lastfm.BuildMix(request)
	if err != nil {
		m.status = "Mix failed: " + err.Error()
		return m, nil
	}
	if result.Created == 0 {
		m.status = "No eligible tracks for this mix and the current filters"
		return m, nil
	}
	m.savePlaybackPanel()
	if !appendQueue {
		m.queueOrder = nil
		m.queued = map[string]library.Variant{}
	}
	for _, v := range result.Variants {
		m.queued[v.ID] = v
		m.queueOrder = append(m.queueOrder, v.ID)
	}
	m.mode = modeNavigate
	if appendQueue {
		m.status = fmt.Sprintf("Added %d of %d requested tracks", result.Created, result.Requested)
		return m, nil
	}
	return m.launchQueue()
}

func (m Model) playbackPanelLines(width, height int) []string {
	order, unique, performance, length := "Queue order", onOff(m.draftOptions.OneVideoPerTrack), m.draftOptions.SelectionStrategy.String(), "All"
	label := "Play first N"
	if m.draftOptions.MaximumItems > 0 {
		length = strconv.Itoa(m.draftOptions.MaximumItems)
	}
	if m.draftMix != 0 {
		order = "Mix order"
		unique = "Always"
		label = "Track count"
		length = strconv.Itoa(m.draftTrackCount)
	}
	if m.draftMix == 1 {
		performance = "Unseen only"
	}
	if m.draftOptions.Shuffle {
		order = "Shuffle"
	}
	if m.optionEditField == 3 {
		length = m.optionEdit
	}
	labels := map[int]string{5: "Mix: " + mixNames[m.draftMix], 4: "Performances: " + performance, 3: label + ": " + length, 0: "Order: " + order, 1: "One video per track: " + unique, 2: "Repeat each: " + m.optionDisplay(2), 6: "Primary period: " + emptyAny(m.periodDraft[0]), 7: "Secondary period: " + emptyAny(m.periodDraft[1]), 8: "Secondary percentage: " + m.periodDraft[2], 9: "Selection: " + m.draftMethod.String(), 10: "Play", 11: "Add to queue"}
	rows := m.playbackRows()
	lines := []string{}
	for _, id := range rows {
		lines = append(lines, fmt.Sprintf("%s %c %s", cursorMark(m.overlayCursor, id), playbackRowKeys[id], labels[id]))
	}
	lines = overlayWindow(lines, slices.Index(rows, m.overlayCursor), height-3)
	summary := m.plannedPreview()
	if m.draftMix != 0 {
		summary = fmt.Sprintf("Up to %d tracks · %d plays", m.draftTrackCount, saturatingMultiply(m.draftTrackCount, m.draftOptions.RepeatEach))
	}
	lines = append(lines, "", summary, "Current search, categories and date filters apply")
	if m.draftMix == 1 {
		lines = append(lines, "3+ Last.fm plays; no counted local video play")
	}
	if m.draftMix == 2 {
		lines = append(lines, "40% recent · 40% older favourites · 20% rare")
	}
	if m.draftMix == 4 {
		lines = append(lines, "10+ plays across 5+ days; unheard for 6 months", "Local attempts rest for 30 days, including skips")
	}
	if m.draftMix == 5 {
		date := "none"
		if m.lastfmStatus.LastPlayedAtUTC != nil {
			date = m.lastfmStatus.LastPlayedAtUTC.UTC().Format("2006-01-02")
		}
		lines = append(lines, "History through: "+date+" UTC", "Growth: latest 14 days versus preceding 30 days")
	}
	if m.optionError != "" {
		lines = append(lines, m.optionError)
	}
	return append(lines, playbackPanelFooter(max(playbackPanelBoxWidth(width)-6, 1)))
}

// playbackPanelFooter fits whole for the same reason as sortFooter: the rows
// show each letter but not that Shift cycles back.
func playbackPanelFooter(contentWidth int) string {
	return fittedFooter(contentWidth,
		"o play · a add mix · letter row · Shift reverse · h/l change · p/Esc close",
		"letter row · Shift reverse · h/l change · p/Esc close",
		"letter row · Shift back · p close",
	)
}
