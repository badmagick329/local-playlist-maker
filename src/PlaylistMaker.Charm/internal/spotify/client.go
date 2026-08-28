package spotify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	Auth    *Auth
	HTTP    *http.Client
	APIBase string
}

type RateLimitError struct {
	RetryAfter time.Duration
	Valid      bool
	Value      string
}

type ResponseError struct {
	StatusCode int
	Message    string
}

func (e *ResponseError) Error() string { return e.Message }

func (e *RateLimitError) Error() string {
	if !e.Valid {
		return fmt.Sprintf("Spotify API rate limited request with invalid Retry-After %q", e.Value)
	}
	return fmt.Sprintf("Spotify API rate limited request; retry after %s", e.RetryAfter)
}

type Device struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Restricted bool   `json:"is_restricted"`
}

type Track struct {
	URI        string `json:"uri"`
	Name       string `json:"name"`
	DurationMS int    `json:"duration_ms"`
	IsPlayable *bool  `json:"is_playable"`
	Album      struct {
		Name        string `json:"name"`
		ReleaseDate string `json:"release_date"`
	} `json:"album"`
	Artists []struct {
		Name string `json:"name"`
	} `json:"artists"`
	ExternalIDs map[string]string `json:"external_ids"`
}

func (c *Client) Devices(ctx context.Context) ([]Device, error) {
	var payload struct {
		Devices []Device `json:"devices"`
	}
	if err := c.do(ctx, http.MethodGet, "/me/player/devices", nil, &payload); err != nil {
		return nil, err
	}
	return payload.Devices, nil
}

func (c *Client) Play(ctx context.Context, deviceID, uri string) error {
	return c.do(ctx, http.MethodPut, "/me/player/play?device_id="+url.QueryEscape(deviceID), map[string]any{"uris": []string{uri}, "position_ms": 0}, nil)
}

func (c *Client) Pause(ctx context.Context, deviceID string) error {
	return c.do(ctx, http.MethodPut, "/me/player/pause?device_id="+url.QueryEscape(deviceID), nil, nil)
}

type PlaybackState struct {
	IsPlaying bool `json:"is_playing"`
	Device    struct {
		ID string `json:"id"`
	} `json:"device"`
}

func (c *Client) CurrentPlayback(ctx context.Context) (PlaybackState, error) {
	var state PlaybackState
	if err := c.do(ctx, http.MethodGet, "/me/player", nil, &state); err != nil {
		return PlaybackState{}, err
	}
	return state, nil
}

func (c *Client) Search(ctx context.Context, query string, limit int) ([]Track, error) {
	if limit < 1 {
		limit = 1
	}
	if limit > 10 {
		limit = 10
	}
	path := "/search?type=track&limit=" + strconv.Itoa(limit) + "&q=" + url.QueryEscape(query)
	var payload struct {
		Tracks struct {
			Items []Track `json:"items"`
		} `json:"tracks"`
	}
	if err := c.do(ctx, http.MethodGet, path, nil, &payload); err != nil {
		return nil, err
	}
	return payload.Tracks.Items, nil
}

func (c *Client) Track(ctx context.Context, uriOrURL string) (Track, error) {
	id, err := trackID(uriOrURL)
	if err != nil {
		return Track{}, err
	}
	var result Track
	if err := c.do(ctx, http.MethodGet, "/tracks/"+url.PathEscape(id), nil, &result); err != nil {
		return Track{}, err
	}
	if result.URI == "" || result.IsPlayable != nil && !*result.IsPlayable {
		return Track{}, fmt.Errorf("Spotify response did not contain a playable track")
	}
	return result, nil
}

func (c *Client) do(ctx context.Context, method, path string, body any, output any) error {
	access, err := c.Auth.AccessToken(ctx, false)
	if err != nil {
		return err
	}
	refreshed := false
	for {
		var encoded []byte
		if body != nil {
			encoded, err = json.Marshal(body)
			if err != nil {
				return err
			}
		}
		request, err := http.NewRequestWithContext(ctx, method, c.apiBase()+path, bytes.NewReader(encoded))
		if err != nil {
			return err
		}
		request.Header.Set("Authorization", "Bearer "+access)
		if body != nil {
			request.Header.Set("Content-Type", "application/json")
		}
		response, err := c.client().Do(request)
		if err != nil {
			return fmt.Errorf("Spotify API request failed: %w", err)
		}
		contents, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if readErr != nil {
			return readErr
		}
		if response.StatusCode == http.StatusUnauthorized && !refreshed {
			access, err = c.Auth.Refresh(ctx)
			if err != nil {
				return err
			}
			refreshed = true
			continue
		}
		if response.StatusCode == http.StatusTooManyRequests {
			value := response.Header.Get("Retry-After")
			delay, valid := retryAfter(value, time.Now())
			return &RateLimitError{RetryAfter: delay, Valid: valid, Value: value}
		}
		if response.StatusCode/100 != 2 {
			return spotifyResponseError(method, path, response, contents, access)
		}
		if output != nil && len(contents) > 0 {
			if err := json.Unmarshal(contents, output); err != nil {
				return err
			}
		}
		return nil
	}
}

func spotifyResponseError(method, path string, response *http.Response, contents []byte, access string) error {
	operation := spotifyOperation(method, path)
	detail := response.Status
	var payload struct {
		Error struct {
			Status  int    `json:"status"`
			Message string `json:"message"`
			Reason  string `json:"reason"`
		} `json:"error"`
	}
	if json.Unmarshal(contents, &payload) == nil {
		if payload.Error.Message != "" {
			detail = response.Status + ": " + payload.Error.Message
		}
		if payload.Error.Reason != "" {
			detail += " (reason: " + payload.Error.Reason + ")"
		}
	}
	if parsed, err := url.Parse(path); err == nil {
		if deviceID := parsed.Query().Get("device_id"); deviceID != "" {
			detail = strings.ReplaceAll(detail, deviceID, "[device]")
		}
	}
	if access != "" {
		detail = strings.ReplaceAll(detail, access, "[token]")
	}
	return &ResponseError{StatusCode: response.StatusCode, Message: fmt.Sprintf("Spotify %s failed: %s", operation, detail)}
}

func spotifyOperation(method, path string) string {
	switch {
	case method == http.MethodPut && strings.HasPrefix(path, "/me/player/play"):
		return "play"
	case method == http.MethodPut && strings.HasPrefix(path, "/me/player/pause"):
		return "pause"
	case method == http.MethodGet && strings.HasPrefix(path, "/me/player/devices"):
		return "list devices"
	case method == http.MethodGet && strings.HasPrefix(path, "/me/player"):
		return "get playback state"
	case method == http.MethodGet && strings.HasPrefix(path, "/tracks/"):
		return "track lookup"
	case method == http.MethodGet && strings.HasPrefix(path, "/search"):
		return "search"
	default:
		return "API request"
	}
}

func retryAfter(value string, now time.Time) (time.Duration, bool) {
	if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second, true
	}
	when, err := http.ParseTime(value)
	if err != nil {
		return 0, false
	}
	delay := when.Sub(now)
	if delay < 0 {
		delay = 0
	}
	return delay, true
}

func trackID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "spotify:track:") {
		value = strings.TrimPrefix(value, "spotify:track:")
	} else if parsed, err := url.Parse(value); err == nil && strings.EqualFold(parsed.Host, "open.spotify.com") {
		value = strings.TrimPrefix(parsed.Path, "/track/")
		value, _, _ = strings.Cut(value, "/")
	} else {
		return "", fmt.Errorf("value is not a Spotify track URL or URI")
	}
	value, _, _ = strings.Cut(value, "?")
	if value == "" || strings.ContainsAny(value, "/:") {
		return "", fmt.Errorf("value is not a Spotify track URL or URI")
	}
	return value, nil
}

func (c *Client) client() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return c.Auth.client()
}
func (c *Client) apiBase() string {
	if c.APIBase != "" {
		return strings.TrimRight(c.APIBase, "/")
	}
	return "https://api.spotify.com/v1"
}
