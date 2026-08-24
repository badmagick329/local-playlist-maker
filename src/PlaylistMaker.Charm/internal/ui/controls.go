package ui

import (
	"math"

	"playlistmaker/charm/internal/backend"
	"playlistmaker/charm/internal/library"
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
			identity := item.TrackID
			if seen[identity] {
				continue
			}
			seen[identity] = true
		}
		count++
	}
	count = saturatingMultiply(count, options.RepeatEach)
	if options.MaximumItems > 0 {
		count = min(count, options.MaximumItems)
	}
	return count
}

func saturatingMultiply(left, right int) int {
	if left == 0 || right == 0 {
		return 0
	}
	if left > math.MaxInt/right {
		return math.MaxInt
	}
	return left * right
}
