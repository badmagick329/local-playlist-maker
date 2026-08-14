package ui

import (
	"context"
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"

	"playlistmaker/charm/internal/backend"
	"playlistmaker/charm/internal/library"
)

type playbackStub struct {
	result backend.PlaybackResult
	err    error
	ids    []string
}

func (p *playbackStub) Launch(_ context.Context, request backend.PlaybackRequest) (backend.PlaybackResult, error) {
	ids := request.VideoIDs
	p.ids = append([]string(nil), ids...)
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

func TestControlEnterLaunchesAndClearsQueueAfterSuccess(t *testing.T) {
	launcher := &playbackStub{result: backend.PlaybackResult{Succeeded: true, PlannedVideoCount: 1}}
	m := New(library.Generate(50, 200), launcher)
	m = updateKey(t, m, "space")

	next, command := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModCtrl})
	m = next.(Model)
	if command == nil || !m.launching {
		t.Fatal("ctrl+enter did not start playback")
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
	next, command := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModCtrl})
	m = next.(Model)
	next, _ = m.Update(command())
	m = next.(Model)
	if len(m.queueOrder) != 1 {
		t.Fatal("failed playback cleared queue")
	}
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
