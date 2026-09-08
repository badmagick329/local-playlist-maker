package lastfm

import (
	"math"
	"strings"
	"time"

	"playlistmaker/charm/internal/library"
)

// Presets use scrobbles for familiarity and listening trends. Local history
// controls video exposure and resurfacing cooldowns; counts remain separate.
func (s *Service) buildPreset(r MixRequest, rng RandomSource, excluded map[string]bool) MixResult {
	now := r.Now
	if now.IsZero() {
		now = time.Now()
	}
	cutoff := now.AddDate(0, 0, -30)
	var obsessionEnd time.Time
	if r.Preset == CurrentObsessions {
		if len(s.index.Scrobbles) > 0 {
			latest := s.index.Scrobbles[len(s.index.Scrobbles)-1].PlayedAtUTC
			// Whole UTC days keep the displayed history date and scoring window aligned.
			obsessionEnd = latest.UTC().Truncate(24*time.Hour).AddDate(0, 0, 1)
		}
	}
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
		if r.Preset == ForgottenFavourites {
			weight = forgottenWeight(track, plays, now)
			if weight == 0 {
				continue
			}
			bucket = 0
		} else if r.Preset == CurrentObsessions {
			weight = obsessionGrowth(plays, obsessionEnd)
			if weight <= 0 {
				continue
			}
			bucket = 0
		} else if r.Preset == FamiliarUnseen {
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
		if len(different) > 0 && r.Preset != CurrentObsessions {
			candidates = different
		}
		method := WeightedRandom
		if r.Preset == CurrentObsessions {
			method = TopPlayed
		}
		chosen := selectCandidates(candidates, 1, method, rng, used)[0]
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

// A cooldown applies to the song, including attempts of other performances, so
// skipping a rediscovery cannot immediately bring it back as another video.
func forgottenWeight(track library.Track, plays []time.Time, now time.Time) int {
	if len(plays) < 10 {
		return 0
	}
	quietSince, cooldown := now.AddDate(0, -6, 0), now.AddDate(0, 0, -30)
	days := map[string]bool{}
	for _, at := range plays {
		if !at.Before(quietSince) {
			return 0
		}
		days[at.UTC().Format("2006-01-02")] = true
	}
	if len(days) < 5 {
		return 0
	}
	if recentLocalHistory(track.History, quietSince, cooldown) {
		return 0
	}
	for _, v := range track.Variants {
		if recentLocalHistory(v.History, quietSince, cooldown) {
			return 0
		}
	}
	// Logarithmic familiarity keeps former heavy rotations from dominating.
	return 1 + int(math.Log2(float64(len(plays))))
}

func recentLocalHistory(h library.History, quietSince, cooldown time.Time) bool {
	return h.LastPlayedAtUTC != nil && !h.LastPlayedAtUTC.Before(quietSince) ||
		h.LastAttemptedAtUTC != nil && !h.LastAttemptedAtUTC.Before(cooldown)
}

// Compare absolute daily-rate growth, avoiding division by a zero baseline and
// giving new favourites a score without treating a single-day burst as a trend.
func obsessionGrowth(plays []time.Time, end time.Time) int {
	recentStart := end.AddDate(0, 0, -14)
	previousStart := recentStart.AddDate(0, 0, -30)
	recent, previous := 0, 0
	days := map[string]bool{}
	for _, at := range plays {
		if !at.Before(end) || at.Before(previousStart) {
			continue
		}
		if !at.Before(recentStart) {
			recent++
			days[at.UTC().Format("2006-01-02")] = true
		} else {
			previous++
		}
	}
	if len(days) < 2 {
		return 0
	}
	return recent*30 - previous*14
}
