// Package snapshotcmp compares backend snapshots without depending on transport JSON.
package snapshotcmp

import (
	"fmt"

	"playlistmaker/charm/internal/backend"
	"playlistmaker/charm/internal/library"
)

const maxReportedDifferences = 20

type Result struct {
	Total       int
	Differences []string
}

func (r Result) Equal() bool { return r.Total == 0 }

func Compare(reference, candidate backend.LibrarySnapshot) Result {
	result := Result{}
	record := func(format string, values ...any) {
		result.Total++
		if len(result.Differences) < maxReportedDifferences {
			result.Differences = append(result.Differences, fmt.Sprintf(format, values...))
		}
	}
	if len(reference.Tracks) != len(candidate.Tracks) {
		record("track count: bridge=%d go=%d", len(reference.Tracks), len(candidate.Tracks))
	}
	for index := 0; index < min(len(reference.Tracks), len(candidate.Tracks)); index++ {
		left, right := reference.Tracks[index], candidate.Tracks[index]
		prefix := fmt.Sprintf("track[%d] (%s)", index, left.ID)
		compareTrack(record, prefix, left, right)
	}
	return result
}

func compareTrack(record func(string, ...any), prefix string, left, right library.Track) {
	for _, field := range []struct{ name, left, right string }{{"id", left.ID, right.ID}, {"artist", left.Artist, right.Artist}, {"title", left.Title, right.Title}, {"release date", left.ReleaseDateLabel, right.ReleaseDateLabel}} {
		if field.left != field.right {
			record("%s %s: bridge=%q go=%q", prefix, field.name, field.left, field.right)
		}
	}
	if len(left.Variants) != len(right.Variants) {
		record("%s variant count: bridge=%d go=%d", prefix, len(left.Variants), len(right.Variants))
	}
	for index := 0; index < min(len(left.Variants), len(right.Variants)); index++ {
		compareVariant(record, fmt.Sprintf("%s variant[%d]", prefix, index), left.Variants[index], right.Variants[index])
	}
	all := map[library.Category]bool{library.MusicVideo: true}
	leftDefault, leftOK := library.DefaultVariant(left, all)
	rightDefault, rightOK := library.DefaultVariant(right, all)
	if leftOK != rightOK || (leftOK && leftDefault.ID != rightDefault.ID) {
		record("%s default variant: bridge=%q go=%q", prefix, leftDefault.ID, rightDefault.ID)
	}
}

func compareVariant(record func(string, ...any), prefix string, left, right library.Variant) {
	for _, field := range []struct{ name, left, right string }{{"id", left.ID, right.ID}, {"video path", left.VideoPath, right.VideoPath}, {"audio path", left.AudioPath, right.AudioPath}, {"filename", left.Filename, right.Filename}, {"category", string(left.Category), string(right.Category)}, {"video date", left.DateLabel, right.DateLabel}, {"modified UTC", left.ModifiedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"), right.ModifiedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00")}} {
		if field.left != field.right {
			record("%s %s: bridge=%q go=%q", prefix, field.name, field.left, field.right)
		}
	}
}

func OverlayHistory(metadata, history backend.LibrarySnapshot) backend.LibrarySnapshot {
	trackHistory := make(map[string]library.History, len(history.Tracks))
	variantHistory := make(map[string]library.History)
	for _, track := range history.Tracks {
		trackHistory[track.ID] = track.History
		for _, variant := range track.Variants {
			variantHistory[variant.ID] = variant.History
		}
	}
	result := metadata
	result.Tracks = append([]library.Track(nil), metadata.Tracks...)
	for index := range result.Tracks {
		result.Tracks[index].History = trackHistory[result.Tracks[index].ID]
		result.Tracks[index].Variants = append([]library.Variant(nil), result.Tracks[index].Variants...)
		for variantIndex := range result.Tracks[index].Variants {
			result.Tracks[index].Variants[variantIndex].History = variantHistory[result.Tracks[index].Variants[variantIndex].ID]
		}
	}
	return result
}
