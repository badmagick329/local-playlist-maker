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

var mixNames = []string{"Manual queue", "Familiar songs, unseen performances", "Balanced rotation", "Listening period"}

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
	return append(rows, 12)
}

func (m Model) handlePlaybackPanelKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := key.String()
	if k == "o" || k == "a" && m.draftMix != 0 {
		if !m.commitPanelEdit() {
			return m, nil
		}
		return m.runPlaybackPanel(k == "a")
	}
	if k == "esc" || k == "p" {
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
		if m.overlayCursor == 12 {
			m.savePlaybackPanel()
			m.mode = modeNavigate
			m.status = "Playback settings saved"
			return m, nil
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
	delta := 0
	if k == "left" || k == "h" {
		delta = -1
	}
	if k == "right" || k == "l" || k == "space" {
		delta = 1
	}
	if m.overlayCursor == 5 && delta != 0 {
		m.draftMix = (m.draftMix + delta + len(mixNames)) % len(mixNames)
		m.draftOptions.Shuffle = false
		m.clearOptionEdit()
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
	if m.overlayCursor == 1 && m.draftMix != 0 || m.overlayCursor == 4 && m.draftMix == 1 {
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
	if m.draftMix != 0 {
		m.playbackOptions.MaximumItems = 0
		m.playbackOptions.OneVideoPerTrack = false
	}
	m.playbackMix, m.mixTrackCount = m.draftMix, m.draftTrackCount
	m.savedPeriod, m.savedMethod = m.periodDraft, m.draftMethod
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

func (m Model) playbackPanelLines(height int) []string {
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
	labels := map[int]string{5: "Mix: " + mixNames[m.draftMix], 4: "Performances: " + performance, 3: label + ": " + length, 0: "Order: " + order, 1: "One video per track: " + unique, 2: "Repeat each: " + m.optionDisplay(2), 6: "Primary period: " + emptyAny(m.periodDraft[0]), 7: "Secondary period: " + emptyAny(m.periodDraft[1]), 8: "Secondary percentage: " + m.periodDraft[2], 9: "Selection: " + m.draftMethod.String(), 10: "Play", 11: "Add to queue", 12: "Save settings"}
	rows := m.playbackRows()
	lines := []string{}
	for _, id := range rows {
		lines = append(lines, cursorMark(m.overlayCursor, id)+" "+labels[id])
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
	if m.optionError != "" {
		lines = append(lines, m.optionError)
	}
	return append(lines, "o play · a add mix · j/k move · h/l change · Esc cancel")
}
