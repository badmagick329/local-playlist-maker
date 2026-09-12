package ui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"playlistmaker/charm/internal/library"
)

func (m Model) handleDetailsKey(key tea.KeyPressMsg) Model {
	maxOffset := max(len(m.detailsLines())-overlayListCapacity(m.height), 0)
	switch key.String() {
	case "esc", "d":
		m.mode = modeNavigate
	case "j", "down", "ctrl+j":
		m.detailsOffset = min(m.detailsOffset+1, maxOffset)
	case "k", "up", "ctrl+k":
		m.detailsOffset = max(m.detailsOffset-1, 0)
	case "ctrl+d", "pgdown":
		m.detailsOffset = min(m.detailsOffset+overlayListCapacity(m.height), maxOffset)
	case "ctrl+u", "pgup":
		m.detailsOffset = max(m.detailsOffset-overlayListCapacity(m.height), 0)
	case "g":
		m.detailsOffset = 0
	case "G":
		m.detailsOffset = maxOffset
	}
	return m
}

func (m Model) handleHelpKey(key tea.KeyPressMsg) Model {
	maxOffset := m.helpMaxOffset()
	switch key.String() {
	case "esc", "?":
		m.mode = modeNavigate
	case "j", "down", "ctrl+j":
		m.helpOffset = min(m.helpOffset+1, maxOffset)
	case "k", "up", "ctrl+k":
		m.helpOffset = max(m.helpOffset-1, 0)
	case "ctrl+d", "pgdown":
		m.helpOffset = min(m.helpOffset+max(m.height-8, 1), maxOffset)
	case "ctrl+u", "pgup":
		m.helpOffset = max(m.helpOffset-max(m.height-8, 1), 0)
	case "g":
		m.helpOffset = 0
	case "G":
		m.helpOffset = maxOffset
	}
	return m
}

func (m Model) helpMaxOffset() int {
	return max(len(helpLines())-overlayListCapacity(m.height), 0)
}

func (m *Model) clampOverlayState() {
	m.helpOffset = min(max(m.helpOffset, 0), m.helpMaxOffset())
	switch m.mode {
	case modeCategories:
		m.overlayCursor = min(max(m.overlayCursor, 0), max(len(m.categoryPresets)+len(library.Categories)-1, 0))
	case modeSort:
		m.overlayCursor = min(max(m.overlayCursor, 0), len(library.Sorts)-1)
	case modeQueue:
		m.overlayCursor = min(max(m.overlayCursor, 0), max(len(m.queueOrder)-1, 0))
	case modePlaybackOptions:
		if !slices.Contains(m.playbackRows(), m.overlayCursor) {
			m.overlayCursor = 5
		}
	case modeFilters:
		m.overlayCursor = min(max(m.overlayCursor, 0), 3)
	case modeDetails:
		maxOffset := max(len(m.detailsLines())-overlayListCapacity(m.height), 0)
		m.detailsOffset = min(max(m.detailsOffset, 0), maxOffset)
	}
}

func (m Model) helpOverlay(height int) (string, []string) {
	var lines []string
	title := "Keyboard shortcuts"
	all := helpLines()
	visible := overlayListCapacity(height)
	end := min(m.helpOffset+visible, len(all))
	lines = append(lines, all[m.helpOffset:end]...)
	lines = append(lines, "", "j/k scroll • ctrl+u/d page • gg/G ends • ?/esc close")
	return title, lines
}

func (m Model) detailsOverlay(height int) (string, []string) {
	var lines []string
	title := "Details"
	all := m.detailsLines()
	m.detailsOffset = min(m.detailsOffset, max(len(all)-overlayListCapacity(height), 0))
	end := min(m.detailsOffset+overlayListCapacity(height), len(all))
	lines = append(lines, all[m.detailsOffset:end]...)
	lines = append(lines, "", "j/k scroll • ctrl+u/d page • gg/G ends • d/esc close")
	return title, lines
}

func (m Model) overlayContent(width, height int) (string, []string) {
	switch m.mode {
	case modeCategories:
		return m.categoriesOverlay(width, height)
	case modeSort:
		return m.sortOverlay(height)
	case modeQueue:
		return m.queueOverlay(height)
	case modePlaybackOptions:
		return m.playbackOptionsOverlay(height)
	case modeFilters:
		return m.filtersOverlay()
	case modeHelp:
		return m.helpOverlay(height)
	case modeDetails:
		return m.detailsOverlay(height)
	case modeMappingUpdate:
		return m.mappingUpdateOverlay()
	case modeMappingPicker:
		return m.mappingPickerOverlay(height)
	case modeSpotifyUpdate:
		return m.spotifyUpdateOverlay()
	case modeSpotifySearch:
		return m.spotifySearchOverlay()
	case modeLastFM:
		return m.lastFMOverlay(height)
	}
	return "", nil
}

func (m Model) renderOverlay(base string, width, height int) string {
	title, lines := m.overlayContent(width, height)

	overlayWidth := min(max(width*2/3, 28), min(88, width))
	if m.mode == modePlaybackOptions {
		overlayWidth = min(88, width)
	}
	contentWidth := max(overlayWidth-6, 1)
	title = truncate(title, contentWidth)
	for index, line := range lines {
		lines[index] = truncateANSI(line, contentWidth)
	}
	separator := "\n\n"
	if height < 16 {
		separator = "\n"
		lines = slices.DeleteFunc(lines, func(line string) bool { return line == "" })
	}
	content := m.theme.overlayTitle.Render(title) + separator + strings.Join(lines, "\n")
	overlay := m.theme.overlay.Width(overlayWidth).Render(content)
	return placeOverlay(base, overlay, width, height)
}

func overlayWindow(items []string, cursor, height int) []string {
	visible := overlayListCapacity(height)
	start := min(max(cursor-visible/2, 0), max(len(items)-visible, 0))
	end := min(start+visible, len(items))
	return items[start:end]
}

func overlayListCapacity(height int) int {
	if height < 16 {
		return max(height-6, 1)
	}
	return max(height-8, 1)
}

func (m Model) detailsLines() []string {
	current, ok := m.currentRow()
	if !ok {
		return []string{"No selected media"}
	}
	track := m.filtered[current.trackIndex]
	if current.isVariant() {
		variant := track.Variants[current.variantIndex]
		return append([]string{"Video: " + variant.Filename, "Video path: " + variant.VideoPath, "Track ID: " + track.ID, "Audio source: " + trackSourceLabel(classifyTrackSource(track)), "Local audio: " + availability(track.LocalAudioPath != ""), "Spotify link: " + availability(track.SpotifyURI != ""), "Category: " + string(variant.Category), "Video date: " + variant.DateLabel, "Modified: " + variant.ModifiedAt.UTC().Format(time.RFC3339), queueState(m.queuedID(variant.ID) != "")}, historyLines(variant.History)...)
	}
	eligible := len(library.EligibleVariants(track, m.currentQuery()))
	lines := []string{"Artist: " + track.Artist, "Title: " + track.Title, "Track ID: " + track.ID, "Audio source: " + trackSourceLabel(classifyTrackSource(track)), "Local audio: " + availability(track.LocalAudioPath != ""), "Spotify link: " + availability(track.SpotifyURI != ""), "Release: " + emptyAny(track.ReleaseDateLabel), fmt.Sprintf("Variants: %d total • %d eligible", len(track.Variants), eligible), queueState(m.trackQueued(track))}
	if track.LastFM.PlayedCount > 0 {
		lines = append(lines, fmt.Sprintf("Last.fm plays: %d", track.LastFM.PlayedCount), "Last.fm first play: "+track.LastFM.FirstPlayedAtUTC.UTC().Format(time.RFC3339), "Last.fm last play: "+track.LastFM.LastPlayedAtUTC.UTC().Format(time.RFC3339))
	}
	return append(lines, historyLines(track.History)...)
}

func trackSourceLabel(source trackSource) string {
	switch source {
	case trackSourceSpotify:
		return "Spotify"
	case trackSourceLocal:
		return "Local"
	default:
		return "Video only"
	}
}

func historyLines(value library.History) []string {
	lines := []string{fmt.Sprintf("History: %d played • %d completed • %d stopped • %d skipped", value.PlayedCount, value.CompletedCount, value.StoppedCount, value.SkippedCount), fmt.Sprintf("Recovery: %d not started • %d abandoned", value.NotStartedCount, value.AbandonedCount)}
	if value.LastPlayedAtUTC != nil {
		lines = append(lines, "Last played: "+value.LastPlayedAtUTC.UTC().Format(time.RFC3339))
	}
	if value.LastAttemptedAtUTC != nil {
		lines = append(lines, "Last attempted: "+value.LastAttemptedAtUTC.UTC().Format(time.RFC3339))
	}
	for _, event := range value.Recent {
		detail := event.Outcome + " • " + event.AtUTC.UTC().Format("2006-01-02 15:04")
		if event.Percent != nil {
			detail += fmt.Sprintf(" • %.0f%%", *event.Percent)
		}
		lines = append(lines, detail)
	}
	return lines
}
