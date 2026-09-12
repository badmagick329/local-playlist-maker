package ui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"playlistmaker/charm/internal/spotifylink"
)

type SpotifyUpdater interface {
	SpotifyScan(context.Context, func(spotifylink.ScanProgress)) (spotifylink.ScanResult, error)
	SpotifySearch(context.Context, string) ([]spotifylink.Candidate, error)
	SpotifyValidate(context.Context, string) (spotifylink.Candidate, error)
	SpotifyConfirm(context.Context, string, string) error
	SpotifyIgnore(string) error
	libraryReloader
}

type spotifyScanMsg struct {
	result    spotifylink.ScanResult
	err       error
	progress  spotifylink.ScanProgress
	cancelled bool
}

type spotifyScanProgressMsg struct{ progress spotifylink.ScanProgress }

type spotifySearchMsg struct {
	items []spotifylink.Candidate
	err   error
}

type spotifyValidateMsg struct {
	item spotifylink.Candidate
	err  error
}

type spotifySaveMsg struct{ err error }

type spotifyScanRunner struct {
	updates chan spotifyScanProgressMsg
	done    chan spotifyScanMsg
}

func (m Model) WithSpotifyUpdater(updater SpotifyUpdater) Model {
	m.spotifyUpdater = updater
	return m
}

func (m Model) handleSpotifySave(message spotifySaveMsg) (tea.Model, tea.Cmd) {
	if message.err != nil {
		m.status = "Spotify link save failed: " + message.err.Error()
	} else {
		m.spotifyDirty = true
		m.removeCurrentSpotifyItem()
		m.status = "Spotify link saved"
	}
	return m, nil
}

func (m Model) handleSpotifyValidate(message spotifyValidateMsg) (tea.Model, tea.Cmd) {
	m.spotifyScanning = false
	if message.err != nil {
		m.status = "Spotify track validation failed: " + message.err.Error()
	} else if item, ok := m.currentSpotifyItem(); ok {
		item.Candidates = []spotifylink.Candidate{message.item}
		item.Reason = "Validated pasted Spotify track"
		m.spotifyItems[m.spotifyIndex], m.spotifyCandidate, m.mode = item, 0, modeSpotifyUpdate
	}
	return m, nil
}

func (m Model) handleSpotifySearch(message spotifySearchMsg) (tea.Model, tea.Cmd) {
	m.spotifyScanning = false
	if message.err != nil {
		m.status = "Spotify search failed: " + message.err.Error()
	} else if item, ok := m.currentSpotifyItem(); ok {
		item.Candidates = message.items
		item.Reason = "Manual search results"
		m.spotifyItems[m.spotifyIndex], m.spotifyCandidate, m.mode = item, 0, modeSpotifyUpdate
	}
	return m, nil
}

func (m Model) handleSpotifyScanProgress(message spotifyScanProgressMsg) (tea.Model, tea.Cmd) {
	if m.spotifyScanning {
		m.spotifyProgress = message.progress
		return m, m.waitSpotifyScanCmd()
	}
	return m, nil
}

func (m Model) handleSpotifyScan(message spotifyScanMsg) (tea.Model, tea.Cmd) {
	m.spotifyScanning = false
	m.spotifyCancelling = false
	m.spotifyScan = nil
	m.spotifyScanCancel = nil
	m.spotifyProgress = message.progress
	m.spotifyItems, m.spotifyIndex, m.spotifyCandidate = message.result.Items, 0, 0
	m.spotifyDirty = m.spotifyDirty || message.result.AutoLinked > 0
	m.spotifyScanError = ""
	if message.cancelled {
		m.status = fmt.Sprintf("Spotify scan cancelled after %d/%d; %d links saved; %d need review", message.progress.Current, message.progress.Total, message.result.AutoLinked, len(message.result.Items))
		return m, nil
	}
	if message.err != nil {
		m.spotifyScanError = message.err.Error()
		if message.progress.Phase == "authenticating" {
			m.status = "Spotify authentication failed: " + message.err.Error()
		} else if message.progress.Total > 0 {
			m.status = fmt.Sprintf("Spotify scan failed at %d/%d; %d links saved; %d need review: %v", message.progress.Current, message.progress.Total, message.result.AutoLinked, len(message.result.Items), message.err)
		} else {
			m.status = "Spotify link scan failed: " + message.err.Error()
		}
	} else {
		m.status = fmt.Sprintf("%d Spotify links saved automatically; %d need review", message.result.AutoLinked, len(message.result.Items))
	}
	return m, nil
}

func (m Model) handleSpotifyUpdateKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.spotifyScanning {
		if key.String() == "esc" || key.String() == "U" || key.String() == "shift+u" {
			if m.spotifyScanCancel != nil {
				m.spotifyCancelling = true
				m.status = "Cancelling Spotify scan…"
				m.spotifyScanCancel()
			}
		}
		return m, nil
	}
	switch key.String() {
	case "esc", "U", "shift+u":
		m.mode = modeNavigate
		if m.spotifyDirty {
			return m, reloadLibraryCmd(m.spotifyUpdater)
		}
	case "j", "down":
		m.spotifyIndex = min(m.spotifyIndex+1, max(len(m.spotifyItems)-1, 0))
		m.spotifyCandidate = 0
	case "k", "up":
		m.spotifyIndex = max(m.spotifyIndex-1, 0)
		m.spotifyCandidate = 0
	case "l", "right":
		if item, ok := m.currentSpotifyItem(); ok {
			m.spotifyCandidate = min(m.spotifyCandidate+1, max(len(item.Candidates)-1, 0))
		}
	case "h", "left":
		m.spotifyCandidate = max(m.spotifyCandidate-1, 0)
	case "/":
		m.mode, m.spotifyQuery = modeSpotifySearch, ""
	case "s":
		m.removeCurrentSpotifyItem()
		m.status = "Spotify link skipped"
	case "i":
		if item, ok := m.currentSpotifyItem(); ok {
			return m, m.spotifyIgnoreCmd(item.TrackID)
		}
	case "r":
		m.spotifyScanning, m.spotifyItems, m.spotifyIndex, m.spotifyCandidate = true, nil, 0, 0
		m.spotifyScanError = ""
		m.status = "Starting Spotify link scan…"
		return m.beginSpotifyScan()
	case "enter":
		if item, ok := m.currentSpotifyItem(); ok && m.spotifyCandidate < len(item.Candidates) {
			return m, m.spotifyConfirmCmd(item.TrackID, item.Candidates[m.spotifyCandidate].URI)
		}
	}
	return m, nil
}

func (m Model) handleSpotifySearchKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc", "/":
		m.mode = modeSpotifyUpdate
	case "enter":
		if strings.TrimSpace(m.spotifyQuery) == "" {
			return m, nil
		}
		m.spotifyScanning = true
		if strings.Contains(m.spotifyQuery, "spotify:track:") || strings.Contains(m.spotifyQuery, "open.spotify.com/track/") {
			return m, m.spotifyValidateCmd()
		}
		return m, m.spotifySearchCmd()
	case "backspace":
		if m.spotifyQuery != "" {
			_, size := utf8.DecodeLastRuneInString(m.spotifyQuery)
			m.spotifyQuery = m.spotifyQuery[:len(m.spotifyQuery)-size]
		}
	default:
		if key.Text != "" && !key.Mod.Contains(tea.ModCtrl) && !key.Mod.Contains(tea.ModAlt) {
			m.spotifyQuery += key.Text
		}
	}
	return m, nil
}

func (m Model) beginSpotifyScan() (tea.Model, tea.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())
	runner := &spotifyScanRunner{updates: make(chan spotifyScanProgressMsg, 1), done: make(chan spotifyScanMsg, 1)}
	m.spotifyScan, m.spotifyScanCancel, m.spotifyCancelling = runner, cancel, false
	service := m.spotifyUpdater
	go func() {
		last := spotifylink.ScanProgress{}
		result, err := service.SpotifyScan(ctx, func(progress spotifylink.ScanProgress) {
			last = progress
			select {
			case runner.updates <- spotifyScanProgressMsg{progress: progress}:
			case <-ctx.Done():
			}
		})
		runner.done <- spotifyScanMsg{result: result, err: err, progress: last, cancelled: errors.Is(err, context.Canceled)}
	}()
	return m, m.waitSpotifyScanCmd()
}

func (m Model) waitSpotifyScanCmd() tea.Cmd {
	runner := m.spotifyScan
	return func() tea.Msg {
		select {
		case progress := <-runner.updates:
			return progress
		case result := <-runner.done:
			return result
		}
	}
}

func (m Model) spotifySearchCmd() tea.Cmd {
	service, query := m.spotifyUpdater, m.spotifyQuery
	return func() tea.Msg {
		items, err := service.SpotifySearch(context.Background(), query)
		return spotifySearchMsg{items: items, err: err}
	}
}

func (m Model) spotifyValidateCmd() tea.Cmd {
	service, value := m.spotifyUpdater, m.spotifyQuery
	return func() tea.Msg {
		item, err := service.SpotifyValidate(context.Background(), value)
		return spotifyValidateMsg{item: item, err: err}
	}
}

func (m Model) spotifyConfirmCmd(trackID, uri string) tea.Cmd {
	service := m.spotifyUpdater
	return func() tea.Msg { return spotifySaveMsg{err: service.SpotifyConfirm(context.Background(), trackID, uri)} }
}

func (m Model) spotifyIgnoreCmd(trackID string) tea.Cmd {
	service := m.spotifyUpdater
	return func() tea.Msg { return spotifySaveMsg{err: service.SpotifyIgnore(trackID)} }
}

func (m Model) currentSpotifyItem() (spotifylink.Item, bool) {
	if m.spotifyIndex < 0 || m.spotifyIndex >= len(m.spotifyItems) {
		return spotifylink.Item{}, false
	}
	return m.spotifyItems[m.spotifyIndex], true
}

func (m *Model) removeCurrentSpotifyItem() {
	if m.spotifyIndex < 0 || m.spotifyIndex >= len(m.spotifyItems) {
		return
	}
	m.spotifyItems = slices.Delete(m.spotifyItems, m.spotifyIndex, m.spotifyIndex+1)
	m.spotifyIndex = min(m.spotifyIndex, max(len(m.spotifyItems)-1, 0))
	m.spotifyCandidate = 0
}

func (m Model) spotifyUpdateOverlay() (string, []string) {
	var lines []string
	title := "Update Spotify links"
	if m.spotifyScanning {
		if m.spotifyCancelling {
			lines = []string{"Cancelling Spotify scan…", "", "Please wait"}
			return title, lines
		}
		switch m.spotifyProgress.Phase {
		case "authenticating":
			lines = []string{"Authenticating with Spotify…", "", "Esc cancel"}
		case "scanning":
			lines = []string{"Scanning Spotify links", fmt.Sprintf("Track %d of %d", m.spotifyProgress.Current, m.spotifyProgress.Total), fmt.Sprintf("Automatically linked: %d", m.spotifyProgress.AutoLinked), fmt.Sprintf("Needs review: %d", m.spotifyProgress.ReviewCount)}
			if m.spotifyProgress.Artist != "" || m.spotifyProgress.Title != "" {
				lines = append(lines, "Current: "+m.spotifyProgress.Artist+" — "+m.spotifyProgress.Title)
			}
			lines = append(lines, "", "Esc cancel")
		default:
			lines = []string{"Starting Spotify link scan…", "", "Esc cancel"}
		}
		return title, lines
	}
	item, ok := m.currentSpotifyItem()
	if !ok {
		if m.spotifyScanError != "" {
			lines = []string{"Spotify scan failed", m.spotifyScanError, "", "r rescan • U/esc close"}
		} else {
			lines = []string{"All eligible catalogue tracks have Spotify links or are ignored", "", "r rescan • U/esc close"}
		}
		return title, lines
	}
	lines = []string{fmt.Sprintf("%d of %d", m.spotifyIndex+1, len(m.spotifyItems)), "LOCAL TRACK", item.Artist + " — " + item.Title, "Release: " + emptyAny(item.ReleaseDate)}
	if len(item.Candidates) == 0 {
		lines = append(lines, "", item.Reason, "No suggestion; use / to search or paste a Spotify track")
	} else {
		candidate := item.Candidates[min(m.spotifyCandidate, len(item.Candidates)-1)]
		lines = append(lines, "", fmt.Sprintf("SPOTIFY CANDIDATE %d of %d", m.spotifyCandidate+1, len(item.Candidates)), candidate.Artist+" — "+candidate.Title, "Release: "+emptyAny(candidate.ReleaseDate))
		if candidate.ReleaseDateMatch {
			lines = append(lines, "Release date matches")
		}
		lines = append(lines, "Album: "+emptyAny(candidate.Album), fmt.Sprintf("Duration: %d:%02d", candidate.DurationMS/60000, candidate.DurationMS/1000%60), "", item.Reason)
	}
	lines = append(lines, "", "h/l candidate • enter confirm • / search or paste • s skip • i ignore • r rescan • U/esc close")
	return title, lines
}

func (m Model) spotifySearchOverlay() (string, []string) {
	var lines []string
	title := "Search Spotify"
	lines = []string{"Query or Spotify track URL/URI:", m.spotifyQuery + "▏", "", "enter validate/search • / or esc cancel"}
	return title, lines
}
