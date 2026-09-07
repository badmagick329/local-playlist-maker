package lastfm

import (
	"strings"
	"time"

	"playlistmaker/charm/internal/library"
)

// Presets use scrobbles for song familiarity and local history only for video
// exposure. Counts from the two stores must never be added together.
func (s *Service) buildPreset(r MixRequest, rng RandomSource, excluded map[string]bool) MixResult {
	now := r.Now
	if now.IsZero() {
		now = time.Now()
	}
	cutoff := now.AddDate(0, 0, -30)
	buckets := [3][]mixCandidate{}
	variants := map[string][]library.Variant{}
	for _, track := range r.Tracks {
		if excluded[track.ID] {
			continue
		}
		eligible := library.EligibleVariants(track, r.Query)
		if r.Preset == FamiliarUnseen {
			unseen := []library.Variant{}
			for _, v := range eligible {
				if v.History.PlayedCount == 0 {
					unseen = append(unseen, v)
				}
			}
			eligible = unseen
		}
		if len(eligible) == 0 {
			continue
		}
		plays := s.index.TrackPlays[track.ID]
		if r.Preset == FamiliarUnseen && len(plays) < 3 {
			continue
		}
		variants[track.ID] = eligible
		recent := 0
		for _, at := range plays {
			if !at.Before(cutoff) && !at.After(now) {
				recent++
			}
		}
		bucket, weight := 2, 1
		if r.Preset == FamiliarUnseen {
			bucket, weight = 0, len(plays)
		} else if recent >= 2 {
			bucket, weight = 0, recent
		} else if len(plays) >= 3 {
			bucket, weight = 1, len(plays)
		}
		buckets[bucket] = append(buckets[bucket], mixCandidate{track: track, plays: weight})
	}
	result := MixResult{Requested: r.Count}
	used := copySet(excluded)
	lastArtist := ""
	pattern := []int{0, 1, 0, 1, 2}
	for len(result.Variants) < r.Count {
		bucket := 0
		if r.Preset == BalancedRotation {
			bucket = pattern[len(result.Variants)%len(pattern)]
		}
		candidates := slicesWithoutExcluded(append([]mixCandidate(nil), buckets[bucket]...), used)
		if len(candidates) == 0 {
			for _, values := range buckets {
				candidates = append(candidates, slicesWithoutExcluded(append([]mixCandidate(nil), values...), used)...)
			}
		}
		if len(candidates) == 0 {
			break
		}
		different := []mixCandidate{}
		for _, c := range candidates {
			if !strings.EqualFold(c.track.Artist, lastArtist) {
				different = append(different, c)
			}
		}
		if len(different) > 0 {
			candidates = different
		}
		chosen := selectCandidates(candidates, 1, WeightedRandom, rng, used)[0]
		strategy := r.SelectionStrategy
		if r.Preset == FamiliarUnseen {
			strategy = library.UnseenSelection
		}
		v, _ := library.SelectVariant(variants[chosen.track.ID], strategy)
		result.Variants = append(result.Variants, v)
		used[chosen.track.ID], lastArtist = true, chosen.track.Artist
	}
	result.Created = len(result.Variants)
	return result
}
