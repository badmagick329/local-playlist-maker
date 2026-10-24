package ui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"playlistmaker/charm/internal/backend"
	"playlistmaker/charm/internal/library"
	"playlistmaker/charm/internal/spotifylink"
	"playlistmaker/charm/internal/updater"
)

func TestBrokenLinkWarningSurvivesStatusChanges(t *testing.T) {
	m := New([]library.Track{{Artist: "TWICE", Title: "I CAN'T STOP ME", LocalAudioIssue: "missing file"}})
	m.status = "Queued a video"
	footer := m.renderFooter(200)
	if !strings.Contains(footer, "1 broken local audio link") || !strings.Contains(footer, "TWICE") {
		t.Fatalf("warning hidden by normal status: %s", footer)
	}
}

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
	items          []updater.Item
	ignored        []updater.Item
	candidates     []updater.Audio
	tracks         []library.Track
	removed        int
	allowNew       bool
	createCalls    int
	searchCalls    int
	confirmedVideo string
	confirmedTrack string
	scanCalls      int
	confirms       int
	ignores        int
	restores       int
}

func (s *mappingUpdaterStub) Scan(context.Context) (updater.ScanResult, error) {
	s.scanCalls++
	return updater.ScanResult{Items: s.items, Removed: s.removed}, nil
}
func (s *mappingUpdaterStub) Ignored(context.Context) ([]updater.Item, error) { return s.ignored, nil }
func (s *mappingUpdaterStub) Search(context.Context, string) ([]updater.Audio, error) {
	s.searchCalls++
	return s.candidates, nil
}
func (s *mappingUpdaterStub) Confirm(video, track string, allowNew bool) error {
	s.confirms++
	s.allowNew = allowNew
	s.confirmedVideo, s.confirmedTrack = video, track
	return nil
}
func (s *mappingUpdaterStub) Create(video, artist, title string, allow bool) error {
	s.confirms++
	s.createCalls++
	s.allowNew = allow
	return nil
}
func (s *mappingUpdaterStub) Ignore(string) error  { s.ignores++; return nil }
func (s *mappingUpdaterStub) Restore(string) error { s.restores++; return nil }
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

func TestCapitalCClearsSearchFromNavigation(t *testing.T) {
	m := New(library.Generate(50, 200))
	m = updateKey(t, m, "/")
	m = updateKey(t, m, "a")
	m = updateKey(t, m, "/")
	m.cursor = min(3, len(m.rows)-1)

	m = updateKey(t, m, "C")

	if m.query != "" || m.cursor != 0 || m.status != "Search cleared" {
		t.Fatalf("C did not clear search: query=%q cursor=%d status=%q", m.query, m.cursor, m.status)
	}
	if len(m.filtered) != len(m.all) {
		t.Fatalf("C filtered %d of %d tracks", len(m.filtered), len(m.all))
	}
}

func TestOpeningKeysCloseEveryMode(t *testing.T) {
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
			// Playback options keep changes on close; every other draft is discarded.
			if test.mode == modePlaybackOptions && !m.playbackOptions.Shuffle {
				t.Fatal("options close discarded a change")
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

func TestExpansionAndPlaybackUseTheSameLanguageAwareDefault(t *testing.T) {
	old := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	track := library.Track{ID: "track", Artist: "Artist", Title: "Song", Variants: []library.Variant{
		{ID: "original", Filename: "Artist - Song.mkv", Category: library.MusicVideo, Date: old, ModifiedAt: old},
		{ID: "japanese", Filename: "Artist - Song Japanese.mkv", Category: library.MusicVideo, Date: old.AddDate(1, 0, 0), ModifiedAt: old.AddDate(1, 0, 0)},
	}, SearchTextByCategory: map[library.Category]string{}}
	playback := &playbackStub{result: backend.PlaybackResult{Succeeded: true, PlannedVideoCount: 1}}
	m := New([]library.Track{track}, playback)
	m = updateKey(t, m, "l")
	if len(m.rows) < 2 {
		t.Fatal("track did not expand")
	}
	firstChild := m.filtered[m.rows[1].trackIndex].Variants[m.rows[1].variantIndex]
	if firstChild.ID != "original" {
		t.Fatalf("expanded default = %s", firstChild.ID)
	}
	m.cursor = 0
	_, command := m.launchHighlighted()
	if command == nil {
		t.Fatal("highlighted playback did not start")
	}
	_ = command()
	if len(playback.ids) != 1 || playback.ids[0] != firstChild.ID {
		t.Fatalf("playback default = %#v, want %s", playback.ids, firstChild.ID)
	}
}

func TestExpansionDirectPlaybackAndBulkQueueUseLatestSelection(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	track := library.Track{ID: "track", Artist: "Artist", Title: "Song", Variants: []library.Variant{
		{ID: "music-video", Filename: "Artist - Song.mkv", Category: library.MusicVideo, Date: base.AddDate(0, 0, 2), ModifiedAt: base},
		{ID: "latest-performance", Filename: "Artist - Song (Live).mkv", Category: library.Performance, Date: base, ModifiedAt: base.AddDate(0, 0, 1), History: library.History{SkippedCount: 9}},
	}, SearchTextByCategory: map[library.Category]string{}}
	playback := &playbackStub{result: backend.PlaybackResult{Succeeded: true, PlannedVideoCount: 1}}
	m := New([]library.Track{track}, playback)
	m.enabled[library.Performance] = true
	m.playbackOptions.SelectionStrategy = library.LatestSelection
	m.refreshResults()
	m = updateKey(t, m, "l")
	if len(m.rows) < 2 {
		t.Fatal("track did not expand")
	}
	firstChild := m.filtered[m.rows[1].trackIndex].Variants[m.rows[1].variantIndex]
	if firstChild.ID != "latest-performance" {
		t.Fatalf("expanded latest = %s", firstChild.ID)
	}
	m.cursor = 0
	_, command := m.launchHighlighted()
	if command == nil {
		t.Fatal("latest highlighted playback did not start")
	}
	_ = command()
	if len(playback.ids) != 1 || playback.ids[0] != firstChild.ID {
		t.Fatalf("latest direct playback = %#v, want %s", playback.ids, firstChild.ID)
	}
	m.queueFilteredTracks()
	if len(m.queueOrder) != 1 || m.queueOrder[0] != firstChild.ID {
		t.Fatalf("latest bulk queue = %#v, want %s", m.queueOrder, firstChild.ID)
	}
}

func TestParentRowsShowTheActiveEligibleDateSortValue(t *testing.T) {
	track := library.Track{ID: "track", Artist: "Artist", Title: "Song", ReleaseDateLabel: "2024-01-01", Variants: []library.Variant{
		{ID: "mv", Filename: "Artist - Song.mkv", Category: library.MusicVideo, Date: time.Date(2025, 8, 11, 0, 0, 0, 0, time.UTC), ModifiedAt: time.Date(2026, 2, 24, 0, 0, 0, 0, time.UTC)},
		{ID: "show", Filename: "Artist - Song (Live).mkv", Category: library.MusicShow, Date: time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC), ModifiedAt: time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)},
	}, SearchTextByCategory: map[library.Category]string{}}
	m := New([]library.Track{track})
	m.enabled[library.MusicShow] = true
	for _, test := range []struct {
		sort library.Sort
		want string
	}{
		{library.ModifiedNewest, "2026-08-15"},
		{library.ModifiedOldest, "2026-08-15"},
		{library.VideoNewest, "2026-07-24"},
		{library.VideoOldest, "2026-07-24"},
		{library.ReleaseNewest, "2024-01-01"},
	} {
		m.sort = test.sort
		m.refreshResults()
		if got := stripStyles(m.renderRow(m.rows[0], false, 120, m.parentRowCountWidth())); !strings.Contains(got, test.want) {
			t.Fatalf("%s parent row = %q, want %s", test.sort, got, test.want)
		}
	}
}

func TestParentRowsKeepDateAlignedAcrossCountWidths(t *testing.T) {
	variants := func(count int) []library.Variant {
		result := make([]library.Variant, count)
		for index := range result {
			result[index] = library.Variant{ID: fmt.Sprintf("video-%d", index), Filename: fmt.Sprintf("video-%d.mkv", index), Category: library.MusicVideo}
		}
		return result
	}
	tracks := []library.Track{
		{ID: "one", Artist: "Artist One", Title: "One", ReleaseDateLabel: "2026-08-10", Variants: variants(1)},
		{ID: "many", Artist: "Artist Many", Title: "Many", ReleaseDateLabel: "2026-08-10", Variants: variants(13)},
	}
	m := New(tracks)
	m.sort = library.ReleaseNewest
	m.refreshResults()
	rows := map[string]string{}
	for _, current := range m.rows {
		if current.isVariant() {
			continue
		}
		track := m.filtered[current.trackIndex]
		rows[track.ID] = stripStyles(m.renderRow(current, false, 100, m.parentRowCountWidth()))
	}
	for _, id := range []string{"one", "many"} {
		if !strings.HasSuffix(rows[id], map[string]string{"one": "  1", "many": "  13"}[id]) {
			t.Fatalf("row %s has unexpected count formatting: %q", id, rows[id])
		}
	}
	if one, many := strings.Index(rows["one"], "2026-08-10"), strings.Index(rows["many"], "2026-08-10"); one != many {
		t.Fatalf("date columns are misaligned: one=%d many=%d\n%s\n%s", one, many, rows["one"], rows["many"])
	}
}

func TestClassifyTrackSourcePrefersSpotifyThenLocal(t *testing.T) {
	for _, test := range []struct {
		name   string
		track  library.Track
		wanted trackSource
	}{
		{name: "Spotify and local", track: library.Track{SpotifyURI: "spotify:track:one", LocalAudioPath: "song.flac"}, wanted: trackSourceSpotify},
		{name: "Spotify", track: library.Track{SpotifyURI: "spotify:track:one"}, wanted: trackSourceSpotify},
		{name: "local", track: library.Track{LocalAudioPath: "song.flac"}, wanted: trackSourceLocal},
		{name: "video only", track: library.Track{}, wanted: trackSourceVideoOnly},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := classifyTrackSource(test.track); got != test.wanted {
				t.Fatalf("source = %d, want %d", got, test.wanted)
			}
		})
	}
}

func TestSourceBadgesRenderOnlyOnParentRows(t *testing.T) {
	tracks := []library.Track{
		{ID: "spotify", Artist: "Spotify", Title: "Track", SpotifyURI: "spotify:track:one", Variants: []library.Variant{{ID: "spotify-video", Filename: "spotify.mkv", Category: library.MusicVideo}}},
		{ID: "local", Artist: "Local", Title: "Track", LocalAudioPath: "song.flac", Variants: []library.Variant{{ID: "local-video", Filename: "local.mkv", Category: library.MusicVideo}}},
		{ID: "video", Artist: "Video", Title: "Only", Variants: []library.Variant{{ID: "video-only", Filename: "video.mkv", Category: library.MusicVideo}}},
	}
	for index, want := range []string{"Spotify", "Local", "Video only"} {
		m := New([]library.Track{tracks[index]})
		row := stripStyles(m.renderRow(m.rows[0], false, 80, m.parentRowCountWidth()))
		if !strings.Contains(row, "•") {
			t.Fatalf("parent row %d has no source badge: %q", index, row)
		}
		if !strings.HasPrefix(row, "›    • "+tracks[index].Artist) {
			t.Fatalf("parent row %d source badge is not at the start: %q", index, row)
		}
		m.mode = modeDetails
		if got := strings.Join(m.detailsLines(), "\n"); !strings.Contains(got, "Audio source: "+want) {
			t.Fatalf("details %d = %q, want %s", index, got, want)
		}
	}

	m := New(tracks)
	m.expanded[tracks[0].ID] = true
	m.rebuildRows()
	variantIndex := -1
	for index, current := range m.rows {
		if current.isVariant() {
			variantIndex = index
			break
		}
	}
	if variantIndex < 0 {
		t.Fatal("expanded track did not produce a variant row")
	}
	variantRow := stripStyles(m.renderRow(m.rows[variantIndex], false, 80, m.parentRowCountWidth()))
	if strings.Contains(variantRow, "•") {
		t.Fatalf("variant row gained source badge: %q", variantRow)
	}

	selectedBackground := ";48;2;139;213;255m"
	selectedCases := []struct {
		name  string
		track library.Track
	}{
		{name: "Spotify", track: tracks[0]},
		{name: "Local", track: tracks[1]},
		{name: "Video only", track: tracks[2]},
	}
	for _, test := range selectedCases {
		t.Run("selected "+test.name, func(t *testing.T) {
			rowModel := New([]library.Track{test.track})
			raw := rowModel.renderRow(rowModel.rows[0], true, 80, rowModel.parentRowCountWidth())
			badge := rowModel.selectedSourceBadge(test.track)
			separator := rowModel.theme.selected.Render(" ")
			before, after, found := strings.Cut(raw, separator+badge+separator)
			if !found || !strings.Contains(before, selectedBackground) || !strings.Contains(after, selectedBackground) {
				t.Fatalf("selected row does not keep background around badge: %q", raw)
			}
			if !strings.Contains(separator, selectedBackground) {
				t.Fatalf("selected badge separator has no selected background: %q", separator)
			}
			if badge == rowModel.theme.selected.Render("•") {
				t.Fatalf("selected %s badge uses the ordinary selected foreground", test.name)
			}
			if got := ansi.StringWidth(raw); got != 80 {
				t.Fatalf("selected row width = %d, want 80", got)
			}
			for _, ordinary := range []string{"› ", test.track.Artist, "  —  ", test.track.Title, "0001-01-01  1"} {
				if !strings.Contains(raw, rowModel.theme.selected.Render(ordinary)) {
					t.Fatalf("selected %s content %q does not use selected foreground: %q", test.name, ordinary, raw)
				}
			}
			unselected := rowModel.renderRow(rowModel.rows[0], false, 80, rowModel.parentRowCountWidth())
			if got := ansi.StringWidth(unselected); got != 80 {
				t.Fatalf("unselected row width = %d, want 80", got)
			}
		})
	}

	localModel := New([]library.Track{tracks[1]})
	localSelected := localModel.renderRow(localModel.rows[0], true, 80, localModel.parentRowCountWidth())
	if !strings.Contains(localSelected, localModel.selectedSourceBadge(tracks[1])) {
		t.Fatalf("selected local badge is not using its contrasting foreground: %q", localSelected)
	}
	if strings.Contains(localSelected, localModel.theme.selected.Render("•")) {
		t.Fatalf("selected local badge still uses the ordinary selected foreground: %q", localSelected)
	}

	queuedModel := New([]library.Track{tracks[0]})
	queuedModel.queued[tracks[0].Variants[0].ID] = tracks[0].Variants[0]
	queuedModel.queueOrder = []string{tracks[0].Variants[0].ID}
	queuedRaw := queuedModel.renderRow(queuedModel.rows[0], true, 80, queuedModel.parentRowCountWidth())
	queuedText := stripStyles(queuedRaw)
	if !strings.Contains(queuedText, "•") || !strings.Contains(queuedText, "●") {
		t.Fatalf("selected row does not retain both queue and source dots: %q", queuedText)
	}
	if !strings.Contains(queuedRaw, queuedModel.theme.selected.Render("● ")) {
		t.Fatalf("selected row lost queued indicator: %q", queuedRaw)
	}

	long := New([]library.Track{{Artist: "Artist", Title: strings.Repeat("Long title ", 12), SpotifyURI: "spotify:track:long", Variants: []library.Variant{{ID: "long-video", Filename: "long.mkv", Category: library.MusicVideo}}}})
	longRow := stripStyles(long.renderRow(long.rows[0], false, 40, long.parentRowCountWidth()))
	if !strings.Contains(longRow, "•") {
		t.Fatalf("narrow parent row lost source badge: %q", longRow)
	}
	if width := ansi.StringWidth(longRow); width > 40 {
		t.Fatalf("narrow parent row width = %d, want <= 40", width)
	}
	longSelected := stripStyles(long.renderRow(long.rows[0], true, 40, long.parentRowCountWidth()))
	if width := ansi.StringWidth(longSelected); width > 40 {
		t.Fatalf("narrow selected parent row width = %d, want <= 40", width)
	}
}

func TestMappingUpdatePersistentIgnoreRestoreAndCursorSafety(t *testing.T) {
	items := []updater.Item{{VideoPath: "one", Filename: "one.mkv"}, {VideoPath: "two", Filename: "two.mkv"}}
	stub := &mappingUpdaterStub{items: items, ignored: []updater.Item{{VideoPath: "ignored", Filename: "ignored.mkv"}}, tracks: library.Generate(1, 2)}
	m := New(library.Generate(1, 2)).WithMappingUpdater(stub)
	m = finishMappingScan(t, m)
	m.mappingIndex = 1
	m = runMappingCommand(t, m, "i")
	if stub.ignores != 1 || len(m.mappingItems) != 1 || m.mappingIndex != 0 || m.status != "Video ignored" {
		t.Fatalf("ignore final item = %#v", m)
	}
	m = runMappingCommand(t, m, "I")
	if !m.mappingIgnored || len(m.mappingItems) != 1 {
		t.Fatalf("ignored list = %#v", m)
	}
	m = runMappingCommand(t, m, "i")
	if stub.restores != 1 || len(m.mappingItems) != 0 || m.mappingIndex != 0 || m.status != "Video restored" {
		t.Fatalf("restore final item = %#v", m)
	}
}

func TestMappingUpdateSummaryCountsOnlySuggestionsAndSkipIsTemporary(t *testing.T) {
	items := []updater.Item{{VideoPath: "one", Filename: "one.mkv", AudioPath: "audio"}, {VideoPath: "two", Filename: "two.mkv"}}
	stub := &mappingUpdaterStub{items: items}
	m := finishMappingScan(t, New(library.Generate(1, 2)).WithMappingUpdater(stub))
	if m.status != "2 unmapped videos • 1 suggestions" || !strings.Contains(stripStyles(m.View().Content), "2 unmapped videos • 1 suggestions") {
		t.Fatalf("mapping summary = %q", m.status)
	}
	m = updateKey(t, m, "s")
	if stub.ignores != 0 || len(m.mappingItems) != 2 || m.mappingIndex != 1 {
		t.Fatalf("temporary skip = %#v", m)
	}
}

func TestMappingScanRemovalMarksDirtyAndReportsCount(t *testing.T) {
	stub := &mappingUpdaterStub{removed: 1, tracks: library.Generate(1, 2)}
	m := finishMappingScan(t, New(library.Generate(1, 2)).WithMappingUpdater(stub))
	if !m.mappingDirty || m.status != "1 missing video removed • 0 unmapped videos • 0 suggestions" {
		t.Fatalf("mapping removal state = dirty %v status %q", m.mappingDirty, m.status)
	}
	next, command := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if next.(Model).mode != modeNavigate || command == nil {
		t.Fatal("closing after removal did not request reload")
	}
}

func TestSpotifyScanErrorDoesNotRenderSuccessfulEmptyState(t *testing.T) {
	m := New(library.Generate(1, 2))
	m.mode = modeSpotifyUpdate
	next, _ := m.Update(spotifyScanMsg{err: errors.New("search failed")})
	m = next.(Model)
	view := stripStyles(m.render())
	if !strings.Contains(view, "Spotify scan failed") || strings.Contains(view, "All eligible catalogue tracks have Spotify links or are ignored") {
		t.Fatalf("failed scan view = %q", view)
	}
	if !strings.Contains(view, "search failed") {
		t.Fatalf("failed scan omitted error = %q", view)
	}
}

func TestSpotifyScanSuccessClearsPreviousError(t *testing.T) {
	m := New(library.Generate(1, 2))
	m.mode = modeSpotifyUpdate
	next, _ := m.Update(spotifyScanMsg{err: errors.New("search failed")})
	m = next.(Model)
	next, _ = m.Update(spotifyScanMsg{})
	m = next.(Model)
	view := stripStyles(m.render())
	if !strings.Contains(view, "All eligible catalogue tracks have Spotify links or are ignored") || strings.Contains(view, "Spotify scan failed") {
		t.Fatalf("successful scan view = %q", view)
	}
}

func TestSpotifyScanErrorKeepsPartialReviewItems(t *testing.T) {
	m := New(library.Generate(1, 2))
	m.mode = modeSpotifyUpdate
	result := spotifylink.ScanResult{Items: []spotifylink.Item{{Artist: "aespa", Title: "Switchblade"}}}
	next, _ := m.Update(spotifyScanMsg{result: result, err: errors.New("search failed")})
	m = next.(Model)
	view := stripStyles(m.render())
	if !strings.Contains(view, "aespa — Switchblade") || !strings.Contains(view, "LOCAL TRACK") {
		t.Fatalf("partial failed scan view = %q", view)
	}
}

func finishMappingScan(t *testing.T, m Model) Model {
	t.Helper()
	next, command := m.Update(tea.KeyPressMsg{Code: 'u', Text: "u"})
	if command == nil {
		t.Fatal("mapping scan command was nil")
	}
	next, _ = next.(Model).Update(command())
	return next.(Model)
}

func runMappingCommand(t *testing.T, m Model, key string) Model {
	t.Helper()
	next, command := m.Update(tea.KeyPressMsg{Code: rune(key[0]), Text: key})
	if command == nil {
		t.Fatalf("%q did not return a command", key)
	}
	next, _ = next.(Model).Update(command())
	return next.(Model)
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

func TestSortLetterAppliesFieldAndShiftLetterReverses(t *testing.T) {
	for _, test := range []struct {
		letter            rune
		forward, reversed library.Sort
	}{
		{'m', library.ModifiedNewest, library.ModifiedOldest},
		{'a', library.ArtistAscending, library.ArtistDescending},
		{'t', library.TitleAscending, library.TitleDescending},
		{'r', library.ReleaseNewest, library.ReleaseOldest},
		{'v', library.VideoNewest, library.VideoOldest},
		{'e', library.Relevance, library.Relevance},
	} {
		for _, shifted := range []bool{false, true} {
			m := New(library.Generate(50, 200))
			// Start from a sort outside the field so the press, not the
			// starting state, decides the result.
			m.sort = library.VideoOldest
			if test.letter == 'v' {
				m.sort = library.TitleDescending
			}
			m.cursor = 20
			m = updateKey(t, m, "s")
			want := test.forward
			if shifted {
				m = shiftLetter(t, m, test.letter)
				want = test.reversed
			} else {
				m = updateKey(t, m, string(test.letter))
			}
			if m.sort != want || m.mode != modeNavigate || m.cursor != 0 || m.status != "Sorted by "+want.String() {
				t.Fatalf("%c shifted=%v: sort %s, mode %s, cursor %d, status %q; want %s, closed, top", test.letter, shifted, m.sort, m.mode, m.cursor, m.status, want)
			}
		}
	}
}

func TestSortEnterOnActiveFieldFlipsDirection(t *testing.T) {
	m := New(library.Generate(50, 200))
	m.sort = library.ArtistAscending
	m = updateKey(t, m, "s")
	if sortFields[m.overlayCursor].forward != library.ArtistAscending {
		t.Fatalf("opened on row %d, want the active Artist field", m.overlayCursor)
	}
	m = updateKey(t, m, "enter")
	if m.sort != library.ArtistDescending || m.mode != modeNavigate {
		t.Fatalf("first enter: sort %s, mode %s", m.sort, m.mode)
	}
	m = updateKey(t, m, "s")
	m = updateKey(t, m, "space")
	if m.sort != library.ArtistAscending {
		t.Fatalf("second enter: sort %s, want back to Artist A-Z", m.sort)
	}

	// Relevance has no reverse, so Enter on it keeps Relevance.
	m.sort = library.Relevance
	m = updateKey(t, m, "s")
	m = updateKey(t, m, "enter")
	if m.sort != library.Relevance {
		t.Fatalf("enter on Relevance gave %s", m.sort)
	}
}

func TestSortViewClosesWithSAndEscapeKeepingSort(t *testing.T) {
	for _, key := range []string{"s", "esc"} {
		m := New(library.Generate(50, 200))
		m.sort = library.TitleDescending
		m = updateKey(t, m, "s")
		m = updateKey(t, m, "j")
		m = updateKey(t, m, key)
		if m.mode != modeNavigate || m.sort != library.TitleDescending {
			t.Fatalf("%s: mode %s, sort %s", key, m.mode, m.sort)
		}
	}
}

func TestSortIgnoresCtrlAndAltLetters(t *testing.T) {
	m := New(library.Generate(50, 200))
	m = updateKey(t, m, "s")
	for _, mod := range []tea.KeyMod{tea.ModCtrl, tea.ModAlt} {
		next, _ := m.Update(tea.KeyPressMsg{Code: 'a', Mod: mod, Text: "a"})
		m = next.(Model)
		if m.mode != modeSort || m.sort != library.ModifiedNewest {
			t.Fatalf("mod %v: mode %s, sort %s", mod, m.mode, m.sort)
		}
	}
}

func TestSortFieldsCoverEverySortWithoutCollidingWithViewKeys(t *testing.T) {
	seen := map[rune]string{}
	covered := map[library.Sort]bool{}
	for _, field := range sortFields {
		if field.key < 'a' || field.key > 'z' || strings.ContainsRune("jks", field.key) {
			t.Fatalf("%s letter %q is not a free lowercase letter", field.name, field.key)
		}
		if other, dup := seen[field.key]; dup {
			t.Fatalf("%s and %s share %q", field.name, other, field.key)
		}
		seen[field.key] = field.name
		for _, sort := range []library.Sort{field.forward, field.reverse} {
			if covered[sort] && field.forward != field.reverse {
				t.Fatalf("%s repeats sort %s", field.name, sort)
			}
			covered[sort] = true
		}
	}
	for _, sort := range library.Sorts {
		if !covered[sort] {
			t.Fatalf("sort %s has no field", sort)
		}
	}
}

func TestSortOverlayShowsLettersActiveDirectionAndFittingFooter(t *testing.T) {
	for _, size := range []struct {
		width int
		want  string
	}{{40, "key sort  Shift flips"}, {80, "letter sort • Shift reverse • s/esc close"}, {120, "j/k move  •  letter sort  •  Shift+letter reverse  •  s/esc close"}} {
		m := New(library.Generate(20, 80))
		m.sort = library.TitleDescending
		m.mode = modeSort
		m.overlayCursor = sortFieldIndex(m.sort)
		content := stripStyles(m.renderOverlay("", size.width, 24))
		if !strings.Contains(content, "› ● t  Title Z-A") || !strings.Contains(content, "  a  Artist A-Z") || !strings.Contains(content, "  e  Relevance") || !strings.Contains(content, size.want) {
			t.Fatalf("width %d overlay = %q", size.width, content)
		}
		for _, line := range strings.Split(content, "\n") {
			if ansi.StringWidth(line) > size.width {
				t.Fatalf("overlay line exceeds width %d: %q", size.width, line)
			}
		}
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

func categoryTestTracks() []library.Track {
	return []library.Track{
		{ID: "video", Artist: "One", Title: "One", Variants: []library.Variant{{ID: "one-video", Category: library.MusicVideo}}},
		{ID: "live", Artist: "Two", Title: "Two", Variants: []library.Variant{{ID: "two-live", Category: library.BandLive}}},
		{ID: "remix", Artist: "Three", Title: "Three", Variants: []library.Variant{{ID: "three-remix", Category: library.Remix}}},
	}
}

// shiftLetter builds Shift+letter the way Bubble Tea's legacy decoder does:
// lowercase Code, Shift modifier, uppercase Text.
func shiftLetter(t *testing.T, m Model, letter rune) Model {
	t.Helper()
	next, _ := m.Update(tea.KeyPressMsg{Code: letter, Mod: tea.ModShift, Text: strings.ToUpper(string(letter))})
	return next.(Model)
}

func enabledCategories(m Model) []library.Category {
	var enabled []library.Category
	for _, category := range library.Categories {
		if m.enabled[category] {
			enabled = append(enabled, category)
		}
	}
	return enabled
}

func filteredIDs(m Model) []string {
	ids := make([]string, 0, len(m.filtered))
	for _, track := range m.filtered {
		ids = append(ids, track.ID)
	}
	slices.Sort(ids)
	return ids
}

func TestCategoryLetterTogglesCategoryAndKeepsViewOpen(t *testing.T) {
	m := New(categoryTestTracks())
	m = updateKey(t, m, "c")
	m = updateKey(t, m, "b")
	if m.mode != modeCategories || !slices.Equal(enabledCategories(m), []library.Category{library.MusicVideo, library.BandLive}) {
		t.Fatalf("after b: mode %s, enabled %v", m.mode, enabledCategories(m))
	}
	if got := filteredIDs(m); !slices.Equal(got, []string{"live", "video"}) {
		t.Fatalf("results not refreshed after b: %v", got)
	}
	m = updateKey(t, m, "m")
	if !slices.Equal(enabledCategories(m), []library.Category{library.BandLive}) || !slices.Equal(filteredIDs(m), []string{"live"}) {
		t.Fatalf("after m: enabled %v, results %v", enabledCategories(m), filteredIDs(m))
	}
}

func TestShiftLetterSolosCategoryAndSecondPressRestoresPriorSet(t *testing.T) {
	m := New(categoryTestTracks())
	m = updateKey(t, m, "c")
	m = updateKey(t, m, "b")
	prior := []library.Category{library.MusicVideo, library.BandLive}

	m = shiftLetter(t, m, 'x')
	if m.mode != modeCategories || !slices.Equal(enabledCategories(m), []library.Category{library.Remix}) || !slices.Equal(filteredIDs(m), []string{"remix"}) {
		t.Fatalf("solo Remix: mode %s, enabled %v, results %v", m.mode, enabledCategories(m), filteredIDs(m))
	}
	m = shiftLetter(t, m, 'x')
	if !slices.Equal(enabledCategories(m), prior) || !slices.Equal(filteredIDs(m), []string{"live", "video"}) {
		t.Fatalf("restore: enabled %v, results %v", enabledCategories(m), filteredIDs(m))
	}

	// A solo while another is active remembers that solo, not the older set.
	m = shiftLetter(t, m, 'x')
	m = shiftLetter(t, m, 'b')
	m = shiftLetter(t, m, 'b')
	if !slices.Equal(enabledCategories(m), []library.Category{library.Remix}) {
		t.Fatalf("chained solo restored %v, want the Remix solo it replaced", enabledCategories(m))
	}
}

func TestShiftLetterOnOnlyEnabledCategoryWithNothingRememberedIsNoOp(t *testing.T) {
	m := New(categoryTestTracks())
	m = updateKey(t, m, "c")
	m = shiftLetter(t, m, 'm')
	if m.mode != modeCategories || !slices.Equal(enabledCategories(m), []library.Category{library.MusicVideo}) {
		t.Fatalf("mode %s, enabled %v", m.mode, enabledCategories(m))
	}
	// Restoring consumes the memory: once the owner is back to a lone category
	// by toggling, Shift+letter must not resurrect the set from an old solo.
	m = updateKey(t, m, "b")
	m = shiftLetter(t, m, 'x')
	m = shiftLetter(t, m, 'x')
	m = updateKey(t, m, "b")
	m = shiftLetter(t, m, 'm')
	if !slices.Equal(enabledCategories(m), []library.Category{library.MusicVideo}) {
		t.Fatalf("enabled %v after consumed restore", enabledCategories(m))
	}
}

func TestResetFiltersForgetsTheSetASoloReplaced(t *testing.T) {
	m := New(categoryTestTracks())
	m = updateKey(t, m, "c")
	m = updateKey(t, m, "b")
	m = shiftLetter(t, m, 'x')
	m = updateKey(t, m, "c")
	m = updateKey(t, m, "f")
	m = updateKey(t, m, "r")
	m = updateKey(t, m, "c")
	m = shiftLetter(t, m, 'm')
	if !slices.Equal(enabledCategories(m), []library.Category{library.MusicVideo}) {
		t.Fatalf("enabled %v after reset, want the reset Music Video set kept", enabledCategories(m))
	}
}

func TestCategoriesViewClosesWithCAndEscape(t *testing.T) {
	for _, key := range []string{"c", "esc"} {
		m := New(categoryTestTracks())
		m = updateKey(t, m, "c")
		m = updateKey(t, m, key)
		if m.mode != modeNavigate {
			t.Fatalf("%s left mode %s", key, m.mode)
		}
	}
}

func TestCategoryKeysCoverEveryCategoryWithoutCollidingWithViewKeys(t *testing.T) {
	seen := map[rune]library.Category{}
	for _, category := range library.Categories {
		letter, ok := categoryKeys[category]
		if !ok {
			t.Fatalf("%s has no letter", category)
		}
		if letter < 'a' || letter > 'z' || strings.ContainsRune("jkc", letter) {
			t.Fatalf("%s letter %q is not a free lowercase letter", category, letter)
		}
		if other, dup := seen[letter]; dup {
			t.Fatalf("%s and %s share %q", category, other, letter)
		}
		seen[letter] = category
	}
}

func TestCategoriesOverlayShowsLettersAndFittingFooter(t *testing.T) {
	for _, size := range []struct {
		width int
		want  string
	}{{40, "key toggle  Shift solo"}, {80, "letter toggle • Shift+letter solo • c/esc close"}, {120, "j/k move  •  space/letter toggle  •  Shift+letter solo  •  c/esc close"}} {
		m := New(categoryTestTracks())
		m.mode = modeCategories
		content := stripStyles(m.renderOverlay("", size.width, 24))
		if !strings.Contains(content, "● m  Music Video") || !strings.Contains(content, "○ x  Remix") || !strings.Contains(content, size.want) {
			t.Fatalf("width %d overlay = %q", size.width, content)
		}
		for _, line := range strings.Split(content, "\n") {
			if ansi.StringWidth(line) > size.width {
				t.Fatalf("overlay line exceeds width %d: %q", size.width, line)
			}
		}
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
	for index, row := range m.rows {
		if row.isVariant() && m.filtered[row.trackIndex].Variants[row.variantIndex].ID == tracks[0].Variants[1].ID {
			m.cursor = index
			break
		}
	}
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

func TestMappingPickerTreatsJKAsSearchText(t *testing.T) {
	m := Model{
		mode:              modeMappingPicker,
		mappingCandidates: []updater.Audio{{Path: "one"}, {Path: "two"}},
	}
	next, command := m.handleMappingPickerKey(tea.KeyPressMsg{Code: 'k', Text: "k"})
	m = next.(Model)
	if m.mappingQuery != "k" || m.mappingCursor != 0 || command == nil {
		t.Fatalf("plain k query = %q, cursor = %d, command = %v", m.mappingQuery, m.mappingCursor, command)
	}
	next, _ = m.handleMappingPickerKey(tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl})
	m = next.(Model)
	if m.mappingCursor != 1 {
		t.Fatalf("ctrl+j cursor = %d", m.mappingCursor)
	}
	next, _ = m.handleMappingPickerKey(tea.KeyPressMsg{Code: tea.KeyUp})
	m = next.(Model)
	if m.mappingCursor != 0 {
		t.Fatalf("up cursor = %d", m.mappingCursor)
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
	case "esc":
		message = tea.KeyPressMsg{Code: tea.KeyEscape}
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
	case "ctrl+a":
		message = tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl}
	default:
		message = tea.KeyPressMsg{Code: rune(key[0]), Text: key}
	}
	next, _ := m.Update(message)
	return next.(Model)
}
