package ui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"playlistmaker/charm/internal/library"
)

type HistorySource interface {
	Refresh(context.Context) ([]library.Track, error)
}

// HistoryWatcher emits debounced notifications for changes to the history file.
// It is separate from HistorySource so tests and alternate backends can inject it.
type HistoryWatcher interface {
	Changes() <-chan struct{}
	Close() error
}

type historyRefreshMsg struct {
	tracks []library.Track
	err    error
	manual bool
}

type historyWatchChangedMsg struct{}

type historyWatchClosedMsg struct{}

func (m Model) WithHistorySource(source HistorySource, watcher ...HistoryWatcher) Model {
	m.historySource = source
	// Init immediately starts the first asynchronous refresh. Mark it in flight
	// now so a filesystem notification arriving during startup is coalesced.
	m.historyRefreshing = source != nil
	if len(watcher) > 0 {
		m.historyWatcher = watcher[0]
	}
	return m
}

func (m Model) handleHistoryWatchChanged() (tea.Model, tea.Cmd) {
	if m.historySource == nil {
		return m, nil
	}
	wait := m.waitForHistoryChangeCmd()
	if m.historyRefreshing {
		m.historyPending = true
		return m, wait
	}
	m.historyRefreshing = true
	return m, tea.Batch(wait, m.startHistoryRefreshCmd(false))
}

func (m Model) handleHistoryRefresh(message historyRefreshMsg) (tea.Model, tea.Cmd) {
	m.historyRefreshing = false
	if message.err != nil {
		m.status = "History refresh failed: " + message.err.Error()
	} else {
		m.applyHistory(message.tracks)
		if message.manual {
			m.status = "History refreshed"
		}
	}
	if m.historyPending {
		m.historyPending = false
		m.historyRefreshing = true
		return m, m.startHistoryRefreshCmd(false)
	}
	return m, nil
}

func (m Model) requestHistoryRefresh() (tea.Model, tea.Cmd) {
	if m.historySource == nil {
		m.status = "History refresh is unavailable"
		return m, nil
	}
	if m.historyRefreshing {
		m.status = "History refresh already in progress"
		return m, nil
	}
	m.status, m.historyRefreshing = "Refreshing history…", true
	return m, m.startHistoryRefreshCmd(true)
}

func (m Model) startHistoryRefreshCmd(manual bool) tea.Cmd {
	if m.historySource == nil {
		return nil
	}
	source := m.historySource
	return func() tea.Msg {
		tracks, err := source.Refresh(context.Background())
		return historyRefreshMsg{tracks: tracks, err: err, manual: manual}
	}
}

func (m Model) waitForHistoryChangeCmd() tea.Cmd {
	if m.historyWatcher == nil {
		return nil
	}
	changes := m.historyWatcher.Changes()
	return func() tea.Msg {
		if _, ok := <-changes; !ok {
			return historyWatchClosedMsg{}
		}
		return historyWatchChangedMsg{}
	}
}

func (m *Model) closeHistoryWatcher() {
	if m.historyWatcher == nil {
		return
	}
	_ = m.historyWatcher.Close()
	m.historyWatcher = nil
}

// History refreshes must not replace catalogue metadata or the user's queue choices.
func (m *Model) applyHistory(updated []library.Track) {
	byTrack := make(map[string]library.Track, len(updated))
	for _, track := range updated {
		byTrack[track.ID] = track
	}
	for trackIndex := range m.all {
		fresh, ok := byTrack[m.all[trackIndex].ID]
		if !ok {
			continue
		}
		m.all[trackIndex].History = fresh.History
		byVariant := make(map[string]library.History, len(fresh.Variants))
		for _, variant := range fresh.Variants {
			byVariant[variant.ID] = variant.History
		}
		for variantIndex := range m.all[trackIndex].Variants {
			if value, ok := byVariant[m.all[trackIndex].Variants[variantIndex].ID]; ok {
				m.all[trackIndex].Variants[variantIndex].History = value
			}
		}
	}
	for _, track := range m.all {
		for _, fresh := range track.Variants {
			if variant, ok := m.queued[fresh.ID]; ok {
				variant.History = fresh.History
				m.queued[fresh.ID] = variant
			}
		}
	}
	m.refreshResults()
}
