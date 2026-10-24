package ui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

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
	imports    int
	// fulls records each Sync's full flag. Sync runs on its own goroutine,
	// so read it only after receiving that sync's lastfmSyncMsg.
	fulls []bool
}

func (s *lastfmStub) Status() lastfm.Status { return s.status }
func (s *lastfmStub) Sync(_ context.Context, _ []library.Track, full bool, _ func(lastfm.SyncProgress)) (lastfm.SyncResult, error) {
	s.fulls = append(s.fulls, full)
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
	s.imports++
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

func TestLastFMLettersMoveToAndRunTheirRows(t *testing.T) {
	stub := &lastfmStub{status: lastfm.Status{Configured: true}, lastSync: &lastfm.SyncReport{SyncedAtUTC: time.Unix(10, 0)}}
	open := func() Model { return updateKey(t, New(library.Generate(1, 1)).WithLastFM(stub), "L") }
	// Shift and Caps Lock read as the plain letter, so S syncs new plays and
	// never starts a full rebuild.
	for _, test := range []struct {
		text string
		row  int
		full bool
	}{{"s", lastfmSyncRow, false}, {"S", lastfmSyncRow, false}, {"b", lastfmRebuildRow, true}, {"B", lastfmRebuildRow, true}} {
		m, cmd := pressLetter(t, open(), test.text)
		if !m.lastfmRunning || m.overlayCursor != test.row || cmd == nil {
			t.Fatalf("%s: running %v, cursor %d", test.text, m.lastfmRunning, m.overlayCursor)
		}
		if _, ok := cmd().(lastfmSyncMsg); !ok {
			t.Fatalf("%s: sync did not finish", test.text)
		}
		if got := stub.fulls[len(stub.fulls)-1]; got != test.full {
			t.Fatalf("%s: full=%v", test.text, got)
		}
	}
	m, cmd := pressLetter(t, open(), "r")
	if m.lastfmReport == nil || m.overlayCursor != lastfmReportRow || cmd != nil || m.mode != modeLastFM {
		t.Fatalf("r: report %v, cursor %d", m.lastfmReport, m.overlayCursor)
	}
	// e exports with the scope already chosen, as Enter on the row would.
	m = open()
	m.lastfmExportScope = exportSinceLastSync
	m = letterAndRun(t, m, "e")
	if stub.exports != 1 || stub.exportedIn == nil || stub.exportedIn.Label != "since last sync" || m.overlayCursor != lastfmExportRow || m.mode != modeLastFM {
		t.Fatalf("e: exports %d, period %#v, cursor %d", stub.exports, stub.exportedIn, m.overlayCursor)
	}
	m = letterAndRun(t, open(), "i")
	if stub.imports != 1 || m.overlayCursor != lastfmImportRow || !strings.Contains(m.status, "1 matched") {
		t.Fatalf("i: imports %d, cursor %d, status %q", stub.imports, m.overlayCursor, m.status)
	}
	for _, mod := range []tea.KeyMod{tea.ModCtrl, tea.ModAlt} {
		next, cmd := open().Update(tea.KeyPressMsg{Code: 'b', Mod: mod, Text: "b"})
		if got := next.(Model); got.lastfmRunning || got.overlayCursor != 0 || cmd != nil {
			t.Fatalf("mod %v b ran its row", mod)
		}
	}
}

func TestLastFMSyncLettersReportUnconfiguredLastFMLikeEnter(t *testing.T) {
	m := updateKey(t, New(library.Generate(1, 1)).WithLastFM(&lastfmStub{}), "L")
	for _, text := range []string{"s", "b"} {
		next, cmd := pressLetter(t, m, text)
		if next.lastfmRunning || cmd != nil || next.status != "Last.fm sync is disabled because username and API key are not configured" {
			t.Fatalf("%s: running %v, status %q", text, next.lastfmRunning, next.status)
		}
	}
}

func TestLastFMResetLetterNeedsSecondPressAndOtherKeysDisarm(t *testing.T) {
	stub := &lastfmStub{status: lastfm.Status{Configured: true}}
	m := updateKey(t, New(library.Generate(1, 1)).WithLastFM(stub), "L")
	// Each key after the first x must disarm, so the following x arms again
	// instead of resetting. q names no row; r and j leave or stay on the row.
	for _, disarm := range []string{"q", "r", "j"} {
		next, cmd := pressLetter(t, m, "x")
		if cmd != nil || !next.lastfmResetArmed || next.overlayCursor != lastfmResetRow || !strings.Contains(next.status, "Press x or Enter again") {
			t.Fatalf("x before %s: armed %v, cursor %d, status %q", disarm, next.lastfmResetArmed, next.overlayCursor, next.status)
		}
		m = updateKey(t, next, disarm)
		if m.lastfmResetArmed || stub.resets != 0 {
			t.Fatalf("%s left the reset armed", disarm)
		}
	}
	m, _ = pressLetter(t, m, "x")
	m = letterAndRun(t, m, "X")
	if stub.resets != 1 || m.lastfmResetArmed {
		t.Fatalf("second x: resets %d, armed %v", stub.resets, m.lastfmResetArmed)
	}
	m, _ = pressLetter(t, m, "x")
	m = enterAndRun(t, m)
	if stub.resets != 2 {
		t.Fatalf("x then Enter: resets %d", stub.resets)
	}
}

func TestLastFMLettersIgnoredWhileSyncRunsAndInReport(t *testing.T) {
	stub := &lastfmStub{status: lastfm.Status{Configured: true}}
	opened := updateKey(t, New(library.Generate(1, 1)).WithLastFM(stub), "L")
	running := opened
	running.lastfmRunning = true
	report := opened
	report.lastfmReport = &lastfm.SyncReport{SyncedAtUTC: time.Unix(10, 0)}
	for _, letter := range lastfmRowKeys {
		for name, m := range map[string]Model{"running": running, "report": report} {
			next, cmd := pressLetter(t, m, string(letter))
			if cmd != nil || next.mode != modeLastFM || next.overlayCursor != 0 || next.lastfmResetArmed || next.lastfmRunning != m.lastfmRunning || next.lastfmReport != m.lastfmReport {
				t.Fatalf("%s: %c acted", name, letter)
			}
		}
	}
	if stub.exports != 0 || stub.imports != 0 || len(stub.fulls) != 0 {
		t.Fatal("a letter reached the service")
	}
}

func TestLastFMExportRangeKeepsDigitsWhileLettersRunRows(t *testing.T) {
	stub := &lastfmStub{status: lastfm.Status{Configured: true}}
	m := updateKey(t, New(library.Generate(1, 1)).WithLastFM(stub), "L")
	for range lastfmExportRow {
		m = updateKey(t, m, "j")
	}
	m = updateKey(t, m, "l")
	m = updateKey(t, m, "l")
	// q, Z and y name no row; space and uppercase are what dictation and
	// stray keys would otherwise leave in the range.
	for _, key := range []string{"2", "0", "q", "2", "space", "6", "Z", "-", "y", "0", "9"} {
		m = updateKey(t, m, key)
	}
	if m.lastfmExportRange != "2026-09" || m.overlayCursor != lastfmExportRow || stub.exports != 0 {
		t.Fatalf("range %q, cursor %d, exports %d", m.lastfmExportRange, m.overlayCursor, stub.exports)
	}
	m = letterAndRun(t, m, "e")
	if stub.exports != 1 || stub.exportedIn == nil || stub.exportedIn.Label != "2026-09" || m.lastfmExportRange != "2026-09" {
		t.Fatalf("e: period %#v, range %q", stub.exportedIn, m.lastfmExportRange)
	}
}

func TestLastFMRowKeysAvoidViewKeysAndDoNotRepeat(t *testing.T) {
	if len(lastfmRowKeys) != lastfmResetRow+1 {
		t.Fatalf("%d letters for %d rows", len(lastfmRowKeys), lastfmResetRow+1)
	}
	seen := map[rune]int{}
	for row, letter := range lastfmRowKeys {
		// l also covers L, which closes the screen.
		if letter < 'a' || letter > 'z' || strings.ContainsRune("jkhl", letter) {
			t.Fatalf("row %d letter %q is not a free lowercase letter", row, letter)
		}
		if other, dup := seen[letter]; dup {
			t.Fatalf("rows %d and %d share %q", row, other, letter)
		}
		seen[letter] = row
	}
}

func TestLastFMOverlayShowsLettersAndFittingFooterInBothLayouts(t *testing.T) {
	m := updateKey(t, New(library.Generate(1, 1)).WithLastFM(&lastfmStub{status: lastfm.Status{Configured: true}}), "L")
	for _, height := range []int{24, 14} {
		for _, size := range []struct {
			width int
			want  string
		}{{40, "letter run • L close"}, {80, "letter run • h/l export scope • L/esc close"}, {120, "j/k move • letter run • h/l export scope • enter activate • L/esc close"}} {
			content := stripStyles(m.renderOverlay("", size.width, height))
			// At width 40 the longer labels are truncated after the letter.
			for _, row := range []string{"› s Sync new plays", "  b Rebuild full", "  r Last sync report", "  e Export unresolved", "  i Import agent", "  x Reset agent", size.want} {
				if !strings.Contains(content, row) {
					t.Fatalf("height %d width %d overlay missing %q: %q", height, size.width, row, content)
				}
			}
			for _, line := range strings.Split(content, "\n") {
				if ansi.StringWidth(line) > size.width {
					t.Fatalf("height %d: overlay line exceeds width %d: %q", height, size.width, line)
				}
			}
			if hint := footerHint(modeLastFM, size.width); ansi.StringWidth(hint) > size.width || !strings.Contains(hint, "letter run") {
				t.Fatalf("width %d footer hint = %q", size.width, hint)
			}
		}
	}
}

// pressLetter delivers a letter as Bubble Tea does, with Shift and the
// uppercase text for an uppercase letter, and returns the scheduled command.
func pressLetter(t *testing.T, m Model, text string) (Model, tea.Cmd) {
	t.Helper()
	message := tea.KeyPressMsg{Code: []rune(strings.ToLower(text))[0], Text: text}
	if text != strings.ToLower(text) {
		message.Mod = tea.ModShift
	}
	next, cmd := m.Update(message)
	return next.(Model), cmd
}

// letterAndRun presses a letter and delivers the command it schedules.
func letterAndRun(t *testing.T, m Model, text string) Model {
	t.Helper()
	m, cmd := pressLetter(t, m, text)
	if cmd == nil {
		t.Fatalf("%s scheduled no command", text)
	}
	next, _ := m.Update(cmd())
	return next.(Model)
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
