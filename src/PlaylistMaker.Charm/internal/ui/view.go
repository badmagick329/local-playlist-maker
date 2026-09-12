package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"playlistmaker/charm/internal/library"
)

type trackSource int

const (
	trackSourceSpotify trackSource = iota
	trackSourceLocal
	trackSourceVideoOnly
)

func classifyTrackSource(track library.Track) trackSource {
	if track.SpotifyURI != "" {
		return trackSourceSpotify
	}
	if track.LocalAudioPath != "" {
		return trackSourceLocal
	}
	return trackSourceVideoOnly
}

func (m Model) View() tea.View {
	started := time.Now()
	content := m.render()
	m.stats.recordView(time.Since(started))
	view := tea.NewView(content)
	view.AltScreen = true
	view.WindowTitle = "PlaylistMaker"
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

	if m.mode == modeCategories || m.mode == modeSort || m.mode == modeQueue || m.mode == modePlaybackOptions || m.mode == modeFilters || m.mode == modeHelp || m.mode == modeDetails || m.mode == modeMappingUpdate || m.mode == modeMappingPicker || m.mode == modeSpotifyUpdate || m.mode == modeSpotifySearch || m.mode == modeLastFM {
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
		labels = append(labels, "track release "+m.trackDate.Label)
	}
	if m.videoDate != nil {
		labels = append(labels, "video date "+m.videoDate.Label)
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
	countWidth := m.parentRowCountWidth()
	for index := start; index < end; index++ {
		lines = append(lines, m.renderRow(m.rows[index], index == m.cursor, width, countWidth))
	}
	return padLines(strings.Join(lines, "\n"), height)
}

func (m Model) renderRow(current row, selected bool, width, countWidth int) string {
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
	eligibleCount := len(library.EligibleVariants(track, m.currentQuery()))
	right := m.theme.muted.Render(formatParentRowRight(m.parentRowDate(track), eligibleCount, countWidth))
	if selected {
		selectedText := m.theme.selected.Render
		selectedQueued := selectedText("  ")
		if queued != "  " {
			selectedQueued = selectedText("● ")
		}
		selectedLeftPrefix := selectedText(expansion+" ") + selectedQueued + selectedText(" ") + m.selectedSourceBadge(track) + selectedText(" ") + selectedText(track.Artist) + selectedText("  —  ") + selectedText(track.Title)
		selectedRight := selectedText(formatParentRowRight(m.parentRowDate(track), eligibleCount, countWidth))
		return joinAlignedWithSuffixFill(selectedLeftPrefix, "", selectedRight, width, m.theme.selected)
	}
	leftPrefix := m.theme.muted.Render(expansion+" ") + queued + " " + m.sourceBadge(track) + " " + m.theme.accent.Render(track.Artist) + m.theme.muted.Render("  —  ") + m.theme.title.Render(track.Title)
	line := joinAligned(leftPrefix, right, width)
	return line
}

func (m Model) parentRowCountWidth() int {
	width := 1
	for _, track := range m.filtered {
		width = max(width, len(strconv.Itoa(len(library.EligibleVariants(track, m.currentQuery())))))
	}
	return width
}

func formatParentRowRight(date string, count, countWidth int) string {
	return fmt.Sprintf("%s  %*d", date, countWidth, count)
}

func (m Model) sourceBadge(track library.Track) string {
	style := m.theme.videoOnly
	switch classifyTrackSource(track) {
	case trackSourceSpotify:
		style = m.theme.spotify
	case trackSourceLocal:
		style = m.theme.localAudio
	}
	return style.Render("•")
}

func (m Model) selectedSourceBadge(track library.Track) string {
	color := "#4F5868"
	switch classifyTrackSource(track) {
	case trackSourceSpotify:
		color = "#176B3A"
	case trackSourceLocal:
		color = "#245A91"
	}
	return m.theme.selected.Foreground(lipgloss.Color(color)).Render("•")
}

func (m Model) parentRowDate(track library.Track) string {
	modified, video, ok := library.LatestEligibleDates(track, m.currentQuery())
	if !ok {
		return track.ReleaseDateLabel
	}
	switch m.sort {
	case library.ModifiedNewest, library.ModifiedOldest:
		return modified.Format("2006-01-02")
	case library.VideoNewest, library.VideoOldest:
		return video.Format("2006-01-02")
	default:
		return track.ReleaseDateLabel
	}
}

func (m Model) renderFooter(width int) string {
	stats := m.stats.snapshot()
	left := footerHint(m.mode, width)
	right := ""
	if width >= 165 {
		right = fmt.Sprintf("update p95 %.2fms  view p95 %.2fms", milliseconds(stats.updateP95), milliseconds(stats.viewP95))
	}
	status := m.status
	if m.trackingError != "" {
		status = m.trackingError
	}
	broken := 0
	firstBroken := ""
	for _, track := range m.all {
		if track.LocalAudioIssue != "" {
			broken++
			if firstBroken == "" {
				firstBroken = track.Artist + " - " + track.Title
			}
		}
	}
	if broken > 0 {
		status = fmt.Sprintf("WARNING: %d broken local audio link(s): %s. Repair local links. %s", broken, firstBroken, status)
	}
	if status == "" {
		status = "Ready"
	}
	return m.theme.bar.Width(width).Render(joinAligned(left, right, width)) + "\n" + m.theme.muted.Render(truncate(status, width))
}

func milliseconds(duration time.Duration) float64 {
	return float64(duration) / float64(time.Millisecond)
}
