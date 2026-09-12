package ui

import (
	"context"
	"fmt"
	"slices"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"playlistmaker/charm/internal/updater"
)

type mappingDebounceMsg struct{ session, revision int }

type mappingSearchMsg struct {
	session int
	items   []updater.Audio
	err     error
}

func (m Model) handleMappingSearch(message mappingSearchMsg) (tea.Model, tea.Cmd) {
	if m.mode != modeMappingPicker || message.session != m.mappingSession {
		return m, nil
	}
	m.mappingLoading = false
	if message.err != nil {
		m.mappingPending = false
		m.status = "Audio search failed: " + message.err.Error()
	} else {
		m.mappingPool = message.items
		command := m.mappingSearchCmd()
		return m, command
	}
	return m, nil
}

func (m Model) handleMappingDebounce(message mappingDebounceMsg) (tea.Model, tea.Cmd) {
	if m.mode != modeMappingPicker || message.session != m.mappingSession || message.revision != m.mappingRevision || m.mappingLoading {
		return m, nil
	}
	m.mappingCandidates, m.mappingCursor = updater.FilterAudio(m.visibleMappingPool(), m.mappingQuery), 0
	m.mappingPending = false
	return m, nil
}

func (m Model) handleMappingPickerKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.mappingSaving {
		return m, nil
	}
	switch key.String() {
	case "esc", "/":
		m.mappingDuplicate = nil
		m.mode = modeMappingUpdate
		if m.mappingRelink {
			m.mode, m.mappingRelink = modeNavigate, false
		}
		return m, nil
	case "down", "ctrl+j":
		m.mappingCursor = min(m.mappingCursor+1, max(len(m.mappingCandidates)-1, 0))
	case "up", "ctrl+k":
		m.mappingCursor = max(m.mappingCursor-1, 0)
	case "ctrl+o":
		m.mappingShowUnused = !m.mappingShowUnused
		command := m.mappingSearchCmd()
		return m, command
	case "ctrl+n":
		if m.mappingDuplicate == nil || m.mappingLoading || m.mappingPending {
			return m, nil
		}
		item, _ := m.currentMappingItem()
		m.mappingSaving = true
		if m.mappingDuplicate.Selection != "" {
			return m, m.mappingConfirmCmd(item.VideoPath, m.mappingDuplicate.Selection, true)
		}
		return m, m.mappingCreateCmd(item.VideoPath, item.Artist, item.Title, true)
	case "ctrl+u":
		m.mappingQuery = ""
		command := m.mappingSearchCmd()
		return m, command
	case "enter":
		if m.mappingLoading || m.mappingPending {
			return m, nil
		}
		item, ok := m.currentMappingItem()
		if !ok || m.mappingCursor >= len(m.mappingCandidates) {
			return m, nil
		}
		if m.mappingRelink || m.mappingDuplicate != nil {
			m.mappingSaving = true
			return m, m.mappingConfirmCmd(item.VideoPath, m.mappingCandidates[m.mappingCursor].Path, false)
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
			command := m.mappingSearchCmd()
			return m, command
		}
	default:
		if key.Text != "" && !key.Mod.Contains(tea.ModCtrl) && !key.Mod.Contains(tea.ModAlt) {
			m.mappingQuery += key.Text
			command := m.mappingSearchCmd()
			return m, command
		}
	}
	return m, nil
}

// Each picker session owns a snapshot; late loads and debounce ticks cannot cross sessions.
func (m Model) openMappingPicker(query string) (tea.Model, tea.Cmd) {
	m.mode, m.mappingQuery, m.mappingCursor = modeMappingPicker, query, 0
	m.mappingShowUnused, m.mappingDuplicate = false, nil
	m.mappingSession++
	m.mappingPool, m.mappingCandidates = nil, nil
	m.mappingLoading, m.mappingPending = true, true
	service, session := m.mappingUpdater, m.mappingSession
	return m, func() tea.Msg {
		items, err := service.Search(context.Background(), "")
		return mappingSearchMsg{session: session, items: items, err: err}
	}
}

func (m *Model) mappingSearchCmd() tea.Cmd {
	m.mappingRevision++
	m.mappingPending = true
	session, revision := m.mappingSession, m.mappingRevision
	return tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg { return mappingDebounceMsg{session, revision} })
}

func (m Model) mappingPickerOverlay(height int) (string, []string) {
	var lines []string
	title := "Choose catalogue track"
	if m.mappingRelink {
		title = "Relink selected video"
		lines = append(lines, "Video: "+m.mappingItems[0].Filename)
	}
	if m.mappingLoading {
		lines = append(lines, "Loading catalogue…")
	}
	if m.mappingPending && !m.mappingLoading {
		lines = append(lines, "Searching…")
	}
	if m.mappingSaving {
		lines = append(lines, "Saving link…")
	}
	if m.mappingDuplicate != nil {
		lines = append(lines, "Matching tracks exist. Choose one or Ctrl+N create separately.")
	}
	unused := "hidden"
	if m.mappingShowUnused {
		unused = "shown"
	}
	lines = append(lines, "Unused tracks: "+unused+" • Ctrl+O toggle")
	lines = append(lines, "Search: "+m.mappingQuery)
	start := max(0, m.mappingCursor-max(1, height-19)+1)
	end := min(len(m.mappingCandidates), start+max(1, height-19))
	for index := start; index < end; index++ {
		candidate := m.mappingCandidates[index]
		if index == start || candidate.Kind != m.mappingCandidates[index-1].Kind {
			lines = append(lines, mappingKindLabel(candidate))
		}
		prefix := "  "
		if index == m.mappingCursor {
			prefix = "› "
		}
		lines = append(lines, prefix+"["+releasePickerLabel(candidate.ReleaseDate)+"] "+candidate.Artist+" — "+candidate.Title+" • "+albumPickerLabel(candidate.Album))
	}
	if len(m.mappingCandidates) > 0 {
		selected := m.mappingCandidates[m.mappingCursor]
		detail := fmt.Sprintf("%d videos", selected.VideoCount)
		if selected.SpotifyOnly {
			detail += " • Spotify-only"
		}
		if selected.Kind == updater.UnlinkedAudio {
			detail = "Creates a catalogue track when linked"
		}
		lines = append(lines, detail, "Source: "+selected.Source)
	}
	if len(m.mappingCandidates) == 0 && !m.mappingLoading && !m.mappingPending {
		lines = append(lines, "No matching track")
	}
	lines = append(lines, "", "type search • ctrl+u clear • arrows move • enter choose • esc cancel")
	return title, lines
}

func releasePickerLabel(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}

func (m Model) visibleMappingPool() []updater.Audio {
	if m.mappingShowUnused {
		return m.mappingPool
	}
	return slices.DeleteFunc(slices.Clone(m.mappingPool), func(a updater.Audio) bool { return a.Kind == updater.UnusedAudio })
}

func mappingKindLabel(a updater.Audio) string {
	switch a.Kind {
	case updater.UnlinkedAudio:
		return "Unlinked local audio"
	case updater.UnusedAudio:
		return "Unused catalogue tracks"
	default:
		return "Catalogue tracks"
	}
}

func albumPickerLabel(value string) string {
	if value == "" {
		return "No album metadata"
	}
	return value
}
