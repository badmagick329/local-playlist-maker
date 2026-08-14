package ui

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"playlistmaker/charm/internal/library"
)

type mode int

const (
	modeNavigate mode = iota
	modeSearch
	modeCategories
	modeSort
	modeQueue
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
	default:
		return "NAV"
	}
}

type row struct {
	trackIndex   int
	variantIndex int
}

func (r row) isVariant() bool { return r.variantIndex >= 0 }

type PlaybackLauncher interface {
	Launch(videoIDs []string) (int, error)
}

type playbackResultMsg struct {
	count int
	err   error
}

type Model struct {
	all           []library.Track
	filtered      []library.Track
	rows          []row
	expanded      map[string]bool
	queued        map[string]library.Variant
	queueOrder    []string
	enabled       map[library.Category]bool
	query         string
	sort          library.Sort
	mode          mode
	cursor        int
	overlayCursor int
	waitingForG   bool
	width         int
	height        int
	status        string
	theme         theme
	stats         *latencyStats
	playback      PlaybackLauncher
	launching     bool
}

func New(tracks []library.Track, playback ...PlaybackLauncher) Model {
	enabled := make(map[library.Category]bool, len(library.Categories))
	for _, category := range library.Categories {
		enabled[category] = category == library.MusicVideo
	}
	m := Model{
		all:      tracks,
		expanded: make(map[string]bool),
		queued:   make(map[string]library.Variant),
		enabled:  enabled,
		sort:     library.ModifiedNewest,
		mode:     modeNavigate,
		width:    120,
		height:   36,
		theme:    newTheme(),
		stats:    &latencyStats{},
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
		m.queued = make(map[string]library.Variant)
		m.queueOrder = nil
		m.status = fmt.Sprintf("Launched %d video(s)", message.count)
		return m, nil
	case tea.WindowSizeMsg:
		m.width = max(message.Width, 40)
		m.height = max(message.Height, 12)
		m.keepCursorVisible()
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
	if key.String() == "ctrl+enter" && m.mode == modeNavigate {
		return m.launchQueue()
	}
	switch m.mode {
	case modeSearch:
		return m.handleSearchKey(key), nil
	case modeCategories:
		return m.handleCategoryKey(key), nil
	case modeSort:
		return m.handleSortKey(key), nil
	case modeQueue:
		return m.handleQueueKey(key), nil
	default:
		return m.handleNavigationKey(key), nil
	}
}

func (m Model) launchQueue() (tea.Model, tea.Cmd) {
	if m.launching {
		m.status = "Playback launch already in progress"
		return m, nil
	}
	if len(m.queueOrder) == 0 {
		m.status = "Queue is empty"
		return m, nil
	}
	if m.playback == nil {
		m.status = "Playback is unavailable in synthetic spike mode"
		return m, nil
	}
	ids := append([]string(nil), m.queueOrder...)
	m.launching = true
	m.status = fmt.Sprintf("Launching %d queued video(s)…", len(ids))
	return m, func() tea.Msg {
		count, err := m.playback.Launch(ids)
		return playbackResultMsg{count: count, err: err}
	}
}

func (m Model) handleNavigationKey(key tea.KeyPressMsg) Model {
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
	case "/":
		m.mode = modeSearch
		m.status = "Search mode: type freely; Enter or Esc returns to navigation"
	case "c":
		m.mode = modeCategories
		m.overlayCursor = 0
	case "s":
		m.mode = modeSort
		m.overlayCursor = slices.Index(library.Sorts, m.sort)
	case "q":
		m.mode = modeQueue
		m.overlayCursor = min(m.overlayCursor, max(len(m.queueOrder)-1, 0))
	case "esc":
		m.status = "Esc closes modes; Ctrl+Q quits"
	}
	return m
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
	m.filtered = library.FilterAndSort(m.all, m.query, m.enabled, m.sort)
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

func (m *Model) rebuildRows() {
	rows := make([]row, 0, len(m.filtered)+64)
	for trackIndex, track := range m.filtered {
		rows = append(rows, row{trackIndex: trackIndex, variantIndex: -1})
		if !m.expanded[track.ID] {
			continue
		}
		for variantIndex, variant := range track.Variants {
			if m.enabled[variant.Category] {
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
		defaultVariant, ok := library.DefaultVariant(track, m.enabled)
		if !ok {
			return
		}
		variant = defaultVariant
	}
	if _, exists := m.queued[variant.ID]; exists {
		delete(m.queued, variant.ID)
		m.queueOrder = slices.DeleteFunc(m.queueOrder, func(id string) bool { return id == variant.ID })
		m.status = "Removed from queue"
		return
	}
	m.queued[variant.ID] = variant
	m.queueOrder = append(m.queueOrder, variant.ID)
	m.status = "Queued " + variant.Filename
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

	if m.mode == modeCategories || m.mode == modeSort || m.mode == modeQueue {
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
	right := fmt.Sprintf("%d tracks  •  %d queued  •  %s", len(m.filtered), len(m.queueOrder), m.sort)
	return joinAligned(left, m.theme.muted.Render(right), width)
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
		if _, queued := m.queued[variant.ID]; queued {
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
		if _, ok := m.queued[variant.ID]; ok {
			queued = m.theme.queued.Render("● ")
			break
		}
	}
	left := m.theme.muted.Render(expansion+" ") + queued + m.theme.accent.Render(track.Artist) + m.theme.muted.Render("  —  ") + m.theme.title.Render(track.Title)
	eligibleCount := len(library.EligibleVariants(track, m.enabled))
	right := m.theme.muted.Render(fmt.Sprintf("%s  %d", track.ReleaseDateLabel, eligibleCount))
	line := joinAligned(left, right, width)
	if selected {
		return m.theme.selected.Width(width).Render(stripStyles(line))
	}
	return line
}

func (m Model) renderFooter(width int) string {
	stats := m.stats.snapshot()
	left := "j/k move  ctrl+u/d page  gg/G ends  h/l fold  space queue  ctrl+enter play  / search  c/s/q"
	if m.mode == modeSearch {
		left = "type search  space inserts space  ctrl+j/k move  ctrl+u clear  enter/esc nav"
	}
	right := fmt.Sprintf("update p95 %.2fms  view p95 %.2fms", milliseconds(stats.updateP95), milliseconds(stats.viewP95))
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
		for index, category := range library.Categories {
			check := "○"
			if m.enabled[category] {
				check = "●"
			}
			prefix := "  "
			if index == m.overlayCursor {
				prefix = "› "
			}
			lines = append(lines, fmt.Sprintf("%s%s  %s", prefix, check, category))
		}
		lines = append(lines, "", "space/enter toggle  •  c/esc close")
	case modeSort:
		title = "Sort tracks"
		for index, option := range library.Sorts {
			prefix := "  "
			if index == m.overlayCursor {
				prefix = "› "
			}
			selected := " "
			if option == m.sort {
				selected = "●"
			}
			lines = append(lines, fmt.Sprintf("%s%s  %s", prefix, selected, option))
		}
		lines = append(lines, "", "enter apply  •  esc close")
	case modeQueue:
		title = fmt.Sprintf("Queue (%d)", len(m.queueOrder))
		if len(m.queueOrder) == 0 {
			lines = []string{"Queue is empty", "", "q/esc close"}
		} else {
			available := max(min(height-10, 14), 3)
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
	}

	overlayWidth := min(max(width*2/3, 42), 88)
	content := m.theme.overlayTitle.Render(title) + "\n\n" + strings.Join(lines, "\n")
	overlay := m.theme.overlay.Width(overlayWidth).Render(content)
	return placeOverlay(base, overlay, width, height)
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
