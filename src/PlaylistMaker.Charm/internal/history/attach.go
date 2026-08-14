package history

import (
	"playlistmaker/charm/internal/library"
	"playlistmaker/charm/internal/pathid"
)

func Attach(tracks []library.Track, index Index) []library.Track {
	for trackIndex := range tracks {
		tracks[trackIndex].History = toLibrary(index.Tracks[pathid.ComparisonKey(tracks[trackIndex].ID)])
		for variantIndex := range tracks[trackIndex].Variants {
			variant := &tracks[trackIndex].Variants[variantIndex]
			variant.History = toLibrary(index.Videos[pathid.ComparisonKey(variant.ID)])
		}
	}
	return tracks
}

func toLibrary(value Summary) library.History {
	result := library.History{PlayedCount: value.Played, CompletedCount: value.Completed, StoppedCount: value.Stopped, SkippedCount: value.Skipped, NotStartedCount: value.NotStarted, AbandonedCount: value.Abandoned, LastPlayedAtUTC: value.LastPlayed}
	for _, event := range value.Recent {
		result.Recent = append(result.Recent, library.HistoryEvent{Outcome: event.Outcome, AtUTC: event.Event.EventAtUTC, Percent: event.Percent})
	}
	return result
}
