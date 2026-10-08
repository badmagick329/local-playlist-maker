package ui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"playlistmaker/charm/internal/lastfm"
	"playlistmaker/charm/internal/library"
)

type lastfmStub struct {
	status     lastfm.Status
	mix        lastfm.MixResult
	resets     int
	request    lastfm.MixRequest
	sync       lastfm.SyncResult
	syncErr    error
	lastSync   *lastfm.SyncReport
	exports    int
	exportedIn *library.DateRange
}

func (s *lastfmStub) Status() lastfm.Status { return s.status }
func (s *lastfmStub) Sync(context.Context, []library.Track, bool, func(lastfm.SyncProgress)) (lastfm.SyncResult, error) {
	return s.sync, s.syncErr
}
func (s *lastfmStub) Attach(v []library.Track) []library.Track { return v }
func (s *lastfmStub) BuildMix(r lastfm.MixRequest) (lastfm.MixResult, error) {
	s.request = r
	return s.mix, nil
}
func (s *lastfmStub) ExportReview(_ []library.Track, _ time.Time, period *library.DateRange) (string, error) {
	s.exports++
	s.exportedIn = period
	return `C:\data\lastfm-review`, nil
}
func (s *lastfmStub) LastSyncReport([]library.Track) (lastfm.SyncReport, bool) {
	if s.lastSync == nil {
		return lastfm.SyncReport{}, false
	}
	return *s.lastSync, true
}
func (s *lastfmStub) LastSyncPeriod() (*library.DateRange, error) {
	if s.lastSync == nil {
		return nil, errors.New("no Last.fm sync is recorded yet")
	}
	return &library.DateRange{Label: "since last sync"}, nil
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
	m.overlayCursor = lastfmResetRow
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

func TestSyncOpensReportOfAddedScrobblesWithoutSpotifyLink(t *testing.T) {
	after := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	report := lastfm.SyncReport{SyncedAtUTC: time.Date(2026, 10, 8, 15, 0, 0, 0, time.UTC), AfterUTC: &after, Added: 31, Linked: 1,
		Unresolved:    []lastfm.ReportSong{{Artist: "Lovelyz", Title: "WoW!", Scrobbles: 22}},
		NoMatch:       []lastfm.ReportSong{},
		NoSpotifyLink: []lastfm.ReportSong{{Artist: "Kep1er", Title: "Shooting Star", Scrobbles: 8, TrackID: "t", SpotifyIgnored: true}}}
	for _, c := range []struct {
		name string
		err  error
	}{{"saved", nil}, {"report not saved", lastfm.ErrSyncLogNotSaved}} {
		t.Run(c.name, func(t *testing.T) {
			stub := &lastfmStub{status: lastfm.Status{Configured: true}}
			m := New(library.Generate(1, 1)).WithLastFM(stub)
			m = updateKey(t, m, "L")
			model, _ := m.handleLastFMSync(lastfmSyncMsg{result: lastfm.SyncResult{Report: report}, err: c.err})
			m = model.(Model)
			if m.lastfmReport == nil {
				t.Fatal("sync did not open its report")
			}
			rendered := stripStyles(m.render())
			if !containsAll(rendered, "Scrobbles after 2026-09-08 12:00 UTC: 31", "Lovelyz – WoW! • 22", "Kep1er – Shooting Star • 8 • Spotify ignored") {
				t.Fatalf("render=%s", rendered)
			}
			want := "added 31 scrobbles: 1 with a Spotify link, 22 unresolved, 0 no match, 8 without a Spotify link"
			if c.err != nil {
				want = "was not saved"
			}
			if !strings.Contains(m.status, want) {
				t.Fatalf("status=%q", m.status)
			}
			m = updateKey(t, m, "esc")
			if m.lastfmReport != nil || m.mode != modeLastFM {
				t.Fatal("Esc did not return to the Last.fm actions")
			}
		})
	}
}

func TestLastSyncReportReopensAndClosesWithL(t *testing.T) {
	stub := &lastfmStub{status: lastfm.Status{Configured: true}}
	m := New(library.Generate(1, 1)).WithLastFM(stub)
	m = updateKey(t, m, "L")
	m.overlayCursor = lastfmReportRow
	m = updateKey(t, m, "enter")
	if m.lastfmReport != nil || !strings.Contains(m.status, "No Last.fm sync") {
		t.Fatalf("report opened without a recorded sync: %q", m.status)
	}
	stub.lastSync = &lastfm.SyncReport{SyncedAtUTC: time.Unix(10, 0)}
	m = updateKey(t, m, "enter")
	if m.lastfmReport == nil {
		t.Fatal("report action did not open the report")
	}
	m = updateKey(t, m, "L")
	if m.mode != modeNavigate || m.lastfmReport != nil {
		t.Fatal("L did not close the report")
	}
}

func TestExportScopeCyclesAndLimitsThePeriod(t *testing.T) {
	stub := &lastfmStub{status: lastfm.Status{Configured: true}}
	m := New(library.Generate(1, 1)).WithLastFM(stub)
	m = updateKey(t, m, "L")
	for range lastfmExportRow {
		m = updateKey(t, m, "j")
	}
	m = updateKey(t, m, "l")
	m = updateKey(t, m, "enter")
	if stub.exports != 0 || !strings.Contains(m.status, "no Last.fm sync") {
		t.Fatalf("since-last-sync export ran without a recorded sync: %q", m.status)
	}
	m = updateKey(t, m, "l")
	m = updateKey(t, m, "enter")
	if stub.exports != 0 || !strings.Contains(m.status, "type a date range") {
		t.Fatalf("blank range exported: %q", m.status)
	}
	m = updateKey(t, m, "2026-09-08..2026-10x")
	m = updateKey(t, m, "backspace")
	m = updateKey(t, m, "0")
	m = enterAndRun(t, m)
	if stub.exports != 1 || stub.exportedIn == nil || stub.exportedIn.Label != "2026-09-08..2026-10" {
		t.Fatalf("range export period=%#v", stub.exportedIn)
	}
	m = updateKey(t, m, "h")
	m = updateKey(t, m, "h")
	m = enterAndRun(t, m)
	if stub.exports != 2 || stub.exportedIn != nil {
		t.Fatalf("all export period=%#v", stub.exportedIn)
	}
}

// enterAndRun presses Enter and delivers the command it schedules.
func enterAndRun(t *testing.T, m Model) Model {
	t.Helper()
	model, cmd := m.handleLastFMKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("no command scheduled")
	}
	next, _ := model.(Model).Update(cmd())
	return next.(Model)
}

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}
