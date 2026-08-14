package ui

import (
	"sort"
	"sync"
	"time"
)

const latencyWindow = 240

type latencyStats struct {
	mu      sync.Mutex
	updates []time.Duration
	views   []time.Duration
}

type latencySnapshot struct {
	updateP95 time.Duration
	viewP95   time.Duration
}

func (s *latencyStats) recordUpdate(duration time.Duration) {
	s.mu.Lock()
	s.updates = appendWindow(s.updates, duration)
	s.mu.Unlock()
}

func (s *latencyStats) recordView(duration time.Duration) {
	s.mu.Lock()
	s.views = appendWindow(s.views, duration)
	s.mu.Unlock()
}

func (s *latencyStats) snapshot() latencySnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return latencySnapshot{
		updateP95: percentile95(s.updates),
		viewP95:   percentile95(s.views),
	}
}

func appendWindow(values []time.Duration, value time.Duration) []time.Duration {
	if len(values) == latencyWindow {
		copy(values, values[1:])
		values[len(values)-1] = value
		return values
	}
	return append(values, value)
}

func percentile95(values []time.Duration) time.Duration {
	if len(values) == 0 {
		return 0
	}
	copyOfValues := append([]time.Duration(nil), values...)
	sort.Slice(copyOfValues, func(i, j int) bool { return copyOfValues[i] < copyOfValues[j] })
	index := (len(copyOfValues)*95 + 99) / 100
	if index > 0 {
		index--
	}
	return copyOfValues[index]
}
