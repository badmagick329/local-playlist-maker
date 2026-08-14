package ui

import (
	"math"

	"playlistmaker/charm/internal/backend"
	"playlistmaker/charm/internal/library"
	"playlistmaker/charm/internal/pathid"
)

func validateOptions(options backend.PlaybackOptions) backend.PlaybackOptions {
	options.RepeatEach = min(max(options.RepeatEach, 1), 10)
	options.MaximumItems = max(options.MaximumItems, 0)
	return options
}

func plannedCount(queue []string, variants map[string]library.Variant, options backend.PlaybackOptions) int {
	options = validateOptions(options)
	count := len(queue)
	if options.OneVideoPerTrack {
		seen := map[string]bool{}
		count = 0
		for _, id := range queue {
			if item, ok := variants[id]; ok && !seen[pathid.ComparisonKey(item.AudioPath)] {
				seen[pathid.ComparisonKey(item.AudioPath)] = true
				count++
			}
		}
	}
	if count > math.MaxInt/options.RepeatEach {
		count = math.MaxInt
	} else {
		count *= options.RepeatEach
	}
	if options.MaximumItems > 0 {
		count = min(count, options.MaximumItems)
	}
	return count
}
