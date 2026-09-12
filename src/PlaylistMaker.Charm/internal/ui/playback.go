package ui

import (
	"context"
	"fmt"
	"os"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"playlistmaker/charm/internal/backend"
	"playlistmaker/charm/internal/library"
)

type PlaybackLauncher = backend.PlaybackService

type playbackResultMsg struct {
	count      int
	err        error
	queued     bool
	queueOrder []string
}

type trackingErrorTick struct{}

// Detached helpers cannot write to the TUI; surface their failures here.
func (m Model) WithTrackingErrors(path string) Model {
	m.trackingErrorPath, m.trackingErrorSince = path, time.Now()
	return m
}

func (m Model) trackingErrorCmd() tea.Cmd {
	if m.trackingErrorPath == "" {
		return nil
	}
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return trackingErrorTick{} })
}

func (m Model) handlePlaybackResult(message playbackResultMsg) (tea.Model, tea.Cmd) {
	m.launching = false
	if message.err != nil {
		m.status = "Playback failed: " + message.err.Error()
		return m, nil
	}
	if message.queued && slices.Equal(m.queueOrder, message.queueOrder) {
		m.queued = make(map[string]library.Variant)
		m.queueOrder = nil
	}
	m.status = fmt.Sprintf("Launched %d video(s)", message.count)
	return m, nil
}

func (m Model) handleTrackingErrorTick() (tea.Model, tea.Cmd) {
	if info, err := os.Stat(m.trackingErrorPath); err == nil && info.ModTime().After(m.trackingErrorSince) {
		if contents, err := os.ReadFile(m.trackingErrorPath); err == nil {
			m.trackingError = string(contents)
			m.trackingErrorSince = info.ModTime()
		}
	}
	return m, m.trackingErrorCmd()
}

func (m Model) launchQueue() (tea.Model, tea.Cmd) {
	if len(m.queueOrder) == 0 {
		return m.launchHighlighted()
	}
	return m.launchIDs(append([]string(nil), m.queueOrder...), true)
}

func (m Model) launchHighlighted() (tea.Model, tea.Cmd) {
	current, ok := m.currentRow()
	if !ok {
		m.status = "No selected media"
		return m, nil
	}
	track := m.filtered[current.trackIndex]
	variant := library.Variant{}
	if current.isVariant() {
		variant = track.Variants[current.variantIndex]
	} else {
		var found bool
		variant, found = m.selectVariant(library.EligibleVariants(track, m.currentQuery()))
		if !found {
			m.status = "No eligible media"
			return m, nil
		}
	}
	return m.launchIDs([]string{variant.ID}, false)
}

func (m Model) launchIDs(ids []string, queued bool) (tea.Model, tea.Cmd) {
	if m.launching {
		m.status = "Playback launch already in progress"
		return m, nil
	}
	if len(ids) == 0 {
		m.status = "Queue is empty"
		return m, nil
	}
	if m.playback == nil {
		m.status = "Playback is unavailable"
		return m, nil
	}
	m.launching = true
	variants := m.queued
	if !queued {
		variants = m.variantIndex()
	}
	m.status = fmt.Sprintf("Launching %d planned video(s)…", plannedCount(ids, variants, m.playbackOptions))
	return m, func() tea.Msg {
		result, err := m.playback.Launch(context.Background(), backend.PlaybackRequest{
			VideoIDs: ids,
			Options:  m.playbackOptions,
		})
		if err == nil && !result.Succeeded {
			err = fmt.Errorf("%s", result.UserSafeError)
		}
		return playbackResultMsg{count: result.PlannedVideoCount, err: err, queued: queued, queueOrder: append([]string(nil), ids...)}
	}
}

func (m Model) selectVariant(candidates []library.Variant) (library.Variant, bool) {
	return library.SelectVariant(candidates, m.playbackOptions.SelectionStrategy)
}

func (m Model) plannedPreview() string {
	queued := len(m.queueOrder)
	if queued == 0 {
		return "Queue is empty"
	}
	planned := plannedCount(m.queueOrder, m.queued, m.draftOptions)
	spotifyCount, foobarCount, untrackedCount := m.plannedSourceCounts(m.draftOptions)
	return fmt.Sprintf("%d queued → %d plays • Spotify %d • foobar %d • untracked %d", queued, planned, spotifyCount, foobarCount, untrackedCount)
}

func (m Model) plannedSourceCounts(options backend.PlaybackOptions) (int, int, int) {
	tracks := make(map[string]library.Track, len(m.all))
	for _, track := range m.all {
		tracks[track.ID] = track
	}
	trackIDs := []string{}
	seen := map[string]bool{}
	for _, id := range m.queueOrder {
		variant, ok := m.queued[id]
		if !ok || options.OneVideoPerTrack && seen[variant.TrackID] {
			continue
		}
		seen[variant.TrackID] = true
		trackIDs = append(trackIDs, variant.TrackID)
	}
	if options.MaximumItems > 0 && len(trackIDs) > options.MaximumItems {
		trackIDs = trackIDs[:options.MaximumItems]
	}
	spotifyCount, foobarCount, untrackedCount := 0, 0, 0
	for _, id := range trackIDs {
		track := tracks[id]
		switch {
		case track.SpotifyURI != "":
			spotifyCount++
		case track.LocalAudioPath != "":
			foobarCount++
		default:
			untrackedCount++
		}
	}
	return saturatingMultiply(spotifyCount, max(options.RepeatEach, 1)), saturatingMultiply(foobarCount, max(options.RepeatEach, 1)), saturatingMultiply(untrackedCount, max(options.RepeatEach, 1))
}
