package spotifylink

import (
	"context"
	"encoding/json"
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
	result, err := service.Scan(context.Background())
	if err != nil || result.AutoLinked != 1 || len(result.Items) != 1 || len(result.Items[0].Candidates) != 2 {
		t.Fatalf("scan = %#v, %v", result, err)
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

func TestScanCheckpointsBeforeAPIErrorAndRescanSkipsSavedTracks(t *testing.T) {
	requests := 0
	fail := true
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		if fail && requests == 26 {
			writer.WriteHeader(http.StatusInternalServerError)
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
	service := Service{CatalogPath: catalogPath, CachePath: filepath.Join(root, "cache.json"), Auth: auth, Client: &spotify.Client{Auth: auth, HTTP: server.Client(), APIBase: server.URL}}
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
	if requests != 27 {
		t.Fatalf("rescan requests = %d, want 27", requests)
	}
}
