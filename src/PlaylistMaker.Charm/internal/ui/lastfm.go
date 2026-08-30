package ui

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

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
		m.overlayCursor = min(m.overlayCursor+1, 5)
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
			m.mode, m.overlayCursor = modeLastFMMix, 0
			m.lastfmMixDraft = [4]string{"", "", "20", "10"}
			m.lastfmMixMethod = lastfm.WeightedRandom
			m.lastfmQueueAction = lastfm.ReplaceQueue
		case 3:
			return m, m.lastfmExportCmd()
		case 4:
			return m, m.lastfmImportCmd()
		case 5:
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

func (m Model) handleLastFMMixKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		m.mode, m.overlayCursor = modeLastFM, 0
		return m, nil
	case "j", "down":
		m.overlayCursor = min(m.overlayCursor+1, 6)
	case "k", "up":
		m.overlayCursor = max(m.overlayCursor-1, 0)
	case "h", "left":
		if m.overlayCursor == 4 {
			m.lastfmMixMethod = m.lastfmMixMethod.Next(-1)
		} else if m.overlayCursor == 5 {
			m.lastfmQueueAction = lastfm.QueueAction(1 - int(m.lastfmQueueAction))
		}
	case "l", "right", "space":
		if m.overlayCursor == 4 {
			m.lastfmMixMethod = m.lastfmMixMethod.Next(1)
		} else if m.overlayCursor == 5 {
			m.lastfmQueueAction = lastfm.QueueAction(1 - int(m.lastfmQueueAction))
		}
	case "backspace":
		if m.overlayCursor < 4 && m.lastfmMixDraft[m.overlayCursor] != "" {
			_, size := utf8.DecodeLastRuneInString(m.lastfmMixDraft[m.overlayCursor])
			m.lastfmMixDraft[m.overlayCursor] = m.lastfmMixDraft[m.overlayCursor][:len(m.lastfmMixDraft[m.overlayCursor])-size]
		}
	case "r":
		m.lastfmMixDraft = [4]string{"", "", "20", "10"}
		m.lastfmMixMethod = lastfm.WeightedRandom
		m.lastfmQueueAction = lastfm.ReplaceQueue
	case "enter":
		if m.overlayCursor == 6 {
			return m.buildLastFMMix()
		}
	}
	if m.overlayCursor < 2 && key.Text != "" && !key.Mod.Contains(tea.ModCtrl) && !key.Mod.Contains(tea.ModAlt) {
		for _, r := range key.Text {
			if (r >= '0' && r <= '9') || r == '-' || r == '.' {
				m.lastfmMixDraft[m.overlayCursor] += string(r)
			}
		}
	}
	if m.overlayCursor >= 2 && m.overlayCursor <= 3 && key.Text >= "0" && key.Text <= "9" {
		m.lastfmMixDraft[m.overlayCursor] += key.Text
	}
	return m, nil
}
func (m Model) buildLastFMMix() (tea.Model, tea.Cmd) {
	primary, err := library.ParseDateRange(strings.TrimSpace(m.lastfmMixDraft[0]))
	if err != nil {
		m.status = "Primary period: " + err.Error()
		return m, nil
	}
	secondary, err := library.ParseDateRange(strings.TrimSpace(m.lastfmMixDraft[1]))
	if err != nil {
		m.status = "Secondary period: " + err.Error()
		return m, nil
	}
	percent, err := strconv.Atoi(m.lastfmMixDraft[2])
	if err != nil || percent < 0 || percent > 100 {
		m.status = "Secondary percentage must be 0 through 100"
		return m, nil
	}
	count, err := strconv.Atoi(m.lastfmMixDraft[3])
	if err != nil || count <= 0 {
		m.status = "Track count must be positive"
		return m, nil
	}
	if secondary == nil {
		percent = 0
	}
	queuedTracks := map[string]bool{}
	for _, id := range m.queueOrder {
		if v, ok := m.queued[id]; ok {
			queuedTracks[v.TrackID] = true
		}
	}
	result, err := m.lastfm.BuildMix(lastfm.MixRequest{Tracks: m.filtered, Query: m.currentQuery(), Primary: primary, Secondary: secondary, SecondaryPercent: percent, Count: count, Method: m.lastfmMixMethod, Action: m.lastfmQueueAction, QueuedTrackIDs: queuedTracks, SelectionStrategy: m.playbackOptions.SelectionStrategy})
	if err != nil {
		m.status = "Period mix failed: " + err.Error()
		return m, nil
	}
	if m.lastfmQueueAction == lastfm.ReplaceQueue {
		m.queued = map[string]library.Variant{}
		m.queueOrder = nil
	}
	for _, v := range result.Variants {
		if m.queuedID(v.ID) == "" {
			m.queued[v.ID] = v
			m.queueOrder = append(m.queueOrder, v.ID)
		}
	}
	m.mode = modeLastFM
	m.overlayCursor = 2
	m.status = fmt.Sprintf("Created %d of %d requested period-mix tracks", result.Created, result.Requested)
	return m, nil
}
