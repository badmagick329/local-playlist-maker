package ui

import (
	"fmt"
	"slices"

	tea "charm.land/bubbletea/v2"

	"playlistmaker/charm/internal/library"
	"playlistmaker/charm/internal/pathid"
)

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

func (m *Model) queueAllFilteredVariants() {
	if len(m.filtered) == 0 {
		m.status = "No matching tracks"
		return
	}

	added, skipped := 0, 0
	for _, track := range m.filtered {
		for _, variant := range library.EligibleVariants(track, m.currentQuery()) {
			if m.queuedID(variant.ID) != "" {
				skipped++
				continue
			}
			m.queued[variant.ID] = variant
			m.queueOrder = append(m.queueOrder, variant.ID)
			added++
		}
	}
	m.status = fmt.Sprintf("Queued %d videos from all filtered tracks; %d already queued", added, skipped)
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

func (m Model) queueOverlay(height int) (string, []string) {
	var lines []string
	queued, planned := len(m.queueOrder), plannedCount(m.queueOrder, m.queued, m.playbackOptions)
	title := fmt.Sprintf("Queue (%d)", queued)
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
	return title, lines
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
