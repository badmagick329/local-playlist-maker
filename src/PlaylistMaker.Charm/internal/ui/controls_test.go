package ui

import (
	"errors"
	"math"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"playlistmaker/charm/internal/backend"
	"playlistmaker/charm/internal/library"
)

func TestPlannedCountAppliesQueueRulesInOrder(t *testing.T) {
	variants := map[string]library.Variant{
		"a": {ID: "a", AudioPath: `C:\\Music\\Alpha.flac`},
		"b": {ID: "b", AudioPath: `c:/music/alpha.flac`},
		"c": {ID: "c", AudioPath: `C:\\Music\\Bravo.flac`},
	}
	queue := []string{"a", "b", "c", "missing"}
	tests := []struct {
		name    string
		options backend.PlaybackOptions
		want    int
	}{
		{"empty", backend.DefaultPlaybackOptions(), 0},
		{"ordinary excludes missing", backend.DefaultPlaybackOptions(), 3},
		{"repeat one", backend.PlaybackOptions{RepeatEach: 1}, 3},
		{"repeat ten", backend.PlaybackOptions{RepeatEach: 10}, 30},
		{"unlimited maximum", backend.PlaybackOptions{RepeatEach: 10, MaximumItems: 0}, 30},
		{"maximum below plan", backend.PlaybackOptions{RepeatEach: 10, MaximumItems: 2}, 2},
		{"maximum equal plan", backend.PlaybackOptions{RepeatEach: 1, MaximumItems: 3}, 3},
		{"maximum above plan", backend.PlaybackOptions{RepeatEach: 1, MaximumItems: 9}, 3},
		{"one per normalized audio", backend.PlaybackOptions{RepeatEach: 1, OneVideoPerTrack: true}, 2},
		{"one per track then repeat then maximum", backend.PlaybackOptions{Shuffle: true, RepeatEach: 3, OneVideoPerTrack: true, MaximumItems: 4}, 4},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actualQueue := queue
			if test.name == "empty" {
				actualQueue = nil
			}
			if got := plannedCount(actualQueue, variants, test.options); got != test.want {
				t.Fatalf("plannedCount() = %d, want %d", got, test.want)
			}
		})
	}
	if got := saturatingMultiply(math.MaxInt, 2); got != math.MaxInt {
		t.Fatalf("overflow multiplication = %d, want MaxInt", got)
	}
}

func TestPlaybackOptionsEditSaveCancelAndPreview(t *testing.T) {
	m := New(library.Generate(4, 8))
	m = updateKey(t, m, "A")
	m = updateKey(t, m, "p")
	if m.mode != modePlaybackOptions || m.draftOptions != m.playbackOptions {
		t.Fatal("options did not open from saved values")
	}
	m = updateKey(t, m, "j")
	m = updateKey(t, m, "space")
	if !m.draftOptions.OneVideoPerTrack {
		t.Fatal("space did not toggle one-video-per-track")
	}
	m = updateKey(t, m, "j")
	m = updateKey(t, m, "3")
	if m.draftOptions.RepeatEach != 3 || m.optionEdit != "3" {
		t.Fatalf("first repeat digit did not replace value: %+v, edit %q", m.draftOptions, m.optionEdit)
	}
	if !strings.Contains(stripStyles(m.render()), "4 queued → 12 plays") {
		t.Fatal("repeat edit did not update the live preview")
	}
	m = updateKey(t, m, "9")
	if m.optionError == "" || m.draftOptions.RepeatEach != 3 {
		t.Fatal("invalid repeat was accepted")
	}
	m = updateKey(t, m, "backspace")
	if m.optionError != "" || m.optionEdit != "3" {
		t.Fatal("backspace did not restore a valid numeric edit")
	}
	m = updateKey(t, m, "j")
	m = updateKey(t, m, "1")
	m = updateKey(t, m, "2")
	if m.draftOptions.MaximumItems != 12 {
		t.Fatalf("maximum digits = %d, want 12", m.draftOptions.MaximumItems)
	}
	m = updateKey(t, m, "backspace")
	m = updateKey(t, m, "left")
	if m.draftOptions.MaximumItems != 0 || m.optionEditField != -1 {
		t.Fatal("numeric adjustment did not clear the edit buffer")
	}
	m = updateKey(t, m, "r")
	if m.draftOptions != backend.DefaultPlaybackOptions() || m.optionEditField != -1 {
		t.Fatal("reset did not restore the complete option draft")
	}
	m = updateKey(t, m, "k")
	m = updateKey(t, m, "2")
	m = updateKey(t, m, "enter")
	if m.mode != modeNavigate || m.playbackOptions.RepeatEach != 2 {
		t.Fatal("valid options did not save")
	}
	m = updateKey(t, m, "p")
	if m.draftOptions.RepeatEach != 2 {
		t.Fatal("reopened options did not use saved values")
	}
	m = updateKey(t, m, "3")
	m = updateKey(t, m, "esc")
	if m.playbackOptions.RepeatEach != 2 {
		t.Fatal("escape changed saved options")
	}
}

func TestPlaybackOptionsCyclesVersionChoice(t *testing.T) {
	m := New(library.Generate(2, 4))
	m = updateKey(t, m, "p")
	for range 4 {
		m = updateKey(t, m, "j")
	}
	for _, strategy := range []library.SelectionStrategy{library.FavouriteSelection, library.FreshSelection, library.UnseenSelection, library.LatestSelection, library.DefaultSelection} {
		m = updateKey(t, m, "space")
		if m.draftOptions.SelectionStrategy != strategy {
			t.Fatalf("forward strategy = %s, want %s", m.draftOptions.SelectionStrategy, strategy)
		}
	}
	m = updateKey(t, m, "left")
	if m.draftOptions.SelectionStrategy != library.LatestSelection {
		t.Fatalf("backward strategy = %s", m.draftOptions.SelectionStrategy)
	}
	m = updateKey(t, m, "right")
	if m.draftOptions.SelectionStrategy != library.DefaultSelection {
		t.Fatalf("forward right strategy = %s", m.draftOptions.SelectionStrategy)
	}
	m = updateKey(t, m, "enter")
	if m.playbackOptions.SelectionStrategy != library.DefaultSelection {
		t.Fatal("strategy did not save")
	}
}

func TestPlaybackLaunchUsesSavedOptionSnapshotAndKeepsQueueOnFailure(t *testing.T) {
	launcher := &playbackStub{result: backend.PlaybackResult{Succeeded: true, PlannedVideoCount: 4}}
	m := New(library.Generate(4, 8), launcher)
	m = updateKey(t, m, "A")
	m.playbackOptions = backend.PlaybackOptions{Shuffle: true, MaximumItems: 7, RepeatEach: 2, OneVideoPerTrack: true}
	next, command := m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	m = next.(Model)
	if command == nil || !m.launching {
		t.Fatal("launch did not start")
	}
	if next, second := m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"}); second != nil || !strings.Contains(next.(Model).status, "already") {
		t.Fatal("concurrent launch was not blocked")
	}
	m.playbackOptions = backend.DefaultPlaybackOptions()
	next, _ = m.Update(command())
	m = next.(Model)
	if launcher.request.Options != (backend.PlaybackOptions{Shuffle: true, MaximumItems: 7, RepeatEach: 2, OneVideoPerTrack: true}) {
		t.Fatalf("launch options = %+v", launcher.request.Options)
	}
	if len(launcher.request.VideoIDs) != 4 || len(m.queueOrder) != 0 || !strings.Contains(m.status, "Launched 4") {
		t.Fatalf("success lifecycle was incomplete: queue=%d status=%q", len(m.queueOrder), m.status)
	}

	failure := &playbackStub{err: errors.New("transport unavailable")}
	m = New(library.Generate(2, 4), failure)
	m = updateKey(t, m, "a")
	next, command = m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	next, _ = next.(Model).Update(command())
	m = next.(Model)
	if len(m.queueOrder) == 0 || !strings.Contains(m.status, "transport unavailable") {
		t.Fatal("failed launch lost queue or error")
	}

	m = New(library.Generate(2, 4), &playbackStub{result: backend.PlaybackResult{Succeeded: false, UserSafeError: "player rejected queue"}})
	m = updateKey(t, m, "a")
	next, command = m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	next, _ = next.(Model).Update(command())
	if got := next.(Model); len(got.queueOrder) == 0 || !strings.Contains(got.status, "player rejected queue") {
		t.Fatal("safe backend failure lost queue or message")
	}

	m = New(library.Generate(2, 4), launcher)
	next, command = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModCtrl})
	if command != nil || next.(Model).launching {
		t.Fatal("retired ctrl+enter launched playback")
	}
}

func TestOPlaysHighlightedMediaWithoutQueueMutation(t *testing.T) {
	launcher := &playbackStub{result: backend.PlaybackResult{Succeeded: true, PlannedVideoCount: 1}}
	m := New(library.Generate(2, 4), launcher)
	next, command := m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	if command == nil || !next.(Model).launching || len(next.(Model).queueOrder) != 0 {
		t.Fatal("parent o did not launch ephemerally")
	}
	next, _ = next.(Model).Update(command())
	if len(next.(Model).queueOrder) != 0 || len(launcher.ids) != 1 {
		t.Fatal("parent ephemeral launch changed queue")
	}
	m = New(library.Generate(2, 4), launcher)
	m = updateKey(t, m, "l")
	m = updateKey(t, m, "j")
	want := m.filtered[m.rows[m.cursor].trackIndex].Variants[m.rows[m.cursor].variantIndex].ID
	next, command = m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	_, _ = next.(Model).Update(command())
	if got := launcher.ids; len(got) != 1 || got[0] != want {
		t.Fatalf("child launch = %#v, want %q", got, want)
	}
}

func TestFiltersResetAndBulkQueueRespectCurrentResults(t *testing.T) {
	m := New(library.Generate(8, 24))
	m = updateKey(t, m, "a")
	beforeQueue := append([]string(nil), m.queueOrder...)
	m.sort = library.TitleDescending
	m.playbackOptions = backend.PlaybackOptions{RepeatEach: 2}
	m = updateKey(t, m, "f")
	for _, key := range []string{"2", "0", "2", "6"} {
		m = updateKey(t, m, key)
	}
	m = updateKey(t, m, "enter")
	if m.trackDate == nil || !strings.Contains(stripStyles(m.render()), "track release 2026") {
		t.Fatal("accepted track-date filter was not applied or rendered")
	}
	m = updateKey(t, m, "f")
	m = updateKey(t, m, "ctrl+u")
	for _, key := range []string{"x"} {
		m = updateKey(t, m, key)
	}
	m = updateKey(t, m, "enter")
	if m.mode != modeFilters || !strings.Contains(m.status, "Track release") {
		t.Fatal("invalid filter did not remain open with an error")
	}
	m = updateKey(t, m, "esc")
	if m.trackDate == nil {
		t.Fatal("escape changed active filters")
	}
	m = updateKey(t, m, "f")
	m = updateKey(t, m, "r")
	if m.trackDate != nil || m.videoDate != nil || m.sort != library.TitleDescending || m.playbackOptions.RepeatEach != 2 {
		t.Fatal("filter reset changed unrelated state")
	}
	if strings.Join(m.queueOrder, "|") != strings.Join(beforeQueue, "|") {
		t.Fatal("filter reset changed queue order")
	}
	m = updateKey(t, m, "A")
	if len(m.queueOrder) <= len(beforeQueue) {
		t.Fatal("bulk all did not append eligible matching variants")
	}
	count := len(m.queueOrder)
	m = updateKey(t, m, "A")
	if len(m.queueOrder) != count || !strings.Contains(m.status, "already queued") {
		t.Fatal("repeat bulk queue did not deterministically skip duplicates")
	}
}

func TestQueueShortcutsToggleAdvanceBulkAndClear(t *testing.T) {
	m := New(library.Generate(3, 6))
	for track := range m.all {
		for variant := range m.all[track].Variants {
			m.all[track].Variants[variant].Category = library.MusicVideo
		}
	}
	m.refreshResults()
	m = updateKey(t, m, "space")
	if len(m.queueOrder) != 1 || m.cursor != 1 {
		t.Fatalf("space add = queue %d, cursor %d", len(m.queueOrder), m.cursor)
	}
	m.cursor = 0
	m = updateKey(t, m, "space")
	if len(m.queueOrder) != 0 || m.cursor != 1 {
		t.Fatalf("space remove = queue %d, cursor %d", len(m.queueOrder), m.cursor)
	}
	m.cursor = len(m.rows) - 1
	m = updateKey(t, m, "space")
	if m.cursor != len(m.rows)-1 {
		t.Fatal("space moved past the final row")
	}

	m = New(library.Generate(3, 6))
	for track := range m.all {
		for variant := range m.all[track].Variants {
			m.all[track].Variants[variant].Category = library.MusicVideo
		}
	}
	m.all[0].Variants[1].Category = library.BandLive
	m.refreshResults()
	m = updateKey(t, m, "a")
	if len(m.queueOrder) != 1 || !strings.Contains(m.status, "from this track") {
		t.Fatalf("a queue = %d, status %q", len(m.queueOrder), m.status)
	}
	m = updateKey(t, m, "a")
	if len(m.queueOrder) != 1 || !strings.Contains(m.status, "already queued") {
		t.Fatal("repeated a did not skip duplicates")
	}

	m = New(library.Generate(3, 6))
	for track := range m.all {
		for variant := range m.all[track].Variants {
			m.all[track].Variants[variant].Category = library.MusicVideo
		}
	}
	m.refreshResults()
	m = updateKey(t, m, "A")
	if len(m.queueOrder) != len(m.filtered) || !strings.Contains(m.status, "Queued 3 tracks") {
		t.Fatalf("A queue = %d, status %q", len(m.queueOrder), m.status)
	}
	m = updateKey(t, m, "A")
	if len(m.queueOrder) != len(m.filtered) || !strings.Contains(m.status, "already queued") {
		t.Fatal("repeated A did not skip duplicates")
	}
	m = updateKey(t, m, "q")
	m.overlayCursor = 2
	m = updateKey(t, m, "C")
	if m.mode != modeQueue || len(m.queueOrder) != 0 || len(m.queued) != 0 || m.overlayCursor != 0 || m.status != "Queue cleared" {
		t.Fatalf("C clear = %#v", m)
	}
}

func TestOverlaysPreserveTerminalCellAlignmentOverKoreanRows(t *testing.T) {
	base := "가나다라마바사라마바사"
	overlay := "┌────┐\n│ help │\n└────┘"
	rendered := placeOverlay(base, overlay, 20, 3)
	for _, line := range strings.Split(rendered, "\n") {
		if got := ansi.StringWidth(line); got > 20 {
			t.Fatalf("line width = %d, want <= 20: %q", got, line)
		}
	}
	for _, current := range []mode{modeCategories, modeHelp} {
		m := New(library.Generate(4, 8))
		m.all[0].Artist = "가나다라마바사"
		m.refreshResults()
		m.mode = current
		for _, line := range strings.Split(stripStyles(m.render()), "\n") {
			if got := ansi.StringWidth(line); got > m.width {
				t.Fatalf("%s overlay line width = %d", current, got)
			}
		}
	}
}

func TestHelpFootersAndNarrowOverlaysRemainUsable(t *testing.T) {
	wide := footerHint(modeNavigate, 140)
	for _, value := range []string{"/ search", "c categories", "s sort", "f filters", "? help"} {
		if !strings.Contains(wide, value) {
			t.Fatalf("wide footer missing %q: %q", value, wide)
		}
	}
	if strings.Contains(wide, "c/s/q") || strings.Contains(wide, " q queue") {
		t.Fatalf("normal quick actions retain opaque queue shortcut: %q", wide)
	}
	for _, width := range []int{40, 80, 140} {
		if got := footerHint(modeNavigate, width); len(got) > width {
			t.Fatalf("footer width %d exceeds %d", len(got), width)
		}
	}
	for _, current := range []mode{modeSearch, modeCategories, modeSort, modeQueue, modePlaybackOptions, modeFilters, modeHelp} {
		if footerHint(current, 140) == "" {
			t.Fatalf("mode %s has no contextual footer", current)
		}
	}
	help := strings.Join(helpLines(), "\n")
	for _, value := range []string{"h/l, left/right, Enter", "digits / Backspace", "Shift+J/K", "Ctrl+U/D, PgUp/Dn", "o  —  play queue or highlighted media"} {
		if !strings.Contains(help, value) {
			t.Fatalf("help missing %q", value)
		}
	}

	for _, size := range []struct{ width, height int }{{40, 12}, {80, 24}, {140, 40}} {
		for _, current := range []mode{modeNavigate, modeSearch, modeCategories, modeSort, modeQueue, modePlaybackOptions, modeFilters, modeHelp, modeDetails} {
			m := New(library.Generate(20, 80))
			m.mode = current
			m.overlayCursor = 9
			m.helpOffset = 99
			if current == modeQueue {
				m.queueFilteredTracks()
			}
			next, _ := m.Update(tea.WindowSizeMsg{Width: size.width, Height: size.height})
			m = next.(Model)
			view := m.render()
			if lines := strings.Count(view, "\n") + 1; lines > size.height {
				t.Fatalf("%s rendered %d lines at %dx%d", current, lines, size.width, size.height)
			}
			if m.overlayCursor < 0 || m.helpOffset > m.helpMaxOffset() {
				t.Fatalf("%s left invalid overlay state", current)
			}
			if strings.Contains(view, `C:\\Users\\`) {
				t.Fatalf("%s rendered machine-local content", current)
			}
		}
	}
	m := New(library.Generate(20, 80))
	m.queueFilteredTracks()
	m.mode, m.overlayCursor = modeQueue, len(m.queueOrder)-1
	next, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 12})
	if view := stripStyles(next.(Model).render()); !strings.Contains(view, "20.") {
		t.Fatal("queue window did not keep its selected row visible")
	}

	m = New(library.Generate(20, 80))
	m = updateKey(t, m, "/")
	m = updateKey(t, m, "?")
	if m.mode != modeSearch || m.query != "?" {
		t.Fatal("question mark did not remain searchable text in search mode")
	}
}

func TestDetailsOverlayShowsSelectedHistoryAndOwnsKeys(t *testing.T) {
	m := New(library.Generate(2, 4))
	percent := 100.0
	m.filtered[0].History = library.History{PlayedCount: 1, CompletedCount: 1, Recent: []library.HistoryEvent{{Outcome: "completed", Percent: &percent}}}
	m = updateKey(t, m, "d")
	if m.mode != modeDetails || !strings.Contains(stripStyles(m.render()), "History: 1 played") {
		t.Fatal("details did not open with selected history")
	}
	m = updateKey(t, m, "a")
	if len(m.queueOrder) != 0 {
		t.Fatal("details action leaked into queue")
	}
	m = updateKey(t, m, "d")
	if m.mode != modeNavigate {
		t.Fatal("details did not close")
	}
}
