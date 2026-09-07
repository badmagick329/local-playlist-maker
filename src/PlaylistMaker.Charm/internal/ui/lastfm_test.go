package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"playlistmaker/charm/internal/lastfm"
	"playlistmaker/charm/internal/library"
)

type lastfmStub struct {
	status  lastfm.Status
	mix     lastfm.MixResult
	resets  int
	request lastfm.MixRequest
}

func (s *lastfmStub) Status() lastfm.Status { return s.status }
func (s *lastfmStub) Sync(context.Context, []library.Track, bool, func(lastfm.SyncProgress)) (lastfm.SyncResult, error) {
	return lastfm.SyncResult{}, nil
}
func (s *lastfmStub) Attach(v []library.Track) []library.Track { return v }
func (s *lastfmStub) BuildMix(r lastfm.MixRequest) (lastfm.MixResult, error) {
	s.request = r
	return s.mix, nil
}
func (s *lastfmStub) ExportReview([]library.Track, time.Time) (string, error) {
	return `C:\data\lastfm-review`, nil
}
func (s *lastfmStub) ImportDecisions([]library.Track) (lastfm.ImportResult, error) {
	return lastfm.ImportResult{Matched: 1}, nil
}
func (s *lastfmStub) ResetAgentDecisions([]library.Track) error { s.resets++; return nil }

func TestUppercaseLOpensLastFMAndLowercaseLRetainsExpansion(t *testing.T) {
	stub := &lastfmStub{status: lastfm.Status{Configured: true, Scrobbles: 12, Matched: 2, Unresolved: 1, CheckpointPages: 289, CheckpointTotal: 290}}
	m := New(library.Generate(2, 2)).WithLastFM(stub)
	m = updateKey(t, m, "l")
	if m.mode != modeNavigate || len(m.expanded) == 0 {
		t.Fatal("lowercase l no longer expands")
	}
	m = updateKey(t, m, "L")
	if m.mode != modeLastFM {
		t.Fatalf("mode=%v", m.mode)
	}
	rendered := stripStyles(m.render())
	if !containsAll(rendered, "Cached scrobbles: 12", "Matched identities: 2", "Unresolved identities: 1", "Saved sync checkpoint: page 289 of 290") {
		t.Fatalf("render=%s", rendered)
	}
	m = updateKey(t, m, "esc")
	if m.mode != modeNavigate {
		t.Fatal("Last.fm screen did not close")
	}
}

func TestPeriodMixUsesSharedPanelAndAppendsQueue(t *testing.T) {
	tracks := library.Generate(2, 2)
	stub := &lastfmStub{mix: lastfm.MixResult{Variants: []library.Variant{tracks[1].Variants[0]}, Requested: 20, Created: 1}}
	m := New(tracks).WithLastFM(stub)
	m = updateKey(t, m, "space")
	m = updateKey(t, m, "p")
	for range 3 {
		m = updateKey(t, m, "right")
	}
	if m.draftMix != 3 || m.mode != modePlaybackOptions {
		t.Fatal("period mix not in playback panel")
	}
	m.periodDraft[0] = "invalid"
	m.overlayCursor = 11
	m = updateKey(t, m, "enter")
	if len(m.queueOrder) != 1 || !strings.Contains(m.status, "Primary period") {
		t.Fatal("invalid period mutated queue")
	}
	m.periodDraft[0] = "2025"
	m = updateKey(t, m, "enter")
	if len(m.queueOrder) != 2 || m.queueOrder[1] != tracks[1].Variants[0].ID {
		t.Fatal("mix did not append")
	}
	m = updateKey(t, m, "p")
	if m.periodDraft[0] != "2025" || m.draftMix != 3 {
		t.Fatal("mix settings not remembered")
	}
}

func TestPeriodNavigationDoesNotEditDates(t *testing.T) {
	m := New(library.Generate(1, 1))
	m.openPlaybackPanel()
	m.draftMix = 3
	m.overlayCursor = 6
	m = updateKey(t, m, "j")
	m = updateKey(t, m, "k")
	if m.overlayCursor != 6 || m.periodDraft[0] != "" || m.periodDraft[1] != "" {
		t.Fatal("navigation edited dates")
	}
	model, _ := m.handlePlaybackPanelKey(tea.KeyPressMsg{Text: "2025-01..2025-03"})
	if model.(Model).periodDraft[0] != "2025-01..2025-03" {
		t.Fatal("date input lost")
	}
}

func TestLastFMResetRequiresSecondConfirmation(t *testing.T) {
	stub := &lastfmStub{status: lastfm.Status{Configured: true}}
	m := New(library.Generate(1, 1)).WithLastFM(stub)
	m = updateKey(t, m, "L")
	m.overlayCursor = 4
	model, cmd := m.handleLastFMKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = model.(Model)
	if cmd != nil || !m.lastfmResetArmed || stub.resets != 0 {
		t.Fatal("first Enter reset decisions")
	}
	model, cmd = m.handleLastFMKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = model.(Model)
	if cmd == nil {
		t.Fatal("second Enter did not schedule reset")
	}
	msg := cmd()
	updated, _ := m.Update(msg)
	m = updated.(Model)
	if stub.resets != 1 {
		t.Fatalf("resets=%d", stub.resets)
	}
}

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}
