package ui

import (
	"context"
	"errors"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"playlistmaker/charm/internal/lastfm"
	"playlistmaker/charm/internal/library"
)

type LastFMService interface {
	Status() lastfm.Status
	Sync(context.Context, []library.Track, bool, func(lastfm.SyncProgress)) (lastfm.SyncResult, error)
	Attach([]library.Track) []library.Track
	BuildMix(lastfm.MixRequest) (lastfm.MixResult, error)
	ExportReview([]library.Track, time.Time) (string, error)
	ImportDecisions([]library.Track) (lastfm.ImportResult, error)
	ResetAgentDecisions([]library.Track) error
}

type lastfmProgressMsg struct{ progress lastfm.SyncProgress }

type lastfmSyncMsg struct {
	result    lastfm.SyncResult
	err       error
	cancelled bool
}

type lastfmActionMsg struct {
	action   string
	path     string
	imported lastfm.ImportResult
	err      error
}

type lastfmSyncRunner struct {
	updates chan lastfmProgressMsg
	done    chan lastfmSyncMsg
}

func (m Model) WithLastFM(service LastFMService) Model {
	m.lastfm = service
	if service != nil {
		m.lastfmStatus = service.Status()
		m.all = service.Attach(m.all)
		m.refreshResults()
	}
	return m
}

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

func (m Model) handleLastFMAction(message lastfmActionMsg) (tea.Model, tea.Cmd) {
	if message.err != nil {
		m.status = "Last.fm " + message.action + " failed: " + message.err.Error()
	} else {
		switch message.action {
		case "export":
			m.status = "Last.fm review exported to " + message.path
		case "import":
			m.all = m.lastfm.Attach(m.all)
			m.refreshResults()
			m.status = fmt.Sprintf("Last.fm decisions: %d matched, %d no-match, %d needs-human, %d missing, %d invalid", message.imported.Matched, message.imported.NoMatch, message.imported.NeedsHuman, message.imported.Missing, message.imported.Invalid)
		case "reset":
			m.all = m.lastfm.Attach(m.all)
			m.refreshResults()
			m.status = "Last.fm agent decisions reset"
		}
	}
	m.lastfmStatus = m.lastfm.Status()
	m.lastfmResetArmed = false
	return m, nil
}

func (m Model) handleLastFMSync(message lastfmSyncMsg) (tea.Model, tea.Cmd) {
	m.lastfmRunning, m.lastfmCancelling, m.lastfmRunner, m.lastfmCancel = false, false, nil, nil
	if message.cancelled {
		m.status = "Last.fm operation cancelled; saved progress will resume next time"
	} else if message.err != nil {
		m.status = "Last.fm sync failed: " + message.err.Error() + " Run the same sync action to resume."
	} else {
		m.all = m.lastfm.Attach(m.all)
		m.refreshResults()
		m.status = fmt.Sprintf("Last.fm sync complete: %d scrobbles, %d matched, %d unresolved", message.result.Scrobbles, message.result.Matched, message.result.Unresolved)
	}
	m.lastfmStatus = m.lastfm.Status()
	return m, nil
}

func (m Model) handleLastFMProgress(message lastfmProgressMsg) (tea.Model, tea.Cmd) {
	if m.lastfmRunning {
		m.lastfmProgress = message.progress
		return m, m.waitLastFMSyncCmd()
	}
	return m, nil
}

func (m Model) lastFMOverlay(height int) (string, []string) {
	var lines []string
	title := "Last.fm history"
	if m.lastfmRunning {
		if m.lastfmCancelling {
			lines = []string{"Cancelling Last.fm operation…", "", "Please wait"}
		} else if m.lastfmProgress.Phase == "spotify" {
			lines = []string{"Enriching Spotify evidence", fmt.Sprintf("Track %d of %d", m.lastfmProgress.SpotifyCurrent, m.lastfmProgress.SpotifyTotal), "", "Esc cancel"}
		} else {
			operation := "Fetching Last.fm scrobbles"
			if m.lastfmProgress.Resumed {
				operation = "Resuming Last.fm scrobbles"
			}
			lines = []string{operation, fmt.Sprintf("Page %d of %d", m.lastfmProgress.PagesFetched, m.lastfmProgress.TotalPages), fmt.Sprintf("Checkpointed: %d", m.lastfmProgress.Scrobbles), "", "Esc cancel"}
		}
		return title, lines
	}
	configured := "disabled"
	if m.lastfmStatus.Configured {
		configured = "configured"
	}
	evidence := "incomplete"
	if m.lastfmStatus.SpotifyComplete {
		evidence = "complete"
	}
	rangeLabel := "none"
	if m.lastfmStatus.FirstPlayedAtUTC != nil && m.lastfmStatus.LastPlayedAtUTC != nil {
		rangeLabel = m.lastfmStatus.FirstPlayedAtUTC.Format("2006-01-02") + " to " + m.lastfmStatus.LastPlayedAtUTC.Format("2006-01-02")
	}
	syncLabel := "never"
	if m.lastfmStatus.LastSyncUTC != nil {
		syncLabel = m.lastfmStatus.LastSyncUTC.Format(time.RFC3339)
	}
	actions := []string{fmt.Sprintf("%s Sync new plays", cursorMark(m.overlayCursor, 0)), fmt.Sprintf("%s Rebuild full history", cursorMark(m.overlayCursor, 1)), fmt.Sprintf("%s Export unresolved matches", cursorMark(m.overlayCursor, 2)), fmt.Sprintf("%s Import agent decisions", cursorMark(m.overlayCursor, 3)), fmt.Sprintf("%s Reset agent decisions", cursorMark(m.overlayCursor, 4))}
	if height < 20 {
		summary := fmt.Sprintf("%d scrobbles • %d matched • %d unresolved", m.lastfmStatus.Scrobbles, m.lastfmStatus.Matched, m.lastfmStatus.Unresolved)
		if m.lastfmStatus.CheckpointPages > 0 {
			summary = fmt.Sprintf("Resume saved: page %d of %d", m.lastfmStatus.CheckpointPages, m.lastfmStatus.CheckpointTotal)
		}
		if m.lastfmStatus.Error != "" {
			summary = "Cache error: " + m.lastfmStatus.Error
		}
		lines = []string{summary}
		lines = append(lines, overlayWindow(actions, m.overlayCursor, height)...)
		lines = append(lines, "", "enter activate • L/esc close")
	} else {
		lines = []string{"Last.fm: " + configured, fmt.Sprintf("Cached scrobbles: %d", m.lastfmStatus.Scrobbles), "Cached range: " + rangeLabel, fmt.Sprintf("Matched identities: %d", m.lastfmStatus.Matched), fmt.Sprintf("Unresolved identities: %d", m.lastfmStatus.Unresolved), "Last successful sync: " + syncLabel, "Spotify evidence: " + evidence, ""}
		if m.lastfmStatus.CheckpointPages > 0 {
			lines = append(lines, fmt.Sprintf("Saved sync checkpoint: page %d of %d", m.lastfmStatus.CheckpointPages, m.lastfmStatus.CheckpointTotal), "")
		}
		lines = append(lines, actions...)
		if m.lastfmStatus.Error != "" {
			lines = append(lines, m.theme.warning.Render("Cache error: "+m.lastfmStatus.Error))
		}
		lines = append(lines, "", "j/k move • enter activate • L/esc close")
	}
	return title, lines
}
