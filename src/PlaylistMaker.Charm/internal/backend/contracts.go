// Package backend contains the small contracts used by the Charm UI.
// Implementations may be the temporary C# bridge or native Go services.
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
	Shuffle          bool
	MaximumItems     int
	RepeatEach       int
	OneVideoPerTrack bool
}

func DefaultPlaybackOptions() PlaybackOptions {
	return PlaybackOptions{RepeatEach: 1}
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
