package ui

import (
	"context"
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"

	"playlistmaker/charm/internal/lastfm"
	"playlistmaker/charm/internal/library"
)

func (m Model) handleLastFMKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.lastfmRunning {
		if key.String() == "esc" || key.Text == "L" {
			if m.lastfmCancel != nil {
				m.lastfmCancelling = true
				m.status = "Cancelling Last.fm sync…"
				m.lastfmCancel()
			}
		}
		return m, nil
	}
	switch key.String() {
	case "esc", "L", "shift+l":
		m.mode = modeNavigate
		m.lastfmResetArmed = false
	case "j", "down":
		m.overlayCursor = min(m.overlayCursor+1, 4)
		m.lastfmResetArmed = false
	case "k", "up":
		m.overlayCursor = max(m.overlayCursor-1, 0)
		m.lastfmResetArmed = false
	case "enter":
		switch m.overlayCursor {
		case 0, 1:
			if !m.lastfmStatus.Configured {
				m.status = "Last.fm sync is disabled because username and API key are not configured"
				return m, nil
			}
			return m.beginLastFMSync(m.overlayCursor == 1)
		case 2:
			return m, m.lastfmExportCmd()
		case 3:
			return m, m.lastfmImportCmd()
		case 4:
			if !m.lastfmResetArmed {
				m.lastfmResetArmed = true
				m.status = "Press Enter again to reset all agent decisions"
				return m, nil
			}
			return m, m.lastfmResetCmd()
		}
	}
	return m, nil
}

func (m Model) beginLastFMSync(full bool) (tea.Model, tea.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())
	runner := &lastfmSyncRunner{updates: make(chan lastfmProgressMsg, 1), done: make(chan lastfmSyncMsg, 1)}
	m.lastfmRunner, m.lastfmCancel, m.lastfmRunning, m.lastfmCancelling = runner, cancel, true, false
	m.lastfmProgress = lastfm.SyncProgress{Phase: "starting"}
	service, tracks := m.lastfm, append([]library.Track(nil), m.all...)
	go func() {
		result, err := service.Sync(ctx, tracks, full, func(p lastfm.SyncProgress) {
			select {
			case runner.updates <- lastfmProgressMsg{p}:
			case <-ctx.Done():
			}
		})
		runner.done <- lastfmSyncMsg{result: result, err: err, cancelled: errors.Is(err, context.Canceled)}
	}()
	return m, m.waitLastFMSyncCmd()
}
func (m Model) waitLastFMSyncCmd() tea.Cmd {
	runner := m.lastfmRunner
	return func() tea.Msg {
		select {
		case p := <-runner.updates:
			return p
		case done := <-runner.done:
			return done
		}
	}
}
func (m Model) lastfmExportCmd() tea.Cmd {
	service, tracks := m.lastfm, append([]library.Track(nil), m.all...)
	return func() tea.Msg {
		path, err := service.ExportReview(tracks, time.Now())
		return lastfmActionMsg{action: "export", path: path, err: err}
	}
}
func (m Model) lastfmImportCmd() tea.Cmd {
	service, tracks := m.lastfm, append([]library.Track(nil), m.all...)
	return func() tea.Msg {
		result, err := service.ImportDecisions(tracks)
		return lastfmActionMsg{action: "import", imported: result, err: err}
	}
}
func (m Model) lastfmResetCmd() tea.Cmd {
	service, tracks := m.lastfm, append([]library.Track(nil), m.all...)
	return func() tea.Msg { return lastfmActionMsg{action: "reset", err: service.ResetAgentDecisions(tracks)} }
}
