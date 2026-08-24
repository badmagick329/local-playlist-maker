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
)

type Client struct {
	Auth    *Auth
	HTTP    *http.Client
	APIBase string
}

type Device struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Restricted    bool   `json:"is_restricted"`
	VolumePercent *int   `json:"volume_percent"`
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

func (c *Client) SetVolume(ctx context.Context, deviceID string, percent int) error {
	path := "/me/player/volume?device_id=" + url.QueryEscape(deviceID) + "&volume_percent=" + strconv.Itoa(percent)
	return c.do(ctx, http.MethodPut, path, nil, nil)
}

func (c *Client) Play(ctx context.Context, deviceID, uri string) error {
	return c.do(ctx, http.MethodPut, "/me/player/play?device_id="+url.QueryEscape(deviceID), map[string]any{"uris": []string{uri}, "position_ms": 0}, nil)
}

func (c *Client) Pause(ctx context.Context, deviceID string) error {
	return c.do(ctx, http.MethodPut, "/me/player/pause?device_id="+url.QueryEscape(deviceID), nil, nil)
}

func (c *Client) Search(ctx context.Context, query string, limit int) ([]Track, error) {
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
	for attempt := 0; attempt < 2; attempt++ {
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
		if response.StatusCode == http.StatusUnauthorized && attempt == 0 {
			access, err = c.Auth.Refresh(ctx)
			if err != nil {
				return err
			}
			continue
		}
		if response.StatusCode/100 != 2 {
			return fmt.Errorf("Spotify API returned %s", response.Status)
		}
		if output != nil && len(contents) > 0 {
			if err := json.Unmarshal(contents, output); err != nil {
				return err
			}
		}
		return nil
	}
	return fmt.Errorf("Spotify authentication retry failed")
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
