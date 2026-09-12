package ui

import (
	"context"
	"errors"
	"fmt"
	"slices"

	tea "charm.land/bubbletea/v2"

	"playlistmaker/charm/internal/updater"
)

type MappingUpdater interface {
	Scan(context.Context) (updater.ScanResult, error)
	Ignored(context.Context) ([]updater.Item, error)
	Search(context.Context, string) ([]updater.Audio, error)
	Confirm(string, string, bool) error
	Create(string, string, string, bool) error
	Ignore(string) error
	Restore(string) error
	libraryReloader
}

type mappingScanMsg struct {
	result updater.ScanResult
	err    error
}

type mappingIgnoredMsg struct {
	items []updater.Item
	err   error
}

type mappingConfirmMsg struct{ err error }

type mappingIgnoreMsg struct {
	restored bool
	err      error
}

func (m Model) WithMappingUpdater(updater MappingUpdater) Model {
	m.mappingUpdater = updater
	return m
}

func (m Model) handleMappingIgnore(message mappingIgnoreMsg) (tea.Model, tea.Cmd) {
	if message.err != nil {
		m.status = "Ignored video save failed: " + message.err.Error()
		return m, nil
	}
	m.mappingDirty = true
	m.removeCurrentMappingItem()
	if message.restored {
		m.status = "Video restored"
	} else {
		m.status = "Video ignored"
	}
	return m, nil
}

func (m Model) handleMappingConfirm(message mappingConfirmMsg) (tea.Model, tea.Cmd) {
	var duplicate *updater.ExistingTracksError
	if errors.As(message.err, &duplicate) {
		m.mappingSaving = false
		next, cmd := m.openMappingPicker(duplicate.Artist + " " + duplicate.Title)
		m = next.(Model)
		m.mappingShowUnused, m.mappingDuplicate = true, duplicate
		return m, cmd
	}
	m.mappingSaving = false
	if message.err != nil {
		m.status = "Mapping save failed: " + message.err.Error()
		return m, nil
	}
	if m.mappingRelink {
		m.mappingRelink, m.mappingDirty, m.mode = false, true, modeNavigate
		return m, reloadLibraryCmd(m.mappingUpdater)
	}
	if m.mappingDuplicate != nil {
		m.mode, m.mappingDuplicate = modeMappingUpdate, nil
	}
	m.mappingDirty, m.mappingIndex = true, min(m.mappingIndex+1, len(m.mappingItems))
	m.status = "Mapping saved"
	return m, nil
}

func (m Model) handleMappingIgnored(message mappingIgnoredMsg) (tea.Model, tea.Cmd) {
	m.mappingScanning = false
	if message.err != nil {
		m.status = "Ignored videos failed: " + message.err.Error()
	} else {
		m.mappingItems, m.mappingIndex, m.mappingIgnored = message.items, 0, true
		m.status = fmt.Sprintf("%d ignored videos", len(message.items))
	}
	return m, nil
}

func (m Model) handleMappingScan(message mappingScanMsg) (tea.Model, tea.Cmd) {
	m.mappingScanning = false
	if message.err != nil {
		m.status = "Mapping scan failed: " + message.err.Error()
	} else {
		m.mappingItems, m.mappingIndex, m.mappingIgnored = message.result.Items, 0, false
		m.mappingDirty = m.mappingDirty || message.result.Removed > 0
		m.status = mappingSummary(message.result.Items, message.result.Removed)
	}
	return m, nil
}

func (m Model) handleMappingUpdateKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.mappingScanning || m.mappingSaving {
		return m, nil
	}
	switch key.String() {
	case "u", "esc":
		m.mode = modeNavigate
		if m.mappingDirty {
			return m, reloadLibraryCmd(m.mappingUpdater)
		}
		return m, nil
	case "r":
		m.mappingIgnored = false
		m.mappingScanning, m.mappingItems, m.mappingIndex = true, nil, 0
		m.status = "Scanning video and audio folders…"
		return m, m.mappingScanCmd()
	case "s":
		if m.mappingIgnored {
			return m, nil
		}
		m.mappingIndex = min(m.mappingIndex+1, len(m.mappingItems))
		m.status = "Mapping skipped"
		return m, nil
	case "/":
		if m.mappingIgnored {
			return m, nil
		}
		m.mappingRelink = false
		return m.openMappingPicker("")
	case "i":
		item, ok := m.currentMappingItem()
		if !ok {
			return m, nil
		}
		if m.mappingIgnored {
			return m, m.mappingIgnoreCmd(item.VideoPath, true)
		}
		return m, m.mappingIgnoreCmd(item.VideoPath, false)
	case "I":
		m.mappingItems, m.mappingIndex, m.mappingScanning = nil, 0, true
		if m.mappingIgnored {
			m.mappingIgnored = false
			m.status = "Scanning video and audio folders…"
			return m, m.mappingScanCmd()
		}
		m.status = "Loading ignored videos…"
		return m, m.mappingIgnoredCmd()
	case "enter":
		if m.mappingIgnored {
			return m, nil
		}
		item, ok := m.currentMappingItem()
		if !ok {
			return m, nil
		}
		m.mappingSaving = true
		if item.AudioPath == "" {
			return m, m.mappingCreateCmd(item.VideoPath, item.Artist, item.Title, false)
		}
		return m, m.mappingConfirmCmd(item.VideoPath, item.AudioPath, false)
	}
	return m, nil
}

func (m Model) currentMappingItem() (updater.Item, bool) {
	if m.mappingIndex < 0 || m.mappingIndex >= len(m.mappingItems) {
		return updater.Item{}, false
	}
	return m.mappingItems[m.mappingIndex], true
}

func (m Model) mappingScanCmd() tea.Cmd {
	service := m.mappingUpdater
	return func() tea.Msg {
		result, err := service.Scan(context.Background())
		return mappingScanMsg{result: result, err: err}
	}
}

func (m Model) mappingIgnoredCmd() tea.Cmd {
	service := m.mappingUpdater
	return func() tea.Msg {
		items, err := service.Ignored(context.Background())
		return mappingIgnoredMsg{items: items, err: err}
	}
}

func (m Model) mappingConfirmCmd(video, trackID string, allowNew bool) tea.Cmd {
	service := m.mappingUpdater
	return func() tea.Msg { return mappingConfirmMsg{err: service.Confirm(video, trackID, allowNew)} }
}

func (m Model) mappingCreateCmd(video, artist, title string, allowNew bool) tea.Cmd {
	service := m.mappingUpdater
	return func() tea.Msg { return mappingConfirmMsg{err: service.Create(video, artist, title, allowNew)} }
}

func (m Model) mappingIgnoreCmd(video string, restored bool) tea.Cmd {
	service := m.mappingUpdater
	return func() tea.Msg {
		if restored {
			return mappingIgnoreMsg{restored: true, err: service.Restore(video)}
		}
		return mappingIgnoreMsg{err: service.Ignore(video)}
	}
}

func (m *Model) removeCurrentMappingItem() {
	if m.mappingIndex < 0 || m.mappingIndex >= len(m.mappingItems) {
		return
	}
	m.mappingItems = slices.Delete(m.mappingItems, m.mappingIndex, m.mappingIndex+1)
	m.mappingIndex = min(m.mappingIndex, max(len(m.mappingItems)-1, 0))
}

func mappingSummary(items []updater.Item, removed ...int) string {
	suggestions := 0
	for _, item := range items {
		if item.AudioPath != "" {
			suggestions++
		}
	}
	summary := fmt.Sprintf("%d unmapped videos • %d suggestions", len(items), suggestions)
	if len(removed) > 0 && removed[0] > 0 {
		word := "videos"
		if removed[0] == 1 {
			word = "video"
		}
		summary = fmt.Sprintf("%d missing %s removed • %s", removed[0], word, summary)
	}
	return summary
}

func (m Model) mappingUpdateOverlay() (string, []string) {
	var lines []string
	title := "Update mappings"
	if m.mappingIgnored {
		title = "Ignored videos"
	}
	if m.mappingScanning {
		lines = []string{"Scanning video and audio folders…", "", "u/esc close"}
		return title, lines
	}
	item, ok := m.currentMappingItem()
	if !ok {
		if m.mappingIgnored {
			lines = []string{"No ignored videos", "", "I unmapped list • u/esc close"}
		} else {
			lines = []string{"0 unmapped videos • 0 suggestions", "", "r rescan • I ignored list • u/esc close"}
		}
		return title, lines
	}
	if m.mappingIgnored {
		lines = []string{fmt.Sprintf("%d ignored videos • %d of %d", len(m.mappingItems), m.mappingIndex+1, len(m.mappingItems)), "Video: " + item.Filename, "Artist: " + emptyAny(item.Artist), "Title: " + emptyAny(item.Title), "", "i restore • I unmapped list • u/esc close"}
		return title, lines
	}
	lines = []string{mappingSummary(m.mappingItems), fmt.Sprintf("%d of %d", m.mappingIndex+1, len(m.mappingItems)), "Video: " + item.Filename, "Artist: " + emptyAny(item.Artist), "Title: " + emptyAny(item.Title)}
	if item.AudioPath == "" {
		lines = append(lines, "No automatic suggestion; Enter checks existing tracks before creating")
	} else {
		audio := item.AudioArtist + " — " + item.AudioTitle
		if item.AudioArtist == "" && item.AudioTitle == "" {
			audio = "Selected audio track"
		}
		lines = append(lines, "Audio: "+audio, item.Reason)
	}
	lines = append(lines, "", "enter link/create • / choose track • s skip • i ignore • I ignored list • r rescan • u/esc close")
	return title, lines
}
