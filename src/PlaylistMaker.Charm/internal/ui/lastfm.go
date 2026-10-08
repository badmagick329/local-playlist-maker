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
	ExportReview([]library.Track, time.Time, *library.DateRange) (string, error)
	ImportDecisions([]library.Track) (lastfm.ImportResult, error)
	ResetAgentDecisions([]library.Track) error
	LastSyncReport([]library.Track) (lastfm.SyncReport, bool)
	LastSyncPeriod() (*library.DateRange, error)
}

// Last.fm screen rows. The export row carries its scope, cycled with h/l.
const (
	lastfmSyncRow = iota
	lastfmRebuildRow
	lastfmReportRow
	lastfmExportRow
	lastfmImportRow
	lastfmResetRow
)

type exportScope int

const (
	exportAll exportScope = iota
	exportSinceLastSync
	exportRange
	exportScopeCount
)

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
	if m.lastfmReport != nil {
		return m.handleLastFMReportKey(key)
	}
	k := key.String()
	switch k {
	case "esc", "L", "shift+l":
		m.mode = modeNavigate
		m.lastfmResetArmed = false
	case "j", "down":
		m.overlayCursor = min(m.overlayCursor+1, lastfmResetRow)
		m.lastfmResetArmed = false
	case "k", "up":
		m.overlayCursor = max(m.overlayCursor-1, 0)
		m.lastfmResetArmed = false
	case "h", "left", "l", "right":
		if m.overlayCursor == lastfmExportRow {
			step := exportScope(1)
			if k == "h" || k == "left" {
				step = exportScopeCount - 1
			}
			m.lastfmExportScope = (m.lastfmExportScope + step) % exportScopeCount
		}
	case "enter":
		switch m.overlayCursor {
		case lastfmSyncRow, lastfmRebuildRow:
			if !m.lastfmStatus.Configured {
				m.status = "Last.fm sync is disabled because username and API key are not configured"
				return m, nil
			}
			return m.beginLastFMSync(m.overlayCursor == lastfmRebuildRow)
		case lastfmReportRow:
			report, ok := m.lastfm.LastSyncReport(m.all)
			if !ok {
				m.status = "No Last.fm sync is recorded yet"
				return m, nil
			}
			m.lastfmReport, m.lastfmReportOffset = &report, 0
		case lastfmExportRow:
			period, err := m.lastfmExportPeriod()
			if err != nil {
				m.status = "Last.fm export: " + err.Error()
				return m, nil
			}
			return m, m.lastfmExportCmd(period)
		case lastfmImportRow:
			return m, m.lastfmImportCmd()
		case lastfmResetRow:
			if !m.lastfmResetArmed {
				m.lastfmResetArmed = true
				m.status = "Press Enter again to reset all agent decisions"
				return m, nil
			}
			return m, m.lastfmResetCmd()
		}
	default:
		if m.overlayCursor == lastfmExportRow && m.lastfmExportScope == exportRange {
			m.editLastFMExportRange(k, key.Text)
		}
	}
	return m, nil
}

// editLastFMExportRange accepts only date characters, so navigation keys and
// dictated words cannot land in the range.
func (m *Model) editLastFMExportRange(k, text string) {
	switch k {
	case "ctrl+u":
		m.lastfmExportRange = ""
		return
	case "backspace":
		if m.lastfmExportRange != "" {
			m.lastfmExportRange = m.lastfmExportRange[:len(m.lastfmExportRange)-1]
		}
		return
	}
	for _, r := range text {
		if r >= '0' && r <= '9' || r == '-' || r == '.' {
			m.lastfmExportRange += string(r)
		}
	}
}

func (m Model) lastfmExportPeriod() (*library.DateRange, error) {
	switch m.lastfmExportScope {
	case exportSinceLastSync:
		return m.lastfm.LastSyncPeriod()
	case exportRange:
		period, err := library.ParseDateRange(m.lastfmExportRange)
		if err == nil && period == nil {
			err = errors.New("type a date range, such as 2026-09 or 2026-09-08..2026-10-08")
		}
		return period, err
	}
	return nil, nil
}

func (m Model) handleLastFMReportKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc", "enter":
		m.lastfmReport = nil
	case "L", "shift+l":
		m.lastfmReport = nil
		m.mode = modeNavigate
	case "j", "down":
		m.lastfmReportOffset = min(m.lastfmReportOffset+1, max(len(lastfmReportLines(*m.lastfmReport))-1, 0))
	case "k", "up":
		m.lastfmReportOffset = max(m.lastfmReportOffset-1, 0)
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

func (m Model) lastfmExportCmd(period *library.DateRange) tea.Cmd {
	service, tracks := m.lastfm, append([]library.Track(nil), m.all...)
	return func() tea.Msg {
		path, err := service.ExportReview(tracks, time.Now(), period)
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
			m.status = fmt.Sprintf("Last.fm decisions: %d matched, %d no-match, %d needs-human, %d already resolved, %d missing, %d invalid", message.imported.Matched, message.imported.NoMatch, message.imported.NeedsHuman, message.imported.AlreadyResolved, message.imported.Missing, message.imported.Invalid)
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
	} else if errors.Is(message.err, lastfm.ErrCacheNotLoaded) {
		m.status = "Last.fm sync refused: " + message.err.Error()
	} else if message.err != nil && !errors.Is(message.err, lastfm.ErrSyncLogNotSaved) {
		m.status = "Last.fm sync failed: " + message.err.Error() + " Run the same sync action to resume."
	} else {
		m.all = m.lastfm.Attach(m.all)
		m.refreshResults()
		report := message.result.Report
		m.lastfmReport, m.lastfmReportOffset = &report, 0
		m.status = fmt.Sprintf("Last.fm sync added %d scrobbles: %d with a Spotify link, %d unresolved, %d no match, %d without a Spotify link", report.Added, report.Linked, lastfm.ScrobbleTotal(report.Unresolved), lastfm.ScrobbleTotal(report.NoMatch), lastfm.ScrobbleTotal(report.NoSpotifyLink))
		if message.err != nil {
			m.status = "Last.fm sync saved its scrobbles, but " + message.err.Error()
		}
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
			lines = []string{"Caching Spotify metadata for linked catalogue tracks", fmt.Sprintf("Track %d of %d", m.lastfmProgress.SpotifyCurrent, m.lastfmProgress.SpotifyTotal), "", "Esc cancel"}
		} else {
			operation := "Fetching Last.fm scrobbles"
			if m.lastfmProgress.Resumed {
				operation = "Resuming Last.fm scrobbles"
			}
			lines = []string{operation, fmt.Sprintf("Page %d of %d", m.lastfmProgress.PagesFetched, m.lastfmProgress.TotalPages), fmt.Sprintf("Checkpointed: %d", m.lastfmProgress.Scrobbles), "", "Esc cancel"}
		}
		return title, lines
	}
	if m.lastfmReport != nil {
		body := lastfmReportLines(*m.lastfmReport)
		visible := overlayListCapacity(height)
		start := min(m.lastfmReportOffset, max(len(body)-visible, 0))
		lines = append(lines, body[start:min(start+visible, len(body))]...)
		return "Last.fm sync report", append(lines, "", "j/k scroll • enter/esc back • L close")
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
	syncLabel := "not recorded"
	if m.lastfmStatus.LastSyncUTC != nil {
		syncLabel = m.lastfmStatus.LastSyncUTC.Format(time.RFC3339)
	}
	actions := []string{
		fmt.Sprintf("%s Sync new plays", cursorMark(m.overlayCursor, lastfmSyncRow)),
		fmt.Sprintf("%s Rebuild full history", cursorMark(m.overlayCursor, lastfmRebuildRow)),
		fmt.Sprintf("%s Last sync report", cursorMark(m.overlayCursor, lastfmReportRow)),
		fmt.Sprintf("%s Export unresolved: %s", cursorMark(m.overlayCursor, lastfmExportRow), m.lastfmExportScopeLabel()),
		fmt.Sprintf("%s Import agent decisions", cursorMark(m.overlayCursor, lastfmImportRow)),
		fmt.Sprintf("%s Reset agent decisions", cursorMark(m.overlayCursor, lastfmResetRow)),
	}
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
		lines = append(lines, "", "j/k move • h/l export scope • enter activate • L/esc close")
	}
	return title, lines
}

func (m Model) lastfmExportScopeLabel() string {
	switch m.lastfmExportScope {
	case exportSinceLastSync:
		return "‹ since last sync ›"
	case exportRange:
		value := m.lastfmExportRange
		if value == "" {
			value = "type YYYY, YYYY-MM, YYYY-MM-DD or START..END"
		}
		return "‹ range " + value + " ›"
	}
	return "‹ all ›"
}

func lastfmReportLines(r lastfm.SyncReport) []string {
	kind := "new plays"
	if r.Full {
		kind = "full rebuild"
	}
	scope := fmt.Sprintf("Scrobbles added: %d (cache was empty)", r.Added)
	if r.AfterUTC != nil {
		scope = fmt.Sprintf("Scrobbles after %s: %d", r.AfterUTC.Format("2006-01-02 15:04 UTC"), r.Added)
	}
	lines := []string{"Synced " + r.SyncedAtUTC.Format("2006-01-02 15:04 UTC") + " (" + kind + ")", scope, fmt.Sprintf("With a Spotify link: %d", r.Linked)}
	section := func(heading string, songs []lastfm.ReportSong) {
		lines = append(lines, "", fmt.Sprintf("%s: %d scrobbles, %d songs", heading, lastfm.ScrobbleTotal(songs), len(songs)))
		for _, song := range songs {
			line := fmt.Sprintf("  %s – %s • %d", song.Artist, song.Title, song.Scrobbles)
			if song.TrackID != "" {
				reason := "not linked"
				if song.SpotifyIgnored {
					reason = "Spotify ignored"
				}
				line += " • " + reason
			}
			lines = append(lines, line)
		}
	}
	section("Unresolved", r.Unresolved)
	section("No match in catalogue", r.NoMatch)
	section("Matched without a Spotify link", r.NoSpotifyLink)
	return lines
}
