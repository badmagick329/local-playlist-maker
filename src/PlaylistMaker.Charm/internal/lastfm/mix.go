package lastfm

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"time"

	"playlistmaker/charm/internal/library"
)

type mixCandidate struct {
	track                   library.Track
	plays                   int
	periodLast, overallLast time.Time
}
type randSource struct{ value *rand.Rand }

func (r randSource) Intn(n int) int { return r.value.Intn(n) }

func (s *Service) BuildMix(request MixRequest) (MixResult, error) {
	if request.Count <= 0 {
		return MixResult{}, fmt.Errorf("track count must be positive")
	}
	if request.SecondaryPercent < 0 || request.SecondaryPercent > 100 {
		return MixResult{}, fmt.Errorf("secondary percentage must be between 0 and 100")
	}
	rng := s.Random
	if rng == nil {
		rng = randSource{rand.New(rand.NewSource(time.Now().UnixNano()))}
	}
	excluded := map[string]bool{}
	if request.Action == AppendQueue {
		for id := range request.QueuedTrackIDs {
			excluded[id] = true
		}
	}
	primary := s.periodCandidates(request.Tracks, request.Query, request.Primary, excluded)
	secondaryCount := 0
	if request.Secondary != nil {
		secondaryCount = int(math.Round(float64(request.Count*request.SecondaryPercent) / 100))
	}
	primaryCount := request.Count - secondaryCount
	chosenP := selectCandidates(primary, primaryCount, request.Method, rng, excluded)
	used := copySet(excluded)
	for _, v := range chosenP {
		used[v.track.ID] = true
	}
	secondary := s.periodCandidates(request.Tracks, request.Query, request.Secondary, used)
	chosenS := selectCandidates(secondary, secondaryCount, request.Method, rng, used)
	for _, v := range chosenS {
		used[v.track.ID] = true
	}
	if missing := primaryCount - len(chosenP); missing > 0 {
		var source []mixCandidate
		if request.Secondary != nil {
			source = s.periodCandidates(request.Tracks, request.Query, request.Secondary, used)
		}
		more := selectCandidates(source, missing, request.Method, rng, used)
		chosenS = append(chosenS, more...)
		for _, v := range more {
			used[v.track.ID] = true
		}
	}
	if missing := secondaryCount - len(chosenS); missing > 0 {
		more := selectCandidates(s.periodCandidates(request.Tracks, request.Query, request.Primary, used), missing, request.Method, rng, used)
		chosenP = append(chosenP, more...)
		for _, v := range more {
			used[v.track.ID] = true
		}
	}
	if missing := request.Count - len(chosenP) - len(chosenS); missing > 0 {
		combined := s.periodCandidates(request.Tracks, request.Query, request.Primary, used)
		if request.Secondary != nil {
			combined = append(combined, s.periodCandidates(request.Tracks, request.Query, request.Secondary, used)...)
		}
		dedup := map[string]mixCandidate{}
		for _, v := range combined {
			dedup[v.track.ID] = v
		}
		combined = combined[:0]
		for _, v := range dedup {
			combined = append(combined, v)
		}
		more := selectCandidates(combined, missing, request.Method, rng, used)
		chosenP = append(chosenP, more...)
	}
	ordered := balancedMerge(chosenP, chosenS)
	result := MixResult{Requested: request.Count}
	for _, v := range ordered {
		if variant, ok := library.SelectVariant(library.EligibleVariants(v.track, request.Query), request.SelectionStrategy); ok {
			result.Variants = append(result.Variants, variant)
		}
	}
	result.Created = len(result.Variants)
	return result, nil
}
func (s *Service) periodCandidates(tracks []library.Track, query library.Query, period *library.DateRange, excluded map[string]bool) []mixCandidate {
	result := []mixCandidate{}
	for _, t := range tracks {
		if excluded[t.ID] || len(library.EligibleVariants(t, query)) == 0 {
			continue
		}
		plays := s.index.TrackPlays[t.ID]
		count := 0
		var last time.Time
		for _, at := range plays {
			if period == nil || period.Contains(at) {
				count++
				last = at
			}
		}
		if count > 0 {
			result = append(result, mixCandidate{track: t, plays: count, periodLast: last, overallLast: plays[len(plays)-1]})
		}
	}
	return result
}
func selectCandidates(values []mixCandidate, count int, method MixMethod, rng RandomSource, excluded map[string]bool) []mixCandidate {
	available := append([]mixCandidate(nil), values...)
	available = slicesWithoutExcluded(available, excluded)
	if count > len(available) {
		count = len(available)
	}
	switch method {
	case TopPlayed:
		sort.Slice(available, func(i, j int) bool {
			if available[i].plays != available[j].plays {
				return available[i].plays > available[j].plays
			}
			if !available[i].periodLast.Equal(available[j].periodLast) {
				return available[i].periodLast.After(available[j].periodLast)
			}
			return available[i].track.ID < available[j].track.ID
		})
		return available[:count]
	case Rediscover:
		sort.Slice(available, func(i, j int) bool {
			if !available[i].overallLast.Equal(available[j].overallLast) {
				return available[i].overallLast.Before(available[j].overallLast)
			}
			if available[i].plays != available[j].plays {
				return available[i].plays > available[j].plays
			}
			return available[i].track.ID < available[j].track.ID
		})
		return available[:count]
	case UniformRandom:
		for i := len(available) - 1; i > 0; i-- {
			j := rng.Intn(i + 1)
			available[i], available[j] = available[j], available[i]
		}
		return available[:count]
	default:
		result := make([]mixCandidate, 0, count)
		for len(result) < count {
			total := 0
			for _, v := range available {
				total += v.plays
			}
			pick := rng.Intn(total)
			index := 0
			for ; index < len(available); index++ {
				pick -= available[index].plays
				if pick < 0 {
					break
				}
			}
			result = append(result, available[index])
			available = append(available[:index], available[index+1:]...)
		}
		return result
	}
}
func balancedMerge(primary, secondary []mixCandidate) []mixCandidate {
	total := len(primary) + len(secondary)
	result := make([]mixCandidate, 0, total)
	p, s, inserted := 0, 0, 0
	for pos := 0; pos < total; pos++ {
		expected := int(math.Round(float64((pos+1)*len(secondary)) / float64(total)))
		if expected > inserted && s < len(secondary) {
			result = append(result, secondary[s])
			s++
			inserted++
		} else if p < len(primary) {
			result = append(result, primary[p])
			p++
		} else {
			result = append(result, secondary[s])
			s++
			inserted++
		}
	}
	return result
}
func slicesWithoutExcluded(v []mixCandidate, excluded map[string]bool) []mixCandidate {
	result := v[:0]
	seen := map[string]bool{}
	for _, item := range v {
		if !excluded[item.track.ID] && !seen[item.track.ID] {
			seen[item.track.ID] = true
			result = append(result, item)
		}
	}
	return result
}
func copySet(v map[string]bool) map[string]bool {
	r := map[string]bool{}
	for k, b := range v {
		r[k] = b
	}
	return r
}
