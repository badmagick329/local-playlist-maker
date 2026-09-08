package ui

import (
	tea "charm.land/bubbletea/v2"
	"playlistmaker/charm/internal/library"
	"playlistmaker/charm/internal/updater"
	"testing"
)

func TestMappingPickerDebouncesAndReadsOnce(t *testing.T) {
	stub := &mappingUpdaterStub{candidates: []updater.Audio{{Path: "one", Artist: "Artist", Title: "Song"}, {Path: "two", Artist: "Other", Title: "Track"}}}
	m := New(nil, nil).WithMappingUpdater(stub)
	next, load := m.openMappingPicker("")
	m = next.(Model)
	next, _ = m.Update(load())
	m = next.(Model)
	next, _ = m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = next.(Model)
	stale := mappingDebounceMsg{m.mappingSession, m.mappingRevision}
	next, _ = m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	m = next.(Model)
	next, cmd := m.Update(stale)
	m = next.(Model)
	if cmd != nil || !m.mappingPending || len(m.mappingCandidates) != 0 {
		t.Fatal("stale debounce applied")
	}
	next, _ = m.Update(mappingDebounceMsg{m.mappingSession, m.mappingRevision})
	m = next.(Model)
	if m.mappingPending || len(m.mappingCandidates) != 1 || m.mappingCandidates[0].Path != "one" {
		t.Fatalf("latest search = %#v", m.mappingCandidates)
	}
	if stub.searchCalls != 1 {
		t.Fatalf("catalogue reads = %d", stub.searchCalls)
	}
}

func TestMappingPickerRejectsPreviousSessionAndClosedResults(t *testing.T) {
	stub := &mappingUpdaterStub{}
	m := New(nil, nil).WithMappingUpdater(stub)
	next, first := m.openMappingPicker("")
	m = next.(Model)
	next, _ = m.openMappingPicker("new")
	m = next.(Model)
	next, cmd := m.Update(first())
	m = next.(Model)
	if cmd != nil || !m.mappingLoading {
		t.Fatal("old session load applied")
	}
	m = updateKey(t, m, "esc")
	next, cmd = m.Update(mappingSearchMsg{session: m.mappingSession, items: []updater.Audio{{Path: "late"}}})
	if cmd != nil || len(next.(Model).mappingCandidates) != 0 {
		t.Fatal("closed picker accepted results")
	}
}

func TestRelinkSelectedVideoSavesAndRefreshesQueuedIdentity(t *testing.T) {
	video := library.Variant{ID: "video", VideoPath: "video.mkv", Filename: "video.mkv", TrackID: "old", Category: library.MusicVideo}
	old := library.Track{ID: "old", Artist: "Artist", Title: "Song", Variants: []library.Variant{video}}
	updated := video
	updated.TrackID = "target"
	target := library.Track{ID: "target", Artist: "Artist", Title: "Song", Variants: []library.Variant{updated}}
	stub := &mappingUpdaterStub{candidates: []updater.Audio{{Path: "target", Artist: "Artist", Title: "Song"}}, tracks: []library.Track{target}}
	m := New([]library.Track{old}, nil).WithMappingUpdater(stub)
	m = updateKey(t, m, "l")
	m.cursor = 1
	m.queued[video.ID], m.queueOrder = video, []string{video.ID}
	next, load := m.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	m = next.(Model)
	if !m.mappingRelink || m.mappingQuery != "Artist Song" || load == nil {
		t.Fatal("relink did not open picker")
	}
	next, _ = m.Update(load())
	m = next.(Model)
	next, _ = m.Update(mappingDebounceMsg{m.mappingSession, m.mappingRevision})
	m = next.(Model)
	next, save := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if save == nil {
		t.Fatal("selection did not save")
	}
	next, duplicate := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if duplicate != nil {
		t.Fatal("duplicate save")
	}
	next, reload := next.(Model).Update(save())
	m = next.(Model)
	if stub.confirmedVideo != "video.mkv" || stub.confirmedTrack != "target" || reload == nil || m.mode != modeNavigate {
		t.Fatal("wrong relink target or no reload")
	}
	next, _ = m.Update(reload())
	m = next.(Model)
	if m.queued[video.ID].TrackID != "target" || len(m.queueOrder) != 1 || m.all[0].ID != "target" {
		t.Fatal("library/queue kept old identity")
	}
}

func TestRelinkRequiresVideoAndCanCancel(t *testing.T) {
	stub := &mappingUpdaterStub{}
	m := New(library.Generate(1, 2), nil).WithMappingUpdater(stub)
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	m = next.(Model)
	if cmd != nil || m.mode != modeNavigate {
		t.Fatal("track row relinked an arbitrary video")
	}
	m = updateKey(t, m, "l")
	m.cursor = 1
	next, _ = m.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	m = updateKey(t, next.(Model), "esc")
	if m.mode != modeNavigate || stub.confirms != 0 {
		t.Fatal("cancel saved a link")
	}
}

func TestMappingPickerCannotChooseStaleResultsWhileTyping(t *testing.T) {
	stub := &mappingUpdaterStub{}
	m := New(nil, nil).WithMappingUpdater(stub)
	m.mode, m.mappingRelink = modeMappingPicker, true
	m.mappingItems = []updater.Item{{VideoPath: "video.mkv"}}
	m.mappingCandidates = []updater.Audio{{Path: "old"}}
	next, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = next.(Model)
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil || next.(Model).mappingSaving || stub.confirms != 0 {
		t.Fatal("selected results for an older query")
	}
}
