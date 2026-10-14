package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"playlistmaker/charm/internal/backend"
	"playlistmaker/charm/internal/lastfm"
	"playlistmaker/charm/internal/library"
)

func TestNewHistoryModesUseSharedControlsAndShowContext(t *testing.T) {
	date := time.Date(2025, 1, 31, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		mode    int
		preset  lastfm.MixPreset
		context string
	}{
		{4, lastfm.ForgottenFavourites, "Local attempts rest for 30 days"},
		{5, lastfm.CurrentObsessions, "History through: 2025-01-31 UTC"},
	} {
		tracks := library.Generate(2, 2)
		history := &lastfmStub{status: lastfm.Status{LastPlayedAtUTC: &date}, mix: lastfm.MixResult{Created: 1, Requested: 20, Variants: []library.Variant{tracks[1].Variants[0]}}}
		m := New(tracks).WithLastFM(history)
		m = updateKey(t, m, "space")
		m = updateKey(t, m, "p")
		for range tc.mode {
			m = updateKey(t, m, "right")
		}
		m.draftOptions.SelectionStrategy = library.LatestSelection
		m.draftOptions.RepeatEach = 2
		if !strings.Contains(stripStyles(m.render()), tc.context) {
			t.Fatalf("mode %d context missing", tc.mode)
		}
		for _, row := range m.playbackRows() {
			if row >= 6 && row <= 9 {
				t.Fatal("unrelated period settings exposed")
			}
		}
		m = updateKey(t, m, "a")
		if history.request.Preset != tc.preset || history.request.SelectionStrategy != library.LatestSelection || !history.request.QueuedTrackIDs[tracks[0].ID] {
			t.Fatalf("request=%+v", history.request)
		}
		if len(m.queueOrder) != 2 || m.playbackOptions.RepeatEach != 2 {
			t.Fatal("shared queue controls changed")
		}
		m = updateKey(t, m, "p")
		if m.draftMix != tc.mode {
			t.Fatal("mode not remembered")
		}
	}
}

func TestGeneratedMixLaunchAppliesSettingsOnce(t *testing.T) {
	tracks := library.Generate(3, 6)
	history := &lastfmStub{mix: lastfm.MixResult{Variants: []library.Variant{tracks[0].Variants[0], tracks[1].Variants[0]}, Requested: 20, Created: 2}}
	player := &playbackStub{result: backend.PlaybackResult{Succeeded: true, PlannedVideoCount: 4}}
	m := New(tracks, player).WithLastFM(history)
	m.playbackOptions = backend.PlaybackOptions{Shuffle: true, MaximumItems: 1, OneVideoPerTrack: true, RepeatEach: 2}
	m = updateKey(t, m, "p")
	m = updateKey(t, m, "right")
	m = updateKey(t, m, "right")
	if m.draftOptions.Shuffle {
		t.Fatal("new preset inherited shuffle")
	}
	// Play directly from the mix selector without traversing the settings.
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	if cmd == nil {
		t.Fatalf("Play did not launch: %s", next.(Model).status)
	}
	cmd()
	if history.request.Preset != lastfm.BalancedRotation || history.request.Count != 20 {
		t.Fatalf("request: %+v", history.request)
	}
	if player.request.Options.MaximumItems != 0 || player.request.Options.OneVideoPerTrack || player.request.Options.RepeatEach != 2 || player.request.Options.Shuffle {
		t.Fatalf("second selection: %+v", player.request.Options)
	}
	if len(player.request.VideoIDs) != 2 {
		t.Fatal("repeated or truncated queue before playback planning")
	}
}

func TestEmptyMixPreservesExistingQueue(t *testing.T) {
	m := New(library.Generate(1, 1)).WithLastFM(&lastfmStub{})
	m = updateKey(t, m, "space")
	before := m.queueOrder[0]
	m = updateKey(t, m, "p")
	m = updateKey(t, m, "right")
	m = updateKey(t, m, "o")
	if len(m.queueOrder) != 1 || m.queueOrder[0] != before || m.mode != modePlaybackOptions {
		t.Fatal("empty mix replaced queue")
	}
	if !strings.Contains(m.status, "No eligible tracks") {
		t.Fatal("missing empty mix explanation")
	}
}

func TestPlaybackPanelLockedSettingsAndExplicitShuffle(t *testing.T) {
	m := New(library.Generate(1, 1))
	m.openPlaybackPanel()
	m = updateKey(t, m, "right")
	m.overlayCursor = 4
	m = updateKey(t, m, "right")
	if m.draftOptions.SelectionStrategy != library.DefaultSelection {
		t.Fatal("locked unseen choice edited")
	}
	m.overlayCursor = 1
	m = updateKey(t, m, "right")
	if m.draftOptions.OneVideoPerTrack {
		t.Fatal("locked one-per-track edited")
	}
	m.overlayCursor = 0
	m = updateKey(t, m, "right")
	if !m.draftOptions.Shuffle {
		t.Fatal("explicit shuffle ignored")
	}
	m = updateKey(t, m, "p")
	m = updateKey(t, m, "p")
	if !m.draftOptions.Shuffle || m.draftMix != 1 {
		t.Fatal("closing lost the preset")
	}
}

func TestPlaybackPanelKeepsChangesOnClose(t *testing.T) {
	for _, closeKey := range []string{"esc", "p"} {
		m := New(library.Generate(2, 2))
		m = updateKey(t, m, "p")
		m.overlayCursor = 3
		m = updateKey(t, m, "5")
		m.overlayCursor = 1
		m = updateKey(t, m, "space")
		// A half-typed repeat count is invalid; closing keeps the last valid value.
		m.overlayCursor = 2
		m = updateKey(t, m, "4")
		m = updateKey(t, m, "backspace")
		m = updateKey(t, m, closeKey)
		if m.mode != modeNavigate {
			t.Fatalf("%s did not close the panel", closeKey)
		}
		want := backend.PlaybackOptions{MaximumItems: 5, OneVideoPerTrack: true, RepeatEach: 4}
		if m.playbackOptions != want {
			t.Fatalf("%s kept %+v, want %+v", closeKey, m.playbackOptions, want)
		}
		m = updateKey(t, m, "p")
		if m.draftOptions != want || m.optionError != "" {
			t.Fatalf("%s reopened with %+v, error %q", closeKey, m.draftOptions, m.optionError)
		}
	}
}

func TestClosingOnMixKeepsManualQueueSettings(t *testing.T) {
	player := &playbackStub{result: backend.PlaybackResult{Succeeded: true}}
	m := New(library.Generate(2, 2), player)
	m = updateKey(t, m, "space")
	m.playbackOptions = backend.PlaybackOptions{MaximumItems: 1, OneVideoPerTrack: true, RepeatEach: 1}
	m = updateKey(t, m, "p")
	m = updateKey(t, m, "right")
	m = updateKey(t, m, "esc")
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	if cmd == nil {
		t.Fatalf("queue did not launch: %s", next.(Model).status)
	}
	cmd()
	if player.request.Options.MaximumItems != 0 || player.request.Options.OneVideoPerTrack {
		t.Fatalf("mix launch kept manual limits: %+v", player.request.Options)
	}
	m = next.(Model)
	m.launching = false
	m = updateKey(t, m, "p")
	m = updateKey(t, m, "left")
	if m.draftOptions.MaximumItems != 1 || !m.draftOptions.OneVideoPerTrack {
		t.Fatalf("manual queue settings lost: %+v", m.draftOptions)
	}
}

func TestPlaybackPanelKeepsFocusedActionsVisible(t *testing.T) {
	for _, height := range []int{12, 18, 24, 40} {
		m := New(library.Generate(1, 1))
		m.width = 80
		m.height = height
		m.openPlaybackPanel()
		m.draftMix = 3
		for _, row := range []int{5, 6, 10, 11} {
			m.overlayCursor = row
			lines := m.playbackPanelLines(height)
			if !strings.Contains(strings.Join(lines, "\n"), cursorMark(row, row)) {
				t.Fatalf("focused row missing at height %d", height)
			}
			rendered := stripStyles(m.render())
			label := map[int]string{5: "Mix:", 6: "Primary period:", 10: "Play", 11: "Add to queue"}[row]
			if !strings.Contains(rendered, label) {
				t.Fatalf("%s clipped at height %d: %s", label, height, rendered)
			}
		}
	}
}
