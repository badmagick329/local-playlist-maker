// Package backend contains the service contracts used by the Charm UI.
// The application provides these services through its native Go implementation.
package backend

import (
	"context"

	"playlistmaker/charm/internal/library"
)

type LibrarySnapshot struct {
	Tracks   []library.Track
	Warnings []string
}

type LibraryLoader interface {
	Load(context.Context) (LibrarySnapshot, error)
}

type PlaybackOptions struct {
	Shuffle           bool
	MaximumItems      int
	RepeatEach        int
	OneVideoPerTrack  bool
	SelectionStrategy library.SelectionStrategy
}

func DefaultPlaybackOptions() PlaybackOptions {
	return PlaybackOptions{RepeatEach: 1, SelectionStrategy: library.DefaultSelection}
}

type PlaybackRequest struct {
	VideoIDs []string
	Options  PlaybackOptions
}

type PlaybackResult struct {
	Succeeded         bool
	PlannedVideoCount int
	UserSafeError     string
}

type PlaybackService interface {
	Launch(context.Context, PlaybackRequest) (PlaybackResult, error)
}
