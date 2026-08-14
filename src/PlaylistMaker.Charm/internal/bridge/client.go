package bridge

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"playlistmaker/charm/internal/library"
)

type Client struct {
	command *exec.Cmd
	input   io.WriteCloser
	output  *bufio.Scanner
	stderr  bytes.Buffer
	mu      sync.Mutex
	nextID  int
}

type response struct {
	ID     int             `json:"id"`
	Type   string          `json:"type"`
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result"`
	Error  string          `json:"error"`
}

type snapshot struct {
	SchemaVersion int        `json:"schemaVersion"`
	Tracks        []trackDTO `json:"tracks"`
}

type trackDTO struct {
	ID          string       `json:"id"`
	Artist      string       `json:"artist"`
	Title       string       `json:"title"`
	ReleaseDate string       `json:"releaseDate"`
	History     historyDTO   `json:"history"`
	Variants    []variantDTO `json:"variants"`
}

type variantDTO struct {
	ID            string     `json:"id"`
	VideoPath     string     `json:"videoPath"`
	AudioPath     string     `json:"audioPath"`
	FileName      string     `json:"fileName"`
	Category      string     `json:"category"`
	VideoDate     string     `json:"videoDate"`
	ModifiedAtUTC time.Time  `json:"modifiedAtUtc"`
	History       historyDTO `json:"history"`
}

type historyDTO struct {
	PlayedCount     int        `json:"playedCount"`
	CompletedCount  int        `json:"completedCount"`
	StoppedCount    int        `json:"stoppedCount"`
	SkippedCount    int        `json:"skippedCount"`
	LastPlayedAtUTC *time.Time `json:"lastPlayedAtUtc"`
}

type playRequest struct {
	ID       int             `json:"id"`
	Type     string          `json:"type"`
	VideoIDs []string        `json:"videoIds"`
	Options  playbackOptions `json:"options"`
}

type playbackOptions struct {
	Shuffle          bool `json:"shuffle"`
	MaximumItems     int  `json:"maximumItems"`
	RepeatEach       int  `json:"repeatEach"`
	OneVideoPerTrack bool `json:"oneVideoPerTrack"`
}

type playResult struct {
	Succeeded         bool   `json:"succeeded"`
	PlannedVideoCount int    `json:"plannedVideoCount"`
	Error             string `json:"error"`
}

func Start(executable, configPath string) (*Client, []library.Track, error) {
	command := exec.Command(executable, "--config", configPath)
	input, err := command.StdinPipe()
	if err != nil {
		return nil, nil, err
	}
	output, err := command.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	client := &Client{command: command, input: input, nextID: 1}
	client.output = bufio.NewScanner(output)
	client.output.Buffer(make([]byte, 64*1024), 32*1024*1024)
	command.Stderr = &client.stderr
	if err := command.Start(); err != nil {
		return nil, nil, err
	}

	ready, err := client.readResponse()
	if err != nil {
		client.Close()
		return nil, nil, fmt.Errorf("bridge startup failed: %w: %s", err, client.stderr.String())
	}
	if !ready.OK {
		client.Close()
		return nil, nil, fmt.Errorf("bridge startup failed: %s", ready.Error)
	}
	var data snapshot
	if err := json.Unmarshal(ready.Result, &data); err != nil {
		client.Close()
		return nil, nil, fmt.Errorf("invalid bridge snapshot: %w", err)
	}
	if data.SchemaVersion != 1 {
		client.Close()
		return nil, nil, fmt.Errorf("unsupported bridge schema version %d", data.SchemaVersion)
	}
	return client, mapTracks(data.Tracks), nil
}

func (c *Client) Launch(videoIDs []string) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nextID++
	request := playRequest{
		ID:       c.nextID,
		Type:     "play",
		VideoIDs: videoIDs,
		Options:  playbackOptions{RepeatEach: 1},
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		return 0, err
	}
	if _, err := c.input.Write(append(encoded, '\n')); err != nil {
		return 0, fmt.Errorf("write playback request: %w", err)
	}
	answer, err := c.readResponse()
	if err != nil {
		return 0, fmt.Errorf("read playback response: %w", err)
	}
	if !answer.OK {
		return 0, fmt.Errorf("%s", answer.Error)
	}
	var result playResult
	if err := json.Unmarshal(answer.Result, &result); err != nil {
		return 0, err
	}
	if !result.Succeeded {
		return 0, fmt.Errorf("%s", result.Error)
	}
	return result.PlannedVideoCount, nil
}

func (c *Client) Close() error {
	if c == nil || c.command == nil || c.command.Process == nil {
		return nil
	}
	_ = c.input.Close()
	return c.command.Wait()
}

func (c *Client) readResponse() (response, error) {
	if !c.output.Scan() {
		if err := c.output.Err(); err != nil {
			return response{}, err
		}
		return response{}, io.EOF
	}
	var answer response
	if err := json.Unmarshal(c.output.Bytes(), &answer); err != nil {
		return response{}, err
	}
	return answer, nil
}

func mapTracks(source []trackDTO) []library.Track {
	tracks := make([]library.Track, 0, len(source))
	for _, item := range source {
		variants := make([]library.Variant, 0, len(item.Variants))
		searchByCategory := make(map[library.Category]string)
		modifiedAt := time.Time{}
		newestVideo := time.Time{}
		for _, video := range item.Variants {
			category := library.Category(video.Category)
			videoDate := parseDate(video.VideoDate)
			variants = append(variants, library.Variant{
				ID:         video.ID,
				VideoPath:  video.VideoPath,
				AudioPath:  video.AudioPath,
				Filename:   video.FileName,
				Category:   category,
				Date:       videoDate,
				DateLabel:  video.VideoDate,
				ModifiedAt: video.ModifiedAtUTC,
				History:    mapHistory(video.History),
			})
			searchByCategory[category] += " " + strings.ToLower(video.FileName)
			if video.ModifiedAtUTC.After(modifiedAt) {
				modifiedAt = video.ModifiedAtUTC
			}
			if videoDate.After(newestVideo) {
				newestVideo = videoDate
			}
		}
		tracks = append(tracks, library.Track{
			ID:                   item.ID,
			Artist:               item.Artist,
			Title:                item.Title,
			ReleaseDate:          parseDate(item.ReleaseDate),
			ReleaseDateLabel:     item.ReleaseDate,
			ModifiedAt:           modifiedAt,
			Variants:             variants,
			BaseSearchText:       strings.ToLower(item.Artist + " " + item.Title),
			SearchTextByCategory: searchByCategory,
			NewestVideoDate:      newestVideo,
			History:              mapHistory(item.History),
		})
	}
	return tracks
}

func mapHistory(source historyDTO) library.History {
	return library.History{
		PlayedCount:     source.PlayedCount,
		CompletedCount:  source.CompletedCount,
		StoppedCount:    source.StoppedCount,
		SkippedCount:    source.SkippedCount,
		LastPlayedAtUTC: source.LastPlayedAtUTC,
	}
}

func parseDate(value string) time.Time {
	for _, layout := range []string{"2006-01-02", "2006-01", "2006"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}
