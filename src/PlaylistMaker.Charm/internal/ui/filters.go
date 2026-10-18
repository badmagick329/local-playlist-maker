package ui

import (
	"fmt"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"playlistmaker/charm/internal/library"
)

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
	// A reset starts fresh, so Shift+m on the reset Music Video set must not
	// resurrect a set from before it.
	m.preSoloCategories = nil
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

// categoryKeys is the Categories view's letter for each category; the letter
// toggles it and Shift+letter shows only it. The letters avoid every key the
// view already handles (j/k move, c closes), so they need no mode switch.
var categoryKeys = map[library.Category]rune{
	library.MusicVideo:   'm',
	library.BandLive:     'b',
	library.Performance:  'p',
	library.Choreography: 'h',
	library.Relay:        'r',
	library.BeOriginal:   'o',
	library.Fancam:       'f',
	library.Concert:      'n',
	library.MusicShow:    's',
	library.Remix:        'x',
	library.LiveAudio:    'a',
}

// categoryKeyPress reports which category a letter key names and whether
// Shift was held. Bubble Tea's legacy and Windows console decoders deliver
// Shift+letter as the uppercase letter in Text, and String() returns that Text
// ("X", not "shift+x"), so the case of Text is the test. Caps Lock also
// uppercases Text, so with it on a letter solos.
func categoryKeyPress(key tea.KeyPressMsg) (category library.Category, solo bool, ok bool) {
	if key.Mod.Contains(tea.ModCtrl) || key.Mod.Contains(tea.ModAlt) || utf8.RuneCountInString(key.Text) != 1 {
		return "", false, false
	}
	pressed, _ := utf8.DecodeRuneInString(key.Text)
	letter := unicode.ToLower(pressed)
	for category, bound := range categoryKeys {
		if bound == letter {
			return category, pressed != letter, true
		}
	}
	return "", false, false
}

func (m Model) handleCategoryKey(key tea.KeyPressMsg) Model {
	switch key.String() {
	case "j", "down", "ctrl+j":
		m.overlayCursor = min(m.overlayCursor+1, len(library.Categories)-1)
		return m
	case "k", "up", "ctrl+k":
		m.overlayCursor = max(m.overlayCursor-1, 0)
		return m
	case "space", "enter":
		m.toggleCategory(library.Categories[m.overlayCursor])
		return m
	case "c", "esc":
		m.mode = modeNavigate
		return m
	}
	if category, solo, ok := categoryKeyPress(key); ok {
		if solo {
			m.soloCategory(category)
		} else {
			m.toggleCategory(category)
		}
	}
	return m
}

func (m *Model) toggleCategory(category library.Category) {
	m.enabled[category] = !m.enabled[category]
	m.refreshResults()
}

// soloCategory shows only one category, remembering the set it replaced so a
// second solo of the same category can bring that set back. Each new solo
// overwrites the memory, so restore always returns to the set active just
// before the latest solo.
func (m *Model) soloCategory(category library.Category) {
	if m.onlyCategoryEnabled(category) {
		if m.preSoloCategories == nil {
			return
		}
		m.enabled, m.preSoloCategories = m.preSoloCategories, nil
		m.refreshResults()
		return
	}
	// A fresh map, not in-place edits: the remembered set must not alias the
	// live one that later toggles mutate.
	m.preSoloCategories = m.enabled
	m.enabled = make(map[library.Category]bool, len(library.Categories))
	for _, each := range library.Categories {
		m.enabled[each] = each == category
	}
	m.refreshResults()
}

func (m Model) onlyCategoryEnabled(category library.Category) bool {
	for _, each := range library.Categories {
		if m.enabled[each] != (each == category) {
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
		items = append(items, fmt.Sprintf("%s%s %c  %s", prefix, check, categoryKeys[category], category))
	}
	lines = append(overlayWindow(items, m.overlayCursor, height), "", categoriesFooter(overlayContentWidth(width)))
	return title, lines
}

// categoriesFooter picks the longest hint that fits whole, because a truncated
// hint would cut off the Shift-solo part that the rows themselves do not show.
func categoriesFooter(contentWidth int) string {
	footers := []string{
		"j/k move  •  space/letter toggle  •  Shift+letter solo  •  c/esc close",
		"letter toggle • Shift+letter solo • c/esc close",
		"key toggle  Shift solo",
	}
	for _, footer := range footers[:len(footers)-1] {
		if utf8.RuneCountInString(footer) <= contentWidth {
			return footer
		}
	}
	return footers[len(footers)-1]
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
