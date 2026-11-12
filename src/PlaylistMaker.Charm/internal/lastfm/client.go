package lastfm

import (
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

const (
	requestTimeout = 30 * time.Second
	// A 200-scrobble page is a few hundred KB; anything near this cap is not a real page.
	maxResponseBytes = 8 << 20
)

type Client struct {
	HTTP    *http.Client
	APIBase string
	Clock   func() time.Time
}
type Page struct {
	Scrobbles        []Scrobble
	Page, TotalPages int
}

func (c *Client) RecentTracks(ctx context.Context, username, apiKey string, page int, from *int64, to int64) (Page, error) {
	q := url.Values{"user": {username}, "api_key": {apiKey}, "method": {"user.getrecenttracks"}, "format": {"json"}, "limit": {"200"}, "page": {strconv.Itoa(page)}, "to": {strconv.FormatInt(to, 10)}}
	if from != nil {
		q.Set("from", strconv.FormatInt(*from, 10))
	}
	// Bound each page on the request context rather than on the http.Client, so
	// injected clients get the same budget and a stalled upstream cannot hang a sync.
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base()+"?"+q.Encode(), nil)
	if err != nil {
		return Page{}, err
	}
	resp, err := c.client().Do(req)
	if err != nil {
		return Page{}, fmt.Errorf("Last.fm request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return Page{}, fmt.Errorf("Last.fm response read failed: %w", err)
	}
	if len(body) > maxResponseBytes {
		return Page{}, fmt.Errorf("Last.fm response exceeded %d bytes", maxResponseBytes)
	}
	var apiErr struct {
		Error   int    `json:"error"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &apiErr)
	if apiErr.Error != 0 {
		return Page{}, fmt.Errorf("Last.fm API error: %s", apiErr.Message)
	}
	if resp.StatusCode/100 != 2 {
		return Page{}, fmt.Errorf("Last.fm request failed: %s", resp.Status)
	}
	var payload struct {
		Recent struct {
			Track []struct {
				Artist struct {
					Text string `json:"#text"`
				} `json:"artist"`
				Name  string `json:"name"`
				Album struct {
					Text string `json:"#text"`
				} `json:"album"`
				MBID string `json:"mbid"`
				Date *struct {
					UTS string `json:"uts"`
				} `json:"date"`
				Attr struct {
					NowPlaying string `json:"nowplaying"`
				} `json:"@attr"`
			} `json:"track"`
			Attr struct {
				Page       string `json:"page"`
				TotalPages string `json:"totalPages"`
			} `json:"@attr"`
		} `json:"recenttracks"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return Page{}, fmt.Errorf("parse Last.fm response: %w", err)
	}
	result := Page{}
	result.Page, _ = strconv.Atoi(payload.Recent.Attr.Page)
	result.TotalPages, _ = strconv.Atoi(payload.Recent.Attr.TotalPages)
	for _, item := range payload.Recent.Track {
		if item.Date == nil || item.Attr.NowPlaying == "true" {
			continue
		}
		seconds, err := strconv.ParseInt(item.Date.UTS, 10, 64)
		if err != nil {
			return Page{}, fmt.Errorf("parse Last.fm scrobble timestamp: %w", err)
		}
		result.Scrobbles = append(result.Scrobbles, Scrobble{SchemaVersion: SchemaVersion, Artist: item.Artist.Text, Title: item.Name, Album: item.Album.Text, MBID: item.MBID, PlayedAtUTC: time.Unix(seconds, 0).UTC()})
	}
	return result, nil
}
func (c *Client) client() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}
func (c *Client) base() string {
	if c.APIBase != "" {
		return strings.TrimRight(c.APIBase, "/")
	}
	return "https://ws.audioscrobbler.com/2.0/"
}
func (c *Client) now() time.Time {
	if c.Clock != nil {
		return c.Clock()
	}
	return time.Now()
}
