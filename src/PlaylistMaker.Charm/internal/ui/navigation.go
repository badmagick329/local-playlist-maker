package ui

import (
	"slices"

	tea "charm.land/bubbletea/v2"

	"playlistmaker/charm/internal/library"
	"playlistmaker/charm/internal/updater"
)

func (m Model) handleKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeSearch:
		return m.handleSearchKey(key), nil
	case modeCategories:
		return m.handleCategoryKey(key), nil
	case modeSort:
		return m.handleSortKey(key), nil
	case modeQueue:
		return m.handleQueueKey(key), nil
	case modePlaybackOptions:
		return m.handlePlaybackPanelKey(key)
	case modeFilters:
		return m.handleFiltersKey(key), nil
	case modeHelp:
		return m.handleHelpKey(key), nil
	case modeDetails:
		return m.handleDetailsKey(key), nil
	case modeMappingUpdate:
		return m.handleMappingUpdateKey(key)
	case modeMappingPicker:
		return m.handleMappingPickerKey(key)
	case modeSpotifyUpdate:
		return m.handleSpotifyUpdateKey(key)
	case modeSpotifySearch:
		return m.handleSpotifySearchKey(key)
	case modeLastFM:
		return m.handleLastFMKey(key)
	default:
		return m.handleNavigationKey(key)
	}
}

func (m Model) handleNavigationKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.String() != "g" {
		m.waitingForG = false
	}
	if key.Text == "R" {
		return m.requestHistoryRefresh()
	}
	if key.Text == "U" {
		if m.spotifyUpdater == nil {
			m.status = "Spotify updates are unavailable"
			return m, nil
		}
		m.mode, m.spotifyScanning, m.spotifyItems, m.spotifyIndex, m.spotifyCandidate = modeSpotifyUpdate, true, nil, 0, 0
		m.spotifyScanError = ""
		m.status = "Starting Spotify link scan…"
		return m.beginSpotifyScan()
	}
	if key.Text == "L" {
		if m.lastfm == nil {
			m.status = "Last.fm is unavailable"
			return m, nil
		}
		m.mode, m.overlayCursor, m.lastfmResetArmed = modeLastFM, 0, false
		m.lastfmStatus = m.lastfm.Status()
		return m, nil
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
		m.moveCursor(1)
	case "a":
		m.queueCurrentTrack()
	case "A":
		m.queueFilteredTracks()
	case "ctrl+a":
		m.queueAllFilteredVariants()
	case "/":
		m.mode = modeSearch
		m.status = "Search mode: type freely; Enter or Esc returns to navigation"
	case "C":
		m.query = ""
		m.refreshResults()
		m.cursor = 0
		m.status = "Search cleared"
	case "c":
		m.mode = modeCategories
		m.overlayCursor = 0
	case "s":
		m.mode = modeSort
		m.overlayCursor = slices.Index(library.Sorts, m.sort)
	case "o":
		return m.launchQueue()
	case "p":
		m.openPlaybackPanel()
	case "R", "shift+r":
		return m.requestHistoryRefresh()
	case "f":
		m.mode, m.overlayCursor = modeFilters, 0
		m.filterDraft = [2]string{}
		if m.trackDate != nil {
			m.filterDraft[0] = m.trackDate.Label
		}
		if m.videoDate != nil {
			m.filterDraft[1] = m.videoDate.Label
		}
	case "q":
		m.mode = modeQueue
		m.overlayCursor = min(m.overlayCursor, max(len(m.queueOrder)-1, 0))
	case "?":
		m.mode, m.helpOffset = modeHelp, 0
	case "d":
		m.mode, m.detailsOffset = modeDetails, 0
	case "m":
		if m.mappingUpdater == nil {
			m.status = "Mapping updates are unavailable"
			return m, nil
		}
		if len(m.rows) == 0 || !m.rows[m.cursor].isVariant() {
			m.status = "Expand a track and select the video to relink"
			return m, nil
		}
		selected := m.rows[m.cursor]
		track := m.filtered[selected.trackIndex]
		video := track.Variants[selected.variantIndex]
		m.mappingItems = []updater.Item{{VideoPath: video.VideoPath, Filename: video.Filename, Artist: track.Artist, Title: track.Title}}
		m.mappingIndex, m.mappingRelink = 0, true
		return m.openMappingPicker(track.Artist + " " + track.Title)
	case "u":
		if m.mappingUpdater == nil {
			m.status = "Mapping updates are unavailable"
			return m, nil
		}
		m.mode, m.mappingScanning, m.mappingItems, m.mappingIndex, m.mappingIgnored = modeMappingUpdate, true, nil, 0, false
		m.status = "Scanning video and audio folders…"
		return m, m.mappingScanCmd()
	case "esc":
		m.status = "Esc closes modes; Ctrl+Q quits"
	}
	return m, nil
}
