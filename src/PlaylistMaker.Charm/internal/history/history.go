// Package history reads append-only mpv history and manages Charm sessions.
package history

import (
	"bufio"
	"encoding/json"
	"fmt"
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
	result.Tracks = summarize(normalized, func(item Normalized) string { return item.Event.AudioPath })
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

type SessionEntry struct {
	EntryID          string `json:"entryId"`
	PlaylistPosition int    `json:"playlistPosition"`
	PlaylistSize     int    `json:"playlistSize"`
	SelectionSource  string `json:"selectionSource"`
	VideoPath        string `json:"videoPath"`
	AudioPath        string `json:"audioPath"`
	Artist           string `json:"artist"`
	Title            string `json:"title"`
}
type Session struct {
	SchemaVersion  int            `json:"schemaVersion"`
	SessionID      string         `json:"sessionId"`
	RequestedAtUTC time.Time      `json:"requestedAtUtc"`
	MpvProcessID   *int           `json:"mpvProcessId,omitempty"`
	Entries        []SessionEntry `json:"entries"`
}
type Service struct {
	DataDirectory         string
	MinimumWatchedPercent int
	Now                   func() time.Time
	NewID                 func() string
	IsAlive               func(int) bool
}

func (s Service) HistoryPath() string { return filepath.Join(s.DataDirectory, HistoryFileName) }
func (s Service) sessionPath(id string) string {
	return filepath.Join(s.DataDirectory, "playback-sessions", id+".json")
}
func (s Service) Create(entries []SessionEntry) (Session, error) {
	if len(entries) == 0 {
		return Session{}, fmt.Errorf("cannot create an empty history session")
	}
	newID := s.NewID
	if newID == nil {
		newID = func() string { return fmt.Sprintf("%d", time.Now().UnixNano()) }
	}
	now := s.Now
	if now == nil {
		now = time.Now
	}
	for index := range entries {
		entries[index].EntryID = newID()
		entries[index].PlaylistPosition = index
		entries[index].PlaylistSize = len(entries)
		entries[index].SelectionSource = "charm-tui"
	}
	session := Session{SchemaVersion: 1, SessionID: newID(), RequestedAtUTC: now().UTC(), Entries: entries}
	if err := s.writeSession(session); err != nil {
		return Session{}, err
	}
	return session, nil
}
func (s Service) MpvArguments(session Session) []string {
	return []string{"--script-opt=playlistmaker_history-session_id=" + session.SessionID, "--script-opt=playlistmaker_history-manifest_path=" + s.sessionPath(session.SessionID), "--script-opt=playlistmaker_history-history_path=" + s.HistoryPath(), fmt.Sprintf("--script-opt=playlistmaker_history-minimum_watched_percent=%d", s.MinimumWatchedPercent)}
}
func (s Service) RecordPID(session *Session, pid int) error {
	session.MpvProcessID = &pid
	return s.writeSession(*session)
}
func (s Service) Recover() error {
	directory := filepath.Join(s.DataDirectory, "playback-sessions")
	files, err := filepath.Glob(filepath.Join(directory, "*.json"))
	if err != nil {
		return err
	}
	for _, path := range files {
		var session Session
		contents, readErr := os.ReadFile(path)
		if readErr != nil || json.Unmarshal(contents, &session) != nil || session.SessionID == "" {
			continue
		}
		if session.MpvProcessID != nil && (s.IsAlive == nil || s.IsAlive(*session.MpvProcessID)) {
			continue
		}
		started, terminal := s.sessionEvents(session.SessionID)
		for _, entry := range session.Entries {
			if terminal[entry.EntryID] {
				continue
			}
			outcome := "not_started"
			if started[entry.EntryID] {
				outcome = "abandoned"
			}
			counted := false
			if err := s.Append(Event{SchemaVersion: 2, Event: outcome, EventAtUTC: time.Now().UTC(), SessionID: session.SessionID, EntryID: entry.EntryID, PlaylistPosition: entry.PlaylistPosition, PlaylistSize: entry.PlaylistSize, SelectionSource: entry.SelectionSource, VideoPath: entry.VideoPath, AudioPath: entry.AudioPath, Artist: entry.Artist, Title: entry.Title, EndReason: "mpv-process-exited-without-terminal-event", CountedAsPlayed: &counted}); err != nil {
				return err
			}
		}
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	return nil
}
func (s Service) sessionEvents(id string) (map[string]bool, map[string]bool) {
	started, terminal := map[string]bool{}, map[string]bool{}
	file, err := os.Open(s.HistoryPath())
	if err != nil {
		return started, terminal
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event Event
		if json.Unmarshal(scanner.Bytes(), &event) == nil && event.SessionID == id {
			if event.Event == "started" {
				started[event.EntryID] = true
			}
			if isTerminal(event.Event) {
				terminal[event.EntryID] = true
			}
		}
	}
	return started, terminal
}
func (s Service) Append(event Event) error {
	if err := os.MkdirAll(s.DataDirectory, 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(s.HistoryPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
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
func (s Service) writeSession(session Session) error {
	directory := filepath.Join(s.DataDirectory, "playback-sessions")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	contents, err := json.Marshal(session)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".session-*.tmp")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if _, err = temporary.Write(contents); err == nil {
		err = temporary.Close()
	} else {
		_ = temporary.Close()
	}
	if err != nil {
		return err
	}
	return os.Rename(name, s.sessionPath(session.SessionID))
}
