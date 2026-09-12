package ui

import (
	"errors"
	"slices"
	"testing"

	"playlistmaker/charm/internal/library"
)

func TestLibraryReloadReconcilesQueuedRelinksAndRemovals(t *testing.T) {
	tracks := library.Generate(2, 4)
	relinked, removed, retained := tracks[0].Variants[0], tracks[0].Variants[1], tracks[1].Variants[0]
	m := New(tracks, &playbackStub{})
	for _, variant := range []library.Variant{retained, removed, relinked} {
		m.queued[variant.ID] = variant
		m.queueOrder = append(m.queueOrder, variant.ID)
	}
	fresh := cloneTracks(tracks)
	relinked.TrackID = fresh[1].ID
	fresh[0].Variants = nil
	fresh[1].Variants = append(fresh[1].Variants, relinked)
	command := reloadLibraryCmd(&mappingUpdaterStub{tracks: fresh})
	next, _ := m.Update(command())
	m = next.(Model)
	if !slices.Equal(m.queueOrder, []string{retained.ID, relinked.ID}) || len(m.queued) != 2 {
		t.Fatalf("queue after reload = %v, %v", m.queueOrder, m.queued)
	}
	if got := m.queued[relinked.ID]; got.TrackID != fresh[1].ID || got.VideoPath != relinked.VideoPath {
		t.Fatalf("queued relink = %#v", got)
	}
	if m.playback != nil || len(m.all[0].Variants) != 0 {
		t.Fatal("reload did not replace the library and playback source")
	}
}

func TestFailedLibraryReloadPreservesLibraryAndQueue(t *testing.T) {
	launcher := &playbackStub{}
	tracks := library.Generate(1, 1)
	m := New(tracks, launcher)
	variant := tracks[0].Variants[0]
	m.queued[variant.ID], m.queueOrder = variant, []string{variant.ID}
	next, _ := m.Update(libraryReloadMsg{err: errors.New("catalogue unavailable")})
	m = next.(Model)
	if len(m.all) != 1 || m.all[0].ID != tracks[0].ID || m.playback != launcher ||
		!slices.Equal(m.queueOrder, []string{variant.ID}) || len(m.queued) != 1 {
		t.Fatal("failed reload replaced the active library or queue")
	}
	if m.status != "Library reload failed: catalogue unavailable" {
		t.Fatalf("status = %q", m.status)
	}
}
