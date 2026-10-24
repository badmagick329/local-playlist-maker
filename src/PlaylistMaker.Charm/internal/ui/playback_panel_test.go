package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
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
			lines := m.playbackPanelLines(80, height)
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

func typeKeys(t *testing.T, m Model, text string) Model {
	t.Helper()
	for _, r := range text {
		m = updateKey(t, m, string(r))
	}
	return m
}

func TestPlaybackLettersReachTheirRowsAndAct(t *testing.T) {
	m := New(library.Generate(1, 1))
	m = updateKey(t, m, "p")
	check := func(step string, row int) {
		t.Helper()
		if m.mode != modePlaybackOptions || m.overlayCursor != row {
			t.Fatalf("%s: mode %s, cursor %d, want row %d", step, m.mode, m.overlayCursor, row)
		}
	}

	m = updateKey(t, m, "s")
	check("s", 0)
	if !m.draftOptions.Shuffle {
		t.Fatal("s did not toggle Shuffle on")
	}
	m = updateKey(t, m, "u")
	check("u", 1)
	if !m.draftOptions.OneVideoPerTrack {
		t.Fatal("u did not toggle One video per track on")
	}
	m = typeKeys(t, m, "x3")
	check("x", 2)
	if m.draftOptions.RepeatEach != 3 {
		t.Fatalf("digits after x set repeat %d", m.draftOptions.RepeatEach)
	}
	// Moving on commits the typed number, as j/k do.
	m = typeKeys(t, m, "n5")
	check("n", 3)
	if m.draftOptions.RepeatEach != 3 || m.draftOptions.MaximumItems != 5 {
		t.Fatalf("after n5: %+v", m.draftOptions)
	}
	m = updateKey(t, m, "v")
	check("v", 4)
	if m.draftOptions.SelectionStrategy != library.DefaultSelection.Next(1) {
		t.Fatalf("v chose %s", m.draftOptions.SelectionStrategy)
	}
	m = shiftLetter(t, m, 'v')
	if m.draftOptions.SelectionStrategy != library.DefaultSelection {
		t.Fatalf("Shift+v chose %s", m.draftOptions.SelectionStrategy)
	}

	m = updateKey(t, m, "m")
	check("m", 5)
	if m.draftMix != 1 || m.draftOptions.Shuffle {
		t.Fatalf("m: mix %d, shuffle %v; a new mix must drop Shuffle", m.draftMix, m.draftOptions.Shuffle)
	}
	m = shiftLetter(t, m, 'm')
	m = shiftLetter(t, m, 'm')
	if m.draftMix != len(mixNames)-1 {
		t.Fatalf("Shift+m twice from mix 1 reached %d", m.draftMix)
	}
	// h/l are letters too; they must still change the focused row.
	m = typeKeys(t, m, "ll")
	if m.draftMix != 1 {
		t.Fatalf("l after the mix letter reached mix %d", m.draftMix)
	}

	m = typeKeys(t, m, "ll")
	if m.draftMix != 3 {
		t.Fatalf("mix %d, want Listening period", m.draftMix)
	}
	m = typeKeys(t, m, "d2024")
	check("d", 6)
	m = typeKeys(t, m, "e2023")
	check("e", 7)
	m = updateKey(t, m, "c")
	check("c", 8)
	m = updateKey(t, m, "ctrl+u")
	m = typeKeys(t, m, "35")
	if m.periodDraft != [3]string{"2024", "2023", "35"} {
		t.Fatalf("period drafts %q", m.periodDraft)
	}
	before := m.draftMethod
	m = updateKey(t, m, "w")
	check("w", 9)
	if m.draftMethod != before.Next(1) {
		t.Fatalf("w chose %s", m.draftMethod)
	}
	m = shiftLetter(t, m, 'w')
	if m.draftMethod != before {
		t.Fatalf("Shift+w chose %s", m.draftMethod)
	}
}

func TestPlaybackLettersForHiddenRowsDoNothing(t *testing.T) {
	m := New(library.Generate(1, 1))
	m = updateKey(t, m, "p")
	want := m
	for _, key := range []string{"d", "e", "c", "w", "a"} {
		m = updateKey(t, m, key)
		if m.mode != modePlaybackOptions || m.overlayCursor != 5 || m.periodDraft != want.periodDraft || m.draftMethod != want.draftMethod {
			t.Fatalf("manual queue %s: mode %s, cursor %d", key, m.mode, m.overlayCursor)
		}
	}
	// Listening period with no secondary period hides only the percentage.
	m = typeKeys(t, m, "mmm")
	m = typeKeys(t, m, "c")
	if m.overlayCursor != 5 || m.periodDraft[2] != "20" {
		t.Fatalf("hidden percentage: cursor %d, draft %q", m.overlayCursor, m.periodDraft[2])
	}
}

func TestPlaybackLettersOnLockedRowsKeepValues(t *testing.T) {
	m := New(library.Generate(1, 1))
	m = updateKey(t, m, "p")
	m = updateKey(t, m, "m")
	m = updateKey(t, m, "u")
	if m.overlayCursor != 1 || m.draftOptions.OneVideoPerTrack {
		t.Fatalf("locked one-per-track: cursor %d, value %v", m.overlayCursor, m.draftOptions.OneVideoPerTrack)
	}
	m = updateKey(t, m, "v")
	m = shiftLetter(t, m, 'v')
	if m.overlayCursor != 4 || m.draftOptions.SelectionStrategy != library.DefaultSelection {
		t.Fatalf("locked unseen choice: cursor %d, value %s", m.overlayCursor, m.draftOptions.SelectionStrategy)
	}
}

func TestPlaybackLetterIsBlockedByInvalidNumberLikeJK(t *testing.T) {
	m := New(library.Generate(1, 1))
	m = updateKey(t, m, "p")
	m = typeKeys(t, m, "x4")
	m = updateKey(t, m, "backspace")
	for _, key := range []string{"j", "s", "m"} {
		m = updateKey(t, m, key)
		if m.overlayCursor != 2 || m.draftOptions.Shuffle || m.draftMix != 0 || m.optionError == "" {
			t.Fatalf("%s left an invalid repeat: cursor %d, error %q", key, m.overlayCursor, m.optionError)
		}
	}

	// A generated mix's track count blocks the same way.
	m = New(library.Generate(1, 1))
	m = updateKey(t, m, "p")
	m = typeKeys(t, m, "mn")
	m = updateKey(t, m, "backspace")
	m = updateKey(t, m, "backspace")
	m = updateKey(t, m, "m")
	if m.overlayCursor != 3 || m.draftMix != 1 || m.optionError == "" {
		t.Fatalf("empty track count let m through: cursor %d, mix %d", m.overlayCursor, m.draftMix)
	}
}

func TestPlaybackRowKeysCoverEveryRowWithoutCollidingWithPanelKeys(t *testing.T) {
	seen := map[rune]int{}
	for row := range 12 {
		letter, ok := playbackRowKeys[row]
		if !ok {
			t.Fatalf("row %d has no letter", row)
		}
		// Play and Add to queue show the keys that already run them.
		switch row {
		case 10, 11:
			if want := map[int]rune{10: 'o', 11: 'a'}[row]; letter != want {
				t.Fatalf("row %d shows %q, want %q", row, letter, want)
			}
		default:
			if letter < 'a' || letter > 'z' || strings.ContainsRune("oarphjkl", letter) {
				t.Fatalf("row %d letter %q is not a free lowercase letter", row, letter)
			}
		}
		if other, dup := seen[letter]; dup {
			t.Fatalf("rows %d and %d share %q", row, other, letter)
		}
		seen[letter] = row
	}
	if len(playbackRowKeys) != 12 {
		t.Fatalf("%d letters for 12 rows", len(playbackRowKeys))
	}
}

func TestPlaybackPanelShowsLettersAndFittingFooter(t *testing.T) {
	for _, size := range []struct {
		width int
		want  string
	}{{40, "letter row · Shift back · p close"}, {80, "o play · a add mix · letter row · Shift reverse · h/l change · p/Esc close"}, {120, "o play · a add mix · letter row · Shift reverse · h/l change · p/Esc close"}} {
		m := New(library.Generate(1, 1))
		m.openPlaybackPanel()
		m.draftMix = 2
		content := stripStyles(m.renderOverlay("", size.width, 24))
		for _, row := range []string{"› m Mix: Balanced rotation", "  v Performances:", "  n Track count: 20", "  s Order: Mix order", "  u One video per track: Always", "  x Repeat each: 1", "  o Play", "  a Add to queue", size.want} {
			if !strings.Contains(content, row) {
				t.Fatalf("width %d missing %q: %q", size.width, row, content)
			}
		}
		for _, line := range strings.Split(content, "\n") {
			if ansi.StringWidth(line) > size.width {
				t.Fatalf("overlay line exceeds width %d: %q", size.width, line)
			}
		}
		if hint := footerHint(modePlaybackOptions, size.width); ansi.StringWidth(hint) > size.width || !strings.Contains(hint, "letter row") {
			t.Fatalf("width %d footer hint %q", size.width, hint)
		}
	}
}
