package library

import "time"

type SelectionStrategy int

const (
	DefaultSelection SelectionStrategy = iota
	FavouriteSelection
	FreshSelection
	UnseenSelection
)

func (s SelectionStrategy) String() string {
	switch s {
	case FavouriteSelection:
		return "Favourite"
	case FreshSelection:
		return "Fresh"
	case UnseenSelection:
		return "Unseen"
	default:
		return "Default"
	}
}
func (s SelectionStrategy) Next(delta int) SelectionStrategy {
	return SelectionStrategy((int(s) + delta + 4) % 4)
}

func SelectVariant(candidates []Variant, strategy SelectionStrategy) (Variant, bool) {
	if len(candidates) == 0 {
		return Variant{}, false
	}
	best := candidates[0]
	for _, candidate := range candidates[1:] {
		if better(candidate, best, strategy) {
			best = candidate
		}
	}
	return best, true
}

func better(left, right Variant, strategy SelectionStrategy) bool {
	switch strategy {
	case FavouriteSelection:
		return betterFavourite(left, right)
	case FreshSelection:
		return betterFresh(left, right)
	case UnseenSelection:
		leftUnseen, rightUnseen := left.History.PlayedCount == 0, right.History.PlayedCount == 0
		if leftUnseen != rightUnseen {
			return leftUnseen
		}
		if leftUnseen {
			if left.History.SkippedCount != right.History.SkippedCount {
				return left.History.SkippedCount < right.History.SkippedCount
			}
			if left.History.StoppedCount != right.History.StoppedCount {
				return left.History.StoppedCount < right.History.StoppedCount
			}
			if left.History.AbandonedCount != right.History.AbandonedCount {
				return left.History.AbandonedCount < right.History.AbandonedCount
			}
			return betterDefault(left, right)
		}
		return betterFresh(left, right)
	default:
		return betterDefault(left, right)
	}
}

func attempts(v Variant) int {
	return v.History.CompletedCount + v.History.StoppedCount + v.History.SkippedCount + v.History.AbandonedCount
}
func betterFavourite(left, right Variant) bool {
	la, ra := attempts(left), attempts(right)
	if (la > 0) != (ra > 0) {
		return la > 0
	}
	if la > 0 && left.History.CompletedCount*ra != right.History.CompletedCount*la {
		return left.History.CompletedCount*ra > right.History.CompletedCount*la
	}
	if left.History.CompletedCount != right.History.CompletedCount {
		return left.History.CompletedCount > right.History.CompletedCount
	}
	if left.History.PlayedCount != right.History.PlayedCount {
		return left.History.PlayedCount > right.History.PlayedCount
	}
	if left.History.SkippedCount != right.History.SkippedCount {
		return left.History.SkippedCount < right.History.SkippedCount
	}
	if later(left.History.LastPlayedAtUTC, right.History.LastPlayedAtUTC) {
		return true
	}
	if later(right.History.LastPlayedAtUTC, left.History.LastPlayedAtUTC) {
		return false
	}
	return betterDefault(left, right)
}
func betterFresh(left, right Variant) bool {
	la, ra := left.History.LastAttemptedAtUTC, right.History.LastAttemptedAtUTC
	if (la == nil) != (ra == nil) {
		return la == nil
	}
	if la != nil && !la.Equal(*ra) {
		return la.Before(*ra)
	}
	lf, rf := left.History.SkippedCount+left.History.StoppedCount+left.History.AbandonedCount, right.History.SkippedCount+right.History.StoppedCount+right.History.AbandonedCount
	if lf != rf {
		return lf < rf
	}
	return betterDefault(left, right)
}
func later(left, right *time.Time) bool { return left != nil && (right == nil || left.After(*right)) }
func betterDefault(left, right Variant) bool {
	if (left.Category == MusicVideo) != (right.Category == MusicVideo) {
		return left.Category == MusicVideo
	}
	if !left.Date.Equal(right.Date) {
		return left.Date.After(right.Date)
	}
	if !left.ModifiedAt.Equal(right.ModifiedAt) {
		return left.ModifiedAt.After(right.ModifiedAt)
	}
	return left.ID < right.ID
}
