package ui

import (
	"context"
	"slices"

	tea "charm.land/bubbletea/v2"

	"playlistmaker/charm/internal/library"
)

// Both mapping workflows reload the same library and must reconcile queues identically.
type libraryReloader interface {
	Reload(context.Context) ([]library.Track, PlaybackLauncher, error)
}

func reloadLibraryCmd(source libraryReloader) tea.Cmd {
	return func() tea.Msg {
		tracks, playback, err := source.Reload(context.Background())
		return libraryReloadMsg{tracks: tracks, playback: playback, err: err}
	}
}

type row struct {
	trackIndex   int
	variantIndex int
}

func (r row) isVariant() bool { return r.variantIndex >= 0 }

type libraryReloadMsg struct {
	tracks   []library.Track
	playback PlaybackLauncher
	err      error
}

func (m Model) handleLibraryReload(message libraryReloadMsg) (tea.Model, tea.Cmd) {
	if message.err != nil {
		m.status = "Library reload failed: " + message.err.Error()
		return m, nil
	}
	m.all, m.playback = message.tracks, message.playback
	valid := m.variantIndex()
	m.queueOrder = slices.DeleteFunc(m.queueOrder, func(id string) bool { _, ok := valid[id]; return !ok })
	for id := range m.queued {
		if variant, ok := valid[id]; ok {
			m.queued[id] = variant
		} else {
			delete(m.queued, id)
		}
	}
	m.refreshResults()
	m.status = "Library reloaded"
	return m, nil
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
