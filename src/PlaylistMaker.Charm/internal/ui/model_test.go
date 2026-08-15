package ui

import (
	"context"
	"errors"
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"

	"playlistmaker/charm/internal/backend"
	"playlistmaker/charm/internal/library"
	"playlistmaker/charm/internal/updater"
)

type playbackStub struct {
	result  backend.PlaybackResult
	err     error
	ids     []string
	request backend.PlaybackRequest
}

type historyStub struct {
	tracks []library.Track
	err    error
	calls  int
}

type historyWatcherStub struct {
	changes chan struct{}
	closes  int
}

type mappingUpdaterStub struct {
	items      []updater.Item
	candidates []updater.Audio
	tracks     []library.Track
	scanCalls  int
	confirms   int
}

func (s *mappingUpdaterStub) Scan(context.Context) ([]updater.Item, error) {
	s.scanCalls++
	return s.items, nil
}
func (s *mappingUpdaterStub) Search(context.Context, string) ([]updater.Audio, error) {
	return s.candidates, nil
}
func (s *mappingUpdaterStub) Confirm(string, string) error { s.confirms++; return nil }
func (s *mappingUpdaterStub) Reload(context.Context) ([]library.Track, PlaybackLauncher, error) {
	return s.tracks, nil, nil
}

func newHistoryWatcherStub() *historyWatcherStub {
	return &historyWatcherStub{changes: make(chan struct{}, 4)}
}

func (w *historyWatcherStub) Changes() <-chan struct{} { return w.changes }
func (w *historyWatcherStub) Close() error {
	w.closes++
	close(w.changes)
	return nil
}

func (s *historyStub) Refresh(_ context.Context) ([]library.Track, error) {
	s.calls++
	return s.tracks, s.err
}

func (p *playbackStub) Launch(_ context.Context, request backend.PlaybackRequest) (backend.PlaybackResult, error) {
	ids := request.VideoIDs
	p.ids = append([]string(nil), ids...)
	p.request = backend.PlaybackRequest{VideoIDs: append([]string(nil), request.VideoIDs...), Options: request.Options}
	return p.result, p.err
}

func TestSearchModeOwnsPrintableKeysAndSpace(t *testing.T) {
	m := New(library.Generate(50, 200))
	m = updateKey(t, m, "/")
	m = updateKey(t, m, "h")
	m = updateKey(t, m, "space")
	m = updateKey(t, m, "l")

	if m.query != "h l" {
		t.Fatalf("query = %q", m.query)
	}
	if len(m.queueOrder) != 0 {
		t.Fatalf("search input unexpectedly changed queue")
	}
	if len(m.expanded) != 0 {
		t.Fatalf("search input unexpectedly expanded a track")
	}
}

func TestSearchEditsMoveFocusToFirstResult(t *testing.T) {
	m := New(library.Generate(50, 200))
	m = updateKey(t, m, "/")
	for _, key := range []string{"a", "space", "backspace", "ctrl+u"} {
		m.cursor = min(3, max(len(m.rows)-1, 0))
		m = updateKey(t, m, key)
		if m.cursor != 0 {
			t.Fatalf("%s left cursor at %d", key, m.cursor)
		}
	}
	m = updateKey(t, m, "/")
	if m.mode != modeNavigate || m.query != "" {
		t.Fatal("slash did not close search while retaining its query")
	}
}

func TestOpeningKeysCloseEveryModeAndDiscardDrafts(t *testing.T) {
	for _, test := range []struct {
		open string
		mode mode
	}{
		{"/", modeSearch}, {"c", modeCategories}, {"s", modeSort}, {"f", modeFilters}, {"p", modePlaybackOptions}, {"q", modeQueue}, {"?", modeHelp}, {"d", modeDetails},
	} {
		t.Run(test.open, func(t *testing.T) {
			m := New(library.Generate(4, 8))
			m = updateKey(t, m, test.open)
			if m.mode != test.mode {
				t.Fatalf("%s mode = %s", test.open, m.mode)
			}
			if test.mode == modeFilters {
				m.filterDraft[0] = "2025"
			}
			if test.mode == modePlaybackOptions {
				m.draftOptions.Shuffle = true
			}
			m = updateKey(t, m, test.open)
			if m.mode != modeNavigate {
				t.Fatalf("%s did not close mode", test.open)
			}
			if test.mode == modeFilters && m.trackDate != nil {
				t.Fatal("filter close applied draft")
			}
			if test.mode == modePlaybackOptions && m.playbackOptions.Shuffle {
				t.Fatal("options close applied draft")
			}
		})
	}
}

func TestMappingUpdateModeScansOnDemandAndReloadsAfterSave(t *testing.T) {
	stub := &mappingUpdaterStub{items: []updater.Item{{VideoPath: "video", Filename: "video.mkv", Artist: "Artist", Title: "Title", AudioPath: "audio", Reason: "Exact cache match"}}, candidates: []updater.Audio{{Path: "manual", Artist: "Manual", Title: "Track"}}, tracks: library.Generate(2, 4)}
	m := New(library.Generate(1, 2)).WithMappingUpdater(stub)
	if stub.scanCalls != 0 {
		t.Fatal("mapping scan ran at startup")
	}
	next, command := m.Update(tea.KeyPressMsg{Code: 'u', Text: "u"})
	m = next.(Model)
	if command == nil || m.mode != modeMappingUpdate || !m.mappingScanning {
		t.Fatal("u did not open an async scan")
	}
	next, _ = m.Update(command())
	m = next.(Model)
	if stub.scanCalls != 1 || m.mappingScanning || len(m.mappingItems) != 1 {
		t.Fatal("scan result was not applied")
	}
	next, command = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	next, _ = next.(Model).Update(command())
	m = next.(Model)
	if stub.confirms != 1 || !m.mappingDirty || m.mappingIndex != 1 {
		t.Fatal("confirmation did not save and advance")
	}
	next, command = m.Update(tea.KeyPressMsg{Code: 'u', Text: "u"})
	if command == nil || next.(Model).mode != modeNavigate {
		t.Fatal("u did not close and reload")
	}
	next, _ = next.(Model).Update(command())
	if len(next.(Model).all) != 2 {
		t.Fatal("reload did not replace the catalogue")
	}
}

func TestEscapeNeverQuits(t *testing.T) {
	m := New(library.Generate(20, 50))
	next, command := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if command != nil {
		t.Fatal("escape returned a command")
	}
	if next.(Model).mode != modeNavigate {
		t.Fatal("escape left navigation mode")
	}
}

func TestQueueSurvivesSearch(t *testing.T) {
	m := New(library.Generate(50, 200))
	m = updateKey(t, m, "space")
	if len(m.queueOrder) != 1 {
		t.Fatal("track was not queued")
	}
	m = updateKey(t, m, "/")
	m = updateKey(t, m, "a")
	m = updateKey(t, m, "e")
	m = updateKey(t, m, "s")
	m = updateKey(t, m, "enter")
	if len(m.queueOrder) != 1 {
		t.Fatal("search changed queue")
	}
}

func TestSortEnterAppliesSelectionAndClosesOverlay(t *testing.T) {
	m := New(library.Generate(50, 200))
	m.cursor = 20
	m = updateKey(t, m, "s")
	m = updateKey(t, m, "j")
	m = updateKey(t, m, "j")
	m = updateKey(t, m, "enter")

	if m.sort != library.ArtistAscending {
		t.Fatalf("sort = %s", m.sort)
	}
	if m.mode != modeNavigate {
		t.Fatalf("mode = %s", m.mode)
	}
	if m.cursor != 0 {
		t.Fatalf("cursor = %d, want top of newly sorted list", m.cursor)
	}
}

func TestCategoryChangesApplyImmediately(t *testing.T) {
	m := New(library.Generate(50, 200))
	m = updateKey(t, m, "c")
	m = updateKey(t, m, "j")
	m = updateKey(t, m, "space")

	if !m.enabled[library.BandLive] {
		t.Fatal("Band Live was not enabled")
	}
	if m.mode != modeCategories {
		t.Fatal("category overlay unexpectedly closed")
	}
}

func TestVimListJumps(t *testing.T) {
	m := New(library.Generate(100, 400))
	m.height = 30
	m = updateKey(t, m, "G")
	if m.cursor != len(m.rows)-1 {
		t.Fatalf("G cursor = %d", m.cursor)
	}
	m = updateKey(t, m, "g")
	m = updateKey(t, m, "g")
	if m.cursor != 0 {
		t.Fatalf("gg cursor = %d", m.cursor)
	}
}

func TestControlUDMoveOneVisiblePage(t *testing.T) {
	m := New(library.Generate(100, 400))
	m.height = 30
	m = updateKey(t, m, "ctrl+d")
	if m.cursor != 26 {
		t.Fatalf("ctrl+d cursor = %d, want 26", m.cursor)
	}
	m = updateKey(t, m, "ctrl+u")
	if m.cursor != 0 {
		t.Fatalf("ctrl+u cursor = %d, want 0", m.cursor)
	}
}

func TestOLaunchesAndClearsQueueAfterSuccess(t *testing.T) {
	launcher := &playbackStub{result: backend.PlaybackResult{Succeeded: true, PlannedVideoCount: 1}}
	m := New(library.Generate(50, 200), launcher)
	m = updateKey(t, m, "space")

	next, command := m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	m = next.(Model)
	if command == nil || !m.launching {
		t.Fatal("o did not start playback")
	}
	next, _ = m.Update(command())
	m = next.(Model)
	if len(launcher.ids) != 1 {
		t.Fatalf("launched %d videos", len(launcher.ids))
	}
	if len(m.queueOrder) != 0 {
		t.Fatal("successful playback did not clear queue")
	}
}

func TestFailedPlaybackKeepsQueue(t *testing.T) {
	launcher := &playbackStub{err: errors.New("mpv unavailable")}
	m := New(library.Generate(50, 200), launcher)
	m = updateKey(t, m, "space")
	next, command := m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	m = next.(Model)
	next, _ = m.Update(command())
	m = next.(Model)
	if len(m.queueOrder) != 1 {
		t.Fatal("failed playback cleared queue")
	}
}

func TestManualHistoryRefreshPreservesUIStateAndQueue(t *testing.T) {
	tracks := library.Generate(3, 6)
	fresh := append([]library.Track(nil), tracks...)
	for index := range fresh {
		fresh[index].Variants = append([]library.Variant(nil), fresh[index].Variants...)
		fresh[index].History.PlayedCount = 4
		for variant := range fresh[index].Variants {
			fresh[index].Variants[variant].History.CompletedCount = 2
		}
	}
	source := &historyStub{tracks: fresh}
	m := New(tracks).WithHistorySource(source)
	m.historyRefreshing = false // Simulate the startup refresh having completed.
	m.query, m.sort, m.expanded[tracks[0].ID] = "", library.TitleAscending, true
	m.refreshResults()
	m = updateKey(t, m, "space")
	queued := append([]string(nil), m.queueOrder...)
	cursor := m.cursor
	next, command := m.requestHistoryRefresh()
	if command == nil || !next.(Model).historyRefreshing {
		t.Fatal("R did not start async refresh")
	}
	next, _ = next.(Model).Update(command())
	m = next.(Model)
	if source.calls != 1 || m.all[0].History.PlayedCount != 4 || m.all[0].Variants[0].History.CompletedCount != 2 {
		t.Fatal("history was not applied")
	}
	if m.sort != library.TitleAscending || m.cursor != cursor || !m.expanded[tracks[0].ID] || !slices.Equal(m.queueOrder, queued) {
		t.Fatal("refresh changed UI state")
	}
	if m.queued[queued[0]].History.CompletedCount != 2 {
		t.Fatal("queued history was not refreshed")
	}
}

func TestHistoryRefreshKeepsHighlightedChildVariant(t *testing.T) {
	tracks := library.Generate(1, 3)
	for index := range tracks[0].Variants {
		tracks[0].Variants[index].Category = library.MusicVideo
	}
	m := New(tracks)
	m.expanded[tracks[0].ID] = true
	m.refreshResults()
	m.cursor = 2
	want := m.filtered[m.rows[m.cursor].trackIndex].Variants[m.rows[m.cursor].variantIndex].ID

	fresh := cloneTracks(tracks)
	fresh[0].Variants[1].History.PlayedCount = 3
	m.applyHistory(fresh)
	current, ok := m.currentRow()
	if !ok || !current.isVariant() {
		t.Fatal("highlighted child was not restored as a child row")
	}
	got := m.filtered[current.trackIndex].Variants[current.variantIndex].ID
	if got != want {
		t.Fatalf("highlighted variant = %q, want %q", got, want)
	}
}

func TestHistoryRefreshKeepsHighlightedParent(t *testing.T) {
	tracks := library.Generate(2, 4)
	m := New(tracks)
	m.cursor = 1
	want := m.filtered[m.rows[m.cursor].trackIndex].ID
	m.applyHistory(cloneTracks(tracks))
	current, ok := m.currentRow()
	if !ok || current.isVariant() || m.filtered[current.trackIndex].ID != want {
		t.Fatal("highlighted parent was not preserved")
	}
}

func TestHistoryRefreshFallsBackToParentWhenChildIsIneligible(t *testing.T) {
	tracks := library.Generate(1, 3)
	for index := range tracks[0].Variants {
		tracks[0].Variants[index].Category = library.MusicVideo
	}
	tracks[0].Variants[1].Category = library.BandLive
	m := New(tracks)
	m.enabled[library.BandLive] = true
	m.expanded[tracks[0].ID] = true
	m.refreshResults()
	m.cursor = 2
	m.enabled[library.BandLive] = false
	m.applyHistory(cloneTracks(m.all))
	current, ok := m.currentRow()
	if !ok || current.isVariant() || m.filtered[current.trackIndex].ID != tracks[0].ID {
		t.Fatal("ineligible child did not fall back to its parent")
	}
}

func TestAutomaticHistoryRefreshPreservesStateAndStatus(t *testing.T) {
	tracks := library.Generate(3, 6)
	m := New(tracks)
	m.query = tracks[0].Artist
	m.enabled[library.BandLive] = true
	m.sort = library.TitleAscending
	m.expanded[tracks[0].ID] = true
	m.refreshResults()
	m = updateKey(t, m, "space")
	m.playbackOptions.Shuffle = true
	m.mode, m.detailsOffset = modeDetails, 1
	m.height = 12
	m.status = "Queued a video"
	queued := append([]string(nil), m.queueOrder...)

	fresh := cloneTracks(m.all)
	fresh[0].History.PlayedCount = 8
	next, _ := m.Update(historyRefreshMsg{tracks: fresh})
	m = next.(Model)
	if m.status != "Queued a video" || m.query != tracks[0].Artist || !m.enabled[library.BandLive] || m.sort != library.TitleAscending || !m.expanded[tracks[0].ID] || !slices.Equal(m.queueOrder, queued) || !m.playbackOptions.Shuffle || m.mode != modeDetails || m.detailsOffset != 1 {
		t.Fatal("automatic history refresh reset UI state or status")
	}
}

func TestHistoryWatchNotificationsCoalesceWhileRefreshIsRunning(t *testing.T) {
	tracks := library.Generate(1, 2)
	source := &historyStub{tracks: cloneTracks(tracks)}
	watcher := newHistoryWatcherStub()
	m := New(tracks).WithHistorySource(source, watcher)
	m.historyRefreshing = true
	watcher.changes <- struct{}{}
	message := m.waitForHistoryChangeCmd()()
	next, command := m.Update(message)
	m = next.(Model)
	if command == nil || !m.historyPending {
		t.Fatal("watch notification was not retained while a refresh was running")
	}
	next, command = m.Update(historyRefreshMsg{tracks: cloneTracks(tracks)})
	m = next.(Model)
	if command == nil || !m.historyRefreshing || m.historyPending {
		t.Fatal("coalesced notification did not schedule exactly one follow-up refresh")
	}
	message = command()
	next, _ = m.Update(message)
	if source.calls != 1 {
		t.Fatalf("follow-up refresh calls = %d, want 1", source.calls)
	}
}

func TestManualHistoryRefreshReportsFailureAndWatcherCloses(t *testing.T) {
	tracks := library.Generate(1, 2)
	source := &historyStub{err: errors.New("history unavailable")}
	watcher := newHistoryWatcherStub()
	m := New(tracks).WithHistorySource(source, watcher)
	m.historyRefreshing = false // Simulate the startup refresh having completed.
	next, command := m.requestHistoryRefresh()
	if command == nil {
		t.Fatal("manual R did not start refresh")
	}
	next, _ = next.(Model).Update(command())
	m = next.(Model)
	if m.status != "History refresh failed: history unavailable" {
		t.Fatalf("manual failure status = %q", m.status)
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: 'q', Mod: tea.ModCtrl})
	if next.(Model).historyWatcher != nil || watcher.closes != 1 {
		t.Fatal("history watcher was not closed on TUI quit")
	}
}

func cloneTracks(tracks []library.Track) []library.Track {
	result := append([]library.Track(nil), tracks...)
	for track := range result {
		result[track].Variants = append([]library.Variant(nil), result[track].Variants...)
	}
	return result
}

func TestHelpOpensScrollsAndClosesWithoutTriggeringActions(t *testing.T) {
	m := New(library.Generate(50, 200))
	m = updateKey(t, m, "?")
	if m.mode != modeHelp {
		t.Fatalf("mode = %s", m.mode)
	}
	m.height = 12
	m = updateKey(t, m, "j")
	if m.helpOffset == 0 {
		t.Fatal("help did not scroll")
	}
	m = updateKey(t, m, "a")
	if len(m.queueOrder) != 0 {
		t.Fatal("help action leaked into queue")
	}
	m = updateKey(t, m, "?")
	if m.mode != modeNavigate {
		t.Fatalf("mode = %s", m.mode)
	}
}

func BenchmarkNavigationAndRenderParityScale(b *testing.B) {
	m := New(library.Generate(1337, 6420))
	m.width = 160
	m.height = 48
	down := tea.KeyPressMsg{Code: 'j', Text: "j"}
	b.ResetTimer()
	for range b.N {
		next, _ := m.Update(down)
		m = next.(Model)
		_ = m.View()
	}
}

func updateKey(t *testing.T, m Model, key string) Model {
	t.Helper()
	message := tea.KeyPressMsg{}
	switch key {
	case "space":
		message = tea.KeyPressMsg{Code: ' ', Text: " "}
	case "enter":
		message = tea.KeyPressMsg{Code: tea.KeyEnter}
	case "backspace":
		message = tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "delete":
		message = tea.KeyPressMsg{Code: tea.KeyDelete}
	case "up":
		message = tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		message = tea.KeyPressMsg{Code: tea.KeyDown}
	case "left":
		message = tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		message = tea.KeyPressMsg{Code: tea.KeyRight}
	case "pgup":
		message = tea.KeyPressMsg{Code: tea.KeyPgUp}
	case "pgdown":
		message = tea.KeyPressMsg{Code: tea.KeyPgDown}
	case "ctrl+d":
		message = tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl}
	case "ctrl+u":
		message = tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl}
	default:
		message = tea.KeyPressMsg{Code: rune(key[0]), Text: key}
	}
	next, _ := m.Update(message)
	return next.(Model)
}
