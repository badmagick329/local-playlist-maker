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
	status lastfm.Status
	mix    lastfm.MixResult
	resets int
}

func (s *lastfmStub) Status() lastfm.Status { return s.status }
func (s *lastfmStub) Sync(context.Context, []library.Track, bool, func(lastfm.SyncProgress)) (lastfm.SyncResult, error) {
	return lastfm.SyncResult{}, nil
}
func (s *lastfmStub) Attach(v []library.Track) []library.Track             { return v }
func (s *lastfmStub) BuildMix(lastfm.MixRequest) (lastfm.MixResult, error) { return s.mix, nil }
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

func TestLastFMMixBuilderReplacesQueueAndValidatesFields(t *testing.T) {
	tracks := library.Generate(2, 2)
	variant := tracks[1].Variants[0]
	stub := &lastfmStub{status: lastfm.Status{Configured: true}, mix: lastfm.MixResult{Variants: []library.Variant{variant}, Requested: 10, Created: 1}}
	m := New(tracks).WithLastFM(stub)
	m = updateKey(t, m, "space")
	m = updateKey(t, m, "L")
	m.overlayCursor = 2
	m = updateKey(t, m, "enter")
	if m.mode != modeLastFMMix {
		t.Fatal("mix builder did not open")
	}
	m.overlayCursor = 6
	m = updateKey(t, m, "enter")
	if m.mode != modeLastFM || len(m.queueOrder) != 1 || m.queueOrder[0] != variant.ID {
		t.Fatalf("queue=%#v mode=%v", m.queueOrder, m.mode)
	}
}

func TestLastFMMixNavigationKeysDoNotLeakIntoDateFields(t *testing.T) {
	m := Model{mode: modeLastFMMix, overlayCursor: 0, lastfmMixDraft: [4]string{"", "", "20", "10"}}

	model, _ := m.handleLastFMMixKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = model.(Model)
	if m.overlayCursor != 1 || m.lastfmMixDraft[0] != "" || m.lastfmMixDraft[1] != "" {
		t.Fatalf("down navigation leaked into date fields: cursor=%d draft=%q", m.overlayCursor, m.lastfmMixDraft)
	}

	model, _ = m.handleLastFMMixKey(tea.KeyPressMsg{Code: 'k', Text: "k"})
	m = model.(Model)
	if m.overlayCursor != 0 || m.lastfmMixDraft[0] != "" || m.lastfmMixDraft[1] != "" {
		t.Fatalf("up navigation leaked into date fields: cursor=%d draft=%q", m.overlayCursor, m.lastfmMixDraft)
	}

	for _, key := range []string{"2025-01..2025-03", "h", "l"} {
		model, _ = m.handleLastFMMixKey(tea.KeyPressMsg{Code: rune(key[0]), Text: key})
		m = model.(Model)
	}
	if m.lastfmMixDraft[0] != "2025-01..2025-03" {
		t.Fatalf("date input filtering changed valid input: draft=%q", m.lastfmMixDraft[0])
	}
}

func TestLastFMResetRequiresSecondConfirmation(t *testing.T) {
	stub := &lastfmStub{status: lastfm.Status{Configured: true}}
	m := New(library.Generate(1, 1)).WithLastFM(stub)
	m = updateKey(t, m, "L")
	m.overlayCursor = 5
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
