package spotifylink

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"playlistmaker/charm/internal/catalog"
	"playlistmaker/charm/internal/spotify"
)

func TestScanAutoSavesOnlyUniqueExactMatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		query, _ := url.QueryUnescape(request.URL.Query().Get("q"))
		count := 1
		name := "Unique"
		if strings.Contains(query, "Ambiguous") {
			count, name = 2, "Ambiguous"
		}
		items := []map[string]any{}
		for index := range count {
			items = append(items, map[string]any{"uri": "spotify:track:" + name + string(rune('a'+index)), "name": name, "duration_ms": 180000, "artists": []map[string]any{{"name": "Artist"}}, "album": map[string]any{"name": "Album", "release_date": "2024"}})
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"tracks": map[string]any{"items": items}})
	}))
	defer server.Close()
	root := t.TempDir()
	catalogPath := filepath.Join(root, "catalog.json")
	media := catalog.New()
	media.Tracks = []catalog.Track{{ID: "trk_unique", Artist: "Artist", Title: "Unique"}, {ID: "trk_ambiguous", Artist: "Artist", Title: "Ambiguous"}}
	if err := catalog.Write(catalogPath, media); err != nil {
		t.Fatal(err)
	}
	tokenPath := filepath.Join(root, "spotify-auth.json")
	token, _ := json.Marshal(spotify.Token{AccessToken: "token", RefreshToken: "refresh", ExpiresAtUTC: time.Now().Add(time.Hour)})
	if err := os.WriteFile(tokenPath, token, 0o600); err != nil {
		t.Fatal(err)
	}
	auth := &spotify.Auth{ClientID: "client", TokenPath: tokenPath, HTTP: server.Client()}
	service := Service{CatalogPath: catalogPath, CachePath: filepath.Join(root, "missing-cache.json"), Auth: auth, Client: &spotify.Client{Auth: auth, HTTP: server.Client(), APIBase: server.URL}}
	progress := []ScanProgress{}
	result, err := service.ScanWithProgress(context.Background(), func(value ScanProgress) { progress = append(progress, value) })
	if err != nil || result.AutoLinked != 1 || len(result.Items) != 1 || len(result.Items[0].Candidates) != 2 {
		t.Fatalf("scan = %#v, %v", result, err)
	}
	if len(progress) < 4 || progress[0].Phase != "authenticating" || progress[1].Phase != "scanning" || progress[1].Total != 2 || progress[2].Current != 1 || progress[3].Current != 2 {
		t.Fatalf("progress = %#v", progress)
	}
	loaded, _ := catalog.Read(catalogPath)
	if loaded.Tracks[0].SpotifyURI == "" && loaded.Tracks[1].SpotifyURI == "" {
		t.Fatal("unique exact match was not saved")
	}
	for _, track := range loaded.Tracks {
		if track.Title == "Ambiguous" && track.SpotifyURI != "" {
			t.Fatal("ambiguous match was saved automatically")
		}
	}
}

func TestReleaseDateMatchesDifferentPrecision(t *testing.T) {
	for _, values := range [][2]string{{"2024-01-01", "2024"}, {"2024", "2024-01"}} {
		if !releaseDateMatches(values[0], values[1]) {
			t.Fatalf("releaseDateMatches(%q, %q) = false", values[0], values[1])
		}
	}
	if releaseDateMatches("2024-01-01", "2024-01-15") || releaseDateMatches("2024-01-01", "2023-01-01") || releaseDateMatches("", "2024") {
		t.Fatal("different or missing release dates matched")
	}
}

func TestConfirmAndIgnorePersistLinkDecisions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog.json")
	media := catalog.New()
	media.Tracks = []catalog.Track{{ID: "trk_one", Artist: "Artist", Title: "Title"}}
	if err := catalog.Write(path, media); err != nil {
		t.Fatal(err)
	}
	service := Service{CatalogPath: path}
	if err := service.Confirm("trk_one", "spotify:track:one"); err != nil {
		t.Fatal(err)
	}
	if err := service.Ignore("trk_one"); err != nil {
		t.Fatal(err)
	}
	loaded, _ := catalog.Read(path)
	if loaded.Tracks[0].SpotifyURI != "" || !loaded.Tracks[0].SpotifyIgnored {
		t.Fatalf("persisted decision = %#v", loaded.Tracks[0])
	}
}

func TestSearchRetriesOneShortRateLimit(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests++
		if requests == 1 {
			writer.Header().Set("Retry-After", "1")
			writer.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"tracks": map[string]any{"items": []any{}}})
	}))
	defer server.Close()
	auth := linkingAuth(t, server)
	waits := 0
	service := Service{Auth: auth, Client: &spotify.Client{Auth: auth, HTTP: server.Client(), APIBase: server.URL}, Wait: func(_ context.Context, delay time.Duration) error {
		waits++
		if delay != time.Second {
			t.Fatalf("wait delay = %s", delay)
		}
		return nil
	}}
	if _, err := service.Search(context.Background(), "query"); err != nil {
		t.Fatal(err)
	}
	if requests != 2 || waits != 1 {
		t.Fatalf("requests = %d, waits = %d", requests, waits)
	}
}

func TestSearchDoesNotRetryInvalidOrExcessiveRateLimit(t *testing.T) {
	for _, retryAfter := range []string{"invalid", "31"} {
		t.Run(retryAfter, func(t *testing.T) {
			requests, waits := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				requests++
				writer.Header().Set("Retry-After", retryAfter)
				writer.WriteHeader(http.StatusTooManyRequests)
			}))
			defer server.Close()
			auth := linkingAuth(t, server)
			service := Service{Auth: auth, Client: &spotify.Client{Auth: auth, HTTP: server.Client(), APIBase: server.URL}, Wait: func(context.Context, time.Duration) error { waits++; return nil }}
			if _, err := service.Search(context.Background(), "query"); err == nil {
				t.Fatal("rate limit was accepted")
			}
			if requests != 1 || waits != 0 {
				t.Fatalf("requests = %d, waits = %d", requests, waits)
			}
		})
	}
}

func TestRateLimitWaitRespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	requests := 0
	_, err := retryRateLimit(ctx, nil, func() (int, error) {
		requests++
		return 0, &spotify.RateLimitError{RetryAfter: time.Second, Valid: true, Value: "1"}
	})
	if !errors.Is(err, context.Canceled) || requests != 1 {
		t.Fatalf("retry error = %v, requests = %d", err, requests)
	}
}

func TestScanCheckpointsBeforeAPIErrorAndRescanSkipsSavedTracks(t *testing.T) {
	requests := 0
	fail := true
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		if fail && requests >= 26 && requests <= 27 {
			writer.Header().Set("Retry-After", "1")
			writer.WriteHeader(http.StatusTooManyRequests)
			return
		}
		query, _ := url.QueryUnescape(request.URL.Query().Get("q"))
		title := strings.TrimSuffix(strings.TrimPrefix(query[strings.Index(query, "track:")+6:], `"`), `"`)
		_ = json.NewEncoder(writer).Encode(map[string]any{"tracks": map[string]any{"items": []map[string]any{{"uri": "spotify:track:" + title, "name": title, "artists": []map[string]any{{"name": "Artist"}}, "album": map[string]any{"name": "Album"}}}}})
	}))
	defer server.Close()
	root := t.TempDir()
	media := catalog.New()
	for index := range 26 {
		media.Tracks = append(media.Tracks, catalog.Track{ID: fmt.Sprintf("trk_%02d", index), Artist: "Artist", Title: fmt.Sprintf("Title %02d", index)})
	}
	catalogPath := filepath.Join(root, "catalog.json")
	if err := catalog.Write(catalogPath, media); err != nil {
		t.Fatal(err)
	}
	tokenPath := filepath.Join(root, "spotify-auth.json")
	token, _ := json.Marshal(spotify.Token{AccessToken: "token", RefreshToken: "refresh", ExpiresAtUTC: time.Now().Add(time.Hour)})
	if err := os.WriteFile(tokenPath, token, 0o600); err != nil {
		t.Fatal(err)
	}
	auth := &spotify.Auth{ClientID: "client", TokenPath: tokenPath, HTTP: server.Client()}
	service := Service{CatalogPath: catalogPath, CachePath: filepath.Join(root, "cache.json"), Auth: auth, Client: &spotify.Client{Auth: auth, HTTP: server.Client(), APIBase: server.URL}, Wait: func(context.Context, time.Duration) error { return nil }}
	if _, err := service.Scan(context.Background()); err == nil {
		t.Fatal("scan API failure was ignored")
	}
	loaded, err := catalog.Read(catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	linked := 0
	for _, track := range loaded.Tracks {
		if track.SpotifyURI != "" {
			linked++
		}
	}
	if linked != 25 {
		t.Fatalf("checkpointed links = %d", linked)
	}
	fail = false
	if _, err := service.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if requests != 28 {
		t.Fatalf("rescan requests = %d, want 28", requests)
	}
}

func linkingAuth(t *testing.T, server *httptest.Server) *spotify.Auth {
	t.Helper()
	tokenPath := filepath.Join(t.TempDir(), "spotify-auth.json")
	token, _ := json.Marshal(spotify.Token{AccessToken: "token", RefreshToken: "refresh", ExpiresAtUTC: time.Now().Add(time.Hour)})
	if err := os.WriteFile(tokenPath, token, 0o600); err != nil {
		t.Fatal(err)
	}
	return &spotify.Auth{ClientID: "client", TokenPath: tokenPath, HTTP: server.Client()}
}
