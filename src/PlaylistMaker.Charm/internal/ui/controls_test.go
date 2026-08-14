package ui

import (
	"errors"
	"math"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

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
	m = updateKey(t, m, "a")
	m = updateKey(t, m, "o")
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
	m = updateKey(t, m, "o")
	if m.draftOptions.RepeatEach != 2 {
		t.Fatal("reopened options did not use saved values")
	}
	m = updateKey(t, m, "3")
	m = updateKey(t, m, "esc")
	if m.playbackOptions.RepeatEach != 2 {
		t.Fatal("escape changed saved options")
	}
}

func TestPlaybackLaunchUsesSavedOptionSnapshotAndKeepsQueueOnFailure(t *testing.T) {
	launcher := &playbackStub{result: backend.PlaybackResult{Succeeded: true, PlannedVideoCount: 4}}
	m := New(library.Generate(4, 8), launcher)
	m = updateKey(t, m, "a")
	m.playbackOptions = backend.PlaybackOptions{Shuffle: true, MaximumItems: 7, RepeatEach: 2, OneVideoPerTrack: true}
	next, command := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModCtrl})
	m = next.(Model)
	if command == nil || !m.launching {
		t.Fatal("launch did not start")
	}
	if next, second := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModCtrl}); second != nil || !strings.Contains(next.(Model).status, "already") {
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
	next, command = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModCtrl})
	next, _ = next.(Model).Update(command())
	m = next.(Model)
	if len(m.queueOrder) == 0 || !strings.Contains(m.status, "transport unavailable") {
		t.Fatal("failed launch lost queue or error")
	}

	m = New(library.Generate(2, 4), &playbackStub{result: backend.PlaybackResult{Succeeded: false, UserSafeError: "player rejected queue"}})
	m = updateKey(t, m, "a")
	next, command = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModCtrl})
	next, _ = next.(Model).Update(command())
	if got := next.(Model); len(got.queueOrder) == 0 || !strings.Contains(got.status, "player rejected queue") {
		t.Fatal("safe backend failure lost queue or message")
	}

	m = New(library.Generate(2, 4), launcher)
	next, command = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModCtrl})
	if command != nil || !strings.Contains(next.(Model).status, "empty") {
		t.Fatal("empty queue launch was not blocked")
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
	if m.trackDate == nil || !strings.Contains(stripStyles(m.render()), "track 2026") {
		t.Fatal("accepted track-date filter was not applied or rendered")
	}
	m = updateKey(t, m, "f")
	m = updateKey(t, m, "ctrl+u")
	for _, key := range []string{"x"} {
		m = updateKey(t, m, key)
	}
	m = updateKey(t, m, "enter")
	if m.mode != modeFilters || !strings.Contains(m.status, "Track date") {
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
	for _, value := range []string{"h/l, left/right, Enter", "digits / Backspace", "Shift+J/K", "Ctrl+U/D, PgUp/Dn", "Ctrl+Enter"} {
		if !strings.Contains(help, value) {
			t.Fatalf("help missing %q", value)
		}
	}

	for _, size := range []struct{ width, height int }{{40, 12}, {80, 24}, {140, 40}} {
		for _, current := range []mode{modeNavigate, modeSearch, modeCategories, modeSort, modeQueue, modePlaybackOptions, modeFilters, modeHelp} {
			m := New(library.Generate(20, 80))
			m.mode = current
			m.overlayCursor = 9
			m.helpOffset = 99
			if current == modeQueue {
				m.queueAll(false)
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
	m.queueAll(false)
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
