package ui

import (
	"fmt"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"playlistmaker/charm/internal/config"
	"playlistmaker/charm/internal/library"
)

func (m Model) WithCategoryPresets(presets []config.CategoryPreset) Model {
	m.categoryPresets = append([]config.CategoryPreset(nil), presets...)
	return m
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
	presetCount := len(m.categoryPresets)
	categoryCount := len(library.Categories)
	total := presetCount + categoryCount
	if key.Text >= "0" && key.Text <= "4" {
		index := int(key.Text[0] - '0')
		if index < presetCount {
			m.applyCategoryPreset(index)
		}
		return m
	}
	switch key.String() {
	case "j", "down", "ctrl+j":
		m.overlayCursor = min(m.overlayCursor+1, total-1)
	case "k", "up", "ctrl+k":
		m.overlayCursor = max(m.overlayCursor-1, 0)
	case "space", "enter":
		if m.overlayCursor < presetCount {
			m.applyCategoryPreset(m.overlayCursor)
			return m
		}
		category := library.Categories[m.overlayCursor-presetCount]
		m.enabled[category] = !m.enabled[category]
		m.refreshResults()
	case "c", "esc":
		m.mode = modeNavigate
	}
	return m
}

func (m *Model) applyCategoryPreset(index int) {
	if index < 0 || index >= len(m.categoryPresets) {
		return
	}
	preset := m.categoryPresets[index]
	m.enabled = presetCategories(preset)
	m.refreshResults()
	m.mode = modeNavigate
	m.status = "Category preset: " + preset.Name
}

func presetCategories(preset config.CategoryPreset) map[library.Category]bool {
	enabled := make(map[library.Category]bool, len(library.Categories))
	if len(preset.Include) > 0 {
		for _, category := range preset.Include {
			enabled[category] = true
		}
	} else {
		for _, category := range library.Categories {
			enabled[category] = true
		}
		for _, category := range preset.Exclude {
			delete(enabled, category)
		}
	}
	return enabled
}

func (m Model) categoryPresetActive(preset config.CategoryPreset) bool {
	target := presetCategories(preset)
	for _, category := range library.Categories {
		if m.enabled[category] != target[category] {
			return false
		}
	}
	return true
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

func (m Model) categoriesOverlay(width int, height int) (string, []string) {
	var lines []string
	title := "Categories"
	items := make([]string, 0, len(m.categoryPresets)+len(library.Categories))
	for index, preset := range m.categoryPresets {
		prefix := "  "
		if index == m.overlayCursor {
			prefix = "› "
		}
		marker := " "
		if m.categoryPresetActive(preset) {
			marker = "●"
		}
		items = append(items, fmt.Sprintf("%s%s %d  %s", prefix, marker, index, preset.Name))
	}
	for index, category := range library.Categories {
		check := "○"
		if m.enabled[category] {
			check = "●"
		}
		prefix := "  "
		if index+len(m.categoryPresets) == m.overlayCursor {
			prefix = "› "
		}
		items = append(items, fmt.Sprintf("%s%s  %s", prefix, check, category))
	}
	footer := "j/k move  •  space/enter toggle  •  c/esc close"
	if len(m.categoryPresets) > 0 {
		footer = "j/k move  •  0-4 preset  •  space/enter apply/toggle  •  c/esc close"
		if width < 60 {
			footer = "0-4 preset  •  c close"
		}
	}
	lines = append(overlayWindow(items, m.overlayCursor, height), "", footer)
	return title, lines
}

func (m Model) sortOverlay(height int) (string, []string) {
	var lines []string
	title := "Sort tracks"
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
	return title, lines
}

func (m Model) filtersOverlay() (string, []string) {
	var lines []string
	title := "Filters"
	lines = []string{fmt.Sprintf("%s Track release: %s", cursorMark(m.overlayCursor, 0), emptyAny(m.filterDraft[0])), fmt.Sprintf("%s Video date: %s", cursorMark(m.overlayCursor, 1), emptyAny(m.filterDraft[1])), fmt.Sprintf("%s Apply", cursorMark(m.overlayCursor, 2)), fmt.Sprintf("%s Reset all", cursorMark(m.overlayCursor, 3)), "", "YYYY or START..END • enter apply • f/esc cancel"}
	return title, lines
}
