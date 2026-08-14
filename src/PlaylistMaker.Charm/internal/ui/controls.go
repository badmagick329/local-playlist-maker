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
	count := 0
	seen := map[string]bool{}
	for _, id := range queue {
		item, ok := variants[id]
		if !ok {
			continue
		}
		if options.OneVideoPerTrack {
			identity := pathid.ComparisonKey(item.AudioPath)
			if seen[identity] {
				continue
			}
			seen[identity] = true
		}
		count++
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
