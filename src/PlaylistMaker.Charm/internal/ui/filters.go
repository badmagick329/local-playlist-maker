package ui

import (
	"fmt"
	"slices"
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

// letterKeyPress reports the lowercase letter a key types and whether Shift
// was held, for the overlays' row letters. Bubble Tea's legacy and Windows
// console decoders deliver Shift+letter as the uppercase letter in Text, and
// String() returns that Text ("X", not "shift+x"), so the case of Text is the
// test. Caps Lock also uppercases Text, so with it on a letter reads as
// Shift+letter. Ctrl and Alt chords are never row letters.
func letterKeyPress(key tea.KeyPressMsg) (letter rune, shifted bool, ok bool) {
	if key.Mod.Contains(tea.ModCtrl) || key.Mod.Contains(tea.ModAlt) || utf8.RuneCountInString(key.Text) != 1 {
		return 0, false, false
	}
	pressed, _ := utf8.DecodeRuneInString(key.Text)
	if !unicode.IsLetter(pressed) {
		return 0, false, false
	}
	letter = unicode.ToLower(pressed)
	return letter, pressed != letter, true
}

// categoryKeyPress reports which category a letter key names and whether
// Shift was held, which solos it.
func categoryKeyPress(key tea.KeyPressMsg) (category library.Category, solo bool, ok bool) {
	letter, shifted, ok := letterKeyPress(key)
	if !ok {
		return "", false, false
	}
	for category, bound := range categoryKeys {
		if bound == letter {
			return category, shifted, true
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

// sortField is one row of the Sort view: a field the library sorts by, in its
// default direction and its reverse.
type sortField struct {
	key     rune
	name    string
	forward library.Sort
	reverse library.Sort
}

// sortFields lists the Sort view's rows, pairing the eleven library.Sorts into
// six fields; a row's letter applies the default direction and Shift+letter
// the reverse. Relevance has no reverse, so both directions are Relevance.
// The letters avoid every key the view already handles (j/k move, s closes).
var sortFields = []sortField{
	{'m', "Modified", library.ModifiedNewest, library.ModifiedOldest},
	{'a', "Artist", library.ArtistAscending, library.ArtistDescending},
	{'t', "Title", library.TitleAscending, library.TitleDescending},
	{'r', "Track release", library.ReleaseNewest, library.ReleaseOldest},
	{'v', "Video date", library.VideoNewest, library.VideoOldest},
	{'e', "Relevance", library.Relevance, library.Relevance},
}

// sortFieldIndex is the row of the field that sort belongs to.
func sortFieldIndex(sort library.Sort) int {
	return slices.IndexFunc(sortFields, func(field sortField) bool { return field.holds(sort) })
}

func (field sortField) holds(sort library.Sort) bool {
	return sort == field.forward || sort == field.reverse
}

func (m Model) handleSortKey(key tea.KeyPressMsg) Model {
	switch key.String() {
	case "j", "down", "ctrl+j":
		m.overlayCursor = min(m.overlayCursor+1, len(sortFields)-1)
		return m
	case "k", "up", "ctrl+k":
		m.overlayCursor = max(m.overlayCursor-1, 0)
		return m
	case "enter", "space":
		// Enter on the field already sorted its default way flips it, so the
		// reverse is reachable without Shift.
		field := sortFields[m.overlayCursor]
		if m.sort == field.forward {
			return m.applySort(field.reverse)
		}
		return m.applySort(field.forward)
	case "esc", "s":
		m.mode = modeNavigate
		return m
	}
	if letter, shifted, ok := letterKeyPress(key); ok {
		for _, field := range sortFields {
			if field.key != letter {
				continue
			}
			if shifted {
				return m.applySort(field.reverse)
			}
			return m.applySort(field.forward)
		}
	}
	return m
}

func (m Model) applySort(sort library.Sort) Model {
	m.sort = sort
	m.refreshResults()
	m.cursor = 0
	m.mode = modeNavigate
	m.status = "Sorted by " + m.sort.String()
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
	return fittedFooter(contentWidth,
		"j/k move  •  space/letter toggle  •  Shift+letter solo  •  c/esc close",
		"letter toggle • Shift+letter solo • c/esc close",
		"key toggle  Shift solo",
	)
}

// fittedFooter returns the first footer, longest first, that fits contentWidth
// whole, falling back to the last.
func fittedFooter(contentWidth int, footers ...string) string {
	for _, footer := range footers[:len(footers)-1] {
		if utf8.RuneCountInString(footer) <= contentWidth {
			return footer
		}
	}
	return footers[len(footers)-1]
}

func (m Model) sortOverlay(width int, height int) (string, []string) {
	var lines []string
	title := "Sort tracks"
	items := make([]string, 0, len(sortFields))
	for index, field := range sortFields {
		prefix := "  "
		if index == m.overlayCursor {
			prefix = "› "
		}
		// The active field shows the direction in use; the others show what
		// their letter would apply.
		selected, shown := " ", field.forward
		if field.holds(m.sort) {
			selected, shown = "●", m.sort
		}
		items = append(items, fmt.Sprintf("%s%s %c  %s", prefix, selected, field.key, shown))
	}
	lines = append(overlayWindow(items, m.overlayCursor, height), "", sortFooter(overlayContentWidth(width)))
	return title, lines
}

// sortFooter fits whole for the same reason as categoriesFooter: the rows do
// not show that Shift reverses.
func sortFooter(contentWidth int) string {
	return fittedFooter(contentWidth,
		"j/k move  •  letter sort  •  Shift+letter reverse  •  s/esc close",
		"letter sort • Shift reverse • s/esc close",
		"key sort  Shift flips",
	)
}

func (m Model) filtersOverlay() (string, []string) {
	var lines []string
	title := "Filters"
	lines = []string{fmt.Sprintf("%s Track release: %s", cursorMark(m.overlayCursor, 0), emptyAny(m.filterDraft[0])), fmt.Sprintf("%s Video date: %s", cursorMark(m.overlayCursor, 1), emptyAny(m.filterDraft[1])), fmt.Sprintf("%s Apply", cursorMark(m.overlayCursor, 2)), fmt.Sprintf("%s Reset all", cursorMark(m.overlayCursor, 3)), "", "YYYY or START..END • enter apply • f/esc cancel"}
	return title, lines
}
