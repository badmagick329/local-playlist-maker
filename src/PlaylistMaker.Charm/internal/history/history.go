// Package history reads and appends normalized playback history events.
package history

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"playlistmaker/charm/internal/pathid"
)

const HistoryFileName = "play-history.jsonl"

type Event struct {
	SchemaVersion    int       `json:"schemaVersion"`
	Event            string    `json:"event"`
	EventAtUTC       time.Time `json:"eventAtUtc"`
	SessionID        string    `json:"sessionId"`
	EntryID          string    `json:"entryId"`
	PlaylistPosition int       `json:"playlistPosition"`
	PlaylistSize     int       `json:"playlistSize"`
	SelectionSource  string    `json:"selectionSource"`
	TrackID          string    `json:"trackId"`
	VideoPath        string    `json:"videoPath"`
	AudioPath        string    `json:"audioPath"`
	Artist           string    `json:"artist"`
	Title            string    `json:"title"`
	DurationSeconds  *float64  `json:"durationSeconds,omitempty"`
	WatchedSeconds   *float64  `json:"watchedSeconds,omitempty"`
	WatchedPercent   *float64  `json:"watchedPercent,omitempty"`
	EndReason        string    `json:"endReason,omitempty"`
	CountedAsPlayed  *bool     `json:"countedAsPlayed,omitempty"`
}

type Normalized struct {
	Event   Event
	Outcome string
	Percent *float64
	Seconds *float64
	Counted bool
}
type Summary struct {
	Played, Completed, Stopped, Skipped, NotStarted, Abandoned int
	LastPlayed                                                 *time.Time
	LastAttempted                                              *time.Time
	Recent                                                     []Normalized
}
type Index struct {
	Tracks, Videos map[string]Summary
	InvalidLines   int
}

func Read(path string) (Index, error) {
	result := Index{Tracks: map[string]Summary{}, Videos: map[string]Summary{}}
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	defer file.Close()
	terminal := map[string]Event{}
	scanner := bufio.NewScanner(file)
	buffer := make([]byte, 64*1024)
	scanner.Buffer(buffer, 1024*1024)
	for scanner.Scan() {
		var event Event
		if json.Unmarshal(scanner.Bytes(), &event) != nil {
			result.InvalidLines++
			continue
		}
		if !isTerminal(event.Event) || event.SessionID == "" || event.EntryID == "" {
			continue
		}
		key := event.SessionID + "\x00" + event.EntryID
		if current, ok := terminal[key]; !ok || !event.EventAtUTC.Before(current.EventAtUTC) {
			terminal[key] = event
		}
	}
	if err := scanner.Err(); err != nil {
		return result, err
	}
	normalized := make([]Normalized, 0, len(terminal))
	for _, event := range terminal {
		normalized = append(normalized, Normalize(event))
	}
	result.Tracks = summarize(normalized, func(item Normalized) string { return item.Event.TrackID })
	result.Videos = summarize(normalized, func(item Normalized) string { return item.Event.VideoPath })
	return result, nil
}

func Normalize(event Event) Normalized {
	percent := event.WatchedPercent
	if percent != nil {
		value := min(max(*percent, 0), 100)
		percent = &value
	}
	seconds := event.WatchedSeconds
	if seconds != nil && event.DurationSeconds != nil && *event.DurationSeconds > 0 {
		value := min(max(*seconds, 0), *event.DurationSeconds)
		seconds = &value
	}
	eof := strings.EqualFold(event.EndReason, "eof")
	if eof {
		value := float64(100)
		percent = &value
	}
	outcome := event.Event
	if eof || event.Event == "stopped" && percent != nil && *percent >= 90 {
		outcome = "completed"
	}
	return Normalized{Event: event, Outcome: outcome, Percent: percent, Seconds: seconds, Counted: outcome == "completed" || event.CountedAsPlayed != nil && *event.CountedAsPlayed}
}

func summarize(events []Normalized, path func(Normalized) string) map[string]Summary {
	groups := map[string][]Normalized{}
	for _, event := range events {
		if value := path(event); value != "" {
			key := pathid.ComparisonKey(value)
			groups[key] = append(groups[key], event)
		}
	}
	result := map[string]Summary{}
	for key, group := range groups {
		sort.Slice(group, func(i, j int) bool { return group[i].Event.EventAtUTC.After(group[j].Event.EventAtUTC) })
		summary := Summary{}
		for _, item := range group {
			if item.Counted {
				summary.Played++
				if summary.LastPlayed == nil {
					value := item.Event.EventAtUTC
					summary.LastPlayed = &value
				}
			}
			if item.Outcome != "not_started" && summary.LastAttempted == nil {
				value := item.Event.EventAtUTC
				summary.LastAttempted = &value
			}
			switch item.Outcome {
			case "completed":
				summary.Completed++
			case "stopped":
				summary.Stopped++
			case "skipped":
				summary.Skipped++
			case "not_started":
				summary.NotStarted++
			case "abandoned":
				summary.Abandoned++
			}
		}
		summary.Recent = group[:min(5, len(group))]
		result[key] = summary
	}
	return result
}

func isTerminal(value string) bool {
	switch value {
	case "completed", "stopped", "skipped", "not_started", "abandoned":
		return true
	}
	return false
}

type Service struct {
	DataDirectory         string
	MinimumWatchedPercent int
}

func (s Service) HistoryPath() string { return filepath.Join(s.DataDirectory, HistoryFileName) }
func TerminalEntryIDs(path, sessionID string) (map[string]bool, error) {
	terminal := map[string]bool{}
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return terminal, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event Event
		if json.Unmarshal(scanner.Bytes(), &event) == nil && event.SessionID == sessionID && isTerminal(event.Event) {
			terminal[event.EntryID] = true
		}
	}
	return terminal, scanner.Err()
}
func (s Service) Append(event Event) error {
	return Append(s.HistoryPath(), event)
}
func Append(path string, event Event) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	contents, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = file.Write(append(contents, '\n'))
	return err
}
