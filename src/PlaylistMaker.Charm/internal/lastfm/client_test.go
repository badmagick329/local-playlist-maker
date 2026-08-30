package lastfm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"playlistmaker/charm/internal/library"
)

func TestSyncPaginatesWithFixedWindowDeduplicatesAndPreservesCacheOnFailure(t *testing.T) {
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	var queries []map[string]string
	fail := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			http.Error(w, "bad", 500)
			return
		}
		q := r.URL.Query()
		values := map[string]string{}
		for _, k := range []string{"method", "limit", "page", "to", "from"} {
			values[k] = q.Get(k)
		}
		queries = append(queries, values)
		page, _ := strconv.Atoi(q.Get("page"))
		tracks := []any{}
		if page == 1 {
			tracks = []any{apiTrack("Artist", "Song", "Album", "100"), map[string]any{"artist": map[string]string{"#text": "Artist"}, "name": "Now", "album": map[string]string{"#text": "Album"}, "@attr": map[string]string{"nowplaying": "true"}}}
		} else {
			tracks = []any{apiTrack("Artist", "Song", "Album", "100"), apiTrack("Other", "Later", "B", "200")}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"recenttracks": map[string]any{"track": tracks, "@attr": map[string]string{"page": strconv.Itoa(page), "totalPages": "2"}}})
	}))
	defer server.Close()
	s := Service{DataDirectory: t.TempDir(), Username: "u", APIKey: "k", Client: &Client{HTTP: server.Client(), APIBase: server.URL, Clock: func() time.Time { return now }}}
	tracks := []library.Track{testTrack("a", "Artist", "Song")}
	result, err := s.Sync(context.Background(), tracks, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Scrobbles != 2 || len(queries) != 2 {
		t.Fatalf("result=%#v queries=%#v", result, queries)
	}
	for _, q := range queries {
		if q["method"] != "user.getrecenttracks" || q["limit"] != "200" || q["to"] != strconv.FormatInt(now.Unix(), 10) || q["from"] != "" {
			t.Fatalf("query=%#v", q)
		}
	}
	stored, err := readScrobbles(filepath.Join(s.DataDirectory, ScrobblesFile))
	if err != nil || len(stored) != 2 || stored[0].PlayedAtUTC.Unix() != 100 || stored[1].PlayedAtUTC.Unix() != 200 {
		t.Fatalf("stored=%#v err=%v", stored, err)
	}
	before, _ := os.ReadFile(filepath.Join(s.DataDirectory, ScrobblesFile))
	fail = true
	if _, err = s.Sync(context.Background(), tracks, false, nil); err == nil {
		t.Fatal("expected failure")
	}
	after, _ := os.ReadFile(filepath.Join(s.DataDirectory, ScrobblesFile))
	if string(before) != string(after) {
		t.Fatal("failed sync changed cache")
	}
}

func TestIncrementalSyncUsesLatestInclusiveTimestamp(t *testing.T) {
	now := time.Unix(999, 0)
	var from string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		from = r.URL.Query().Get("from")
		_ = json.NewEncoder(w).Encode(map[string]any{"recenttracks": map[string]any{"track": []any{apiTrack("A", "B", "C", "200")}, "@attr": map[string]string{"page": "1", "totalPages": "1"}}})
	}))
	defer server.Close()
	dir := t.TempDir()
	_ = writeScrobbles(filepath.Join(dir, ScrobblesFile), []Scrobble{{Artist: "A", Title: "B", Album: "C", PlayedAtUTC: time.Unix(200, 0)}})
	s := Service{DataDirectory: dir, Username: "u", APIKey: "k", Client: &Client{HTTP: server.Client(), APIBase: server.URL, Clock: func() time.Time { return now }}}
	_, _ = s.Load(nil)
	result, err := s.Sync(context.Background(), nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if from != "200" || result.Scrobbles != 1 {
		t.Fatalf("from=%q result=%#v", from, result)
	}
}

func TestClientReportsAPIErrorAndHonoursCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"error": 6, "message": "User not found"})
	}))
	defer server.Close()
	c := Client{HTTP: server.Client(), APIBase: server.URL}
	if _, err := c.RecentTracks(context.Background(), "u", "k", 1, nil, 1); err == nil {
		t.Fatal("expected API error")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.RecentTracks(ctx, "u", "k", 1, nil, 1); err == nil {
		t.Fatal("expected cancellation")
	}
}

func TestCancelledFetchPreservesPreviousCache(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ScrobblesFile)
	if err := writeScrobbles(path, []Scrobble{{Artist: "Old", Title: "Play", PlayedAtUTC: time.Unix(10, 0)}}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := Service{DataDirectory: dir, Username: "u", APIKey: "k", Client: &Client{APIBase: "http://127.0.0.1:1"}}
	_, _ = s.Load(nil)
	if _, err := s.Sync(ctx, nil, false, nil); err == nil {
		t.Fatal("expected cancellation")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("cancelled fetch changed cache")
	}
}

func TestFailedFullSyncResumesRemainingPagesAfterRestart(t *testing.T) {
	firstWindow := time.Unix(1000, 0)
	secondWindow := time.Unix(2000, 0)
	failLast := true
	var requestedPages []string
	var requestedTo []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		requestedPages = append(requestedPages, page)
		requestedTo = append(requestedTo, r.URL.Query().Get("to"))
		if page == "3" && failLast {
			_ = json.NewEncoder(w).Encode(map[string]any{"error": 8, "message": "Operation failed"})
			return
		}
		seconds := map[string]string{"1": "300", "2": "200", "3": "100"}[page]
		_ = json.NewEncoder(w).Encode(map[string]any{"recenttracks": map[string]any{"track": []any{apiTrack("Artist", "Song "+page, "Album", seconds)}, "@attr": map[string]string{"page": page, "totalPages": "3"}}})
	}))
	defer server.Close()
	dir := t.TempDir()
	first := Service{DataDirectory: dir, Username: "u", APIKey: "k", Client: &Client{HTTP: server.Client(), APIBase: server.URL, Clock: func() time.Time { return firstWindow }}}
	if _, err := first.Sync(context.Background(), nil, true, nil); err == nil {
		t.Fatal("expected late API failure")
	}
	if got := requestedPages; len(got) != 3 || got[0] != "1" || got[2] != "3" {
		t.Fatalf("first pages=%v", got)
	}
	var checkpoint SyncCheckpoint
	if err := readJSON(filepath.Join(dir, SyncCheckpointFile), &checkpoint, "checkpoint"); err != nil {
		t.Fatal(err)
	}
	if checkpoint.NextPage != 3 {
		t.Fatalf("checkpoint=%#v", checkpoint)
	}
	status := first.Status()
	if status.CheckpointPages != 2 || status.CheckpointTotal != 3 {
		t.Fatalf("checkpoint status=%#v", status)
	}
	failLast = false
	requestedPages = nil
	requestedTo = nil
	second := Service{DataDirectory: dir, Username: "u", APIKey: "k", Client: &Client{HTTP: server.Client(), APIBase: server.URL, Clock: func() time.Time { return secondWindow }}}
	if _, err := second.Load(nil); err != nil {
		t.Fatal(err)
	}
	result, err := second.Sync(context.Background(), nil, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(requestedPages) != 1 || requestedPages[0] != "3" {
		t.Fatalf("resumed pages=%v", requestedPages)
	}
	if requestedTo[0] != strconv.FormatInt(firstWindow.Unix(), 10) {
		t.Fatalf("resumed to=%q", requestedTo[0])
	}
	if result.Scrobbles != 3 {
		t.Fatalf("result=%#v", result)
	}
	if _, err := os.Stat(filepath.Join(dir, SyncCheckpointFile)); !os.IsNotExist(err) {
		t.Fatalf("checkpoint metadata remains: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, SyncCheckpointScrobblesFile)); !os.IsNotExist(err) {
		t.Fatalf("checkpoint scrobbles remain: %v", err)
	}
}

func TestCancellationKeepsCompletedPageForResume(t *testing.T) {
	var pages []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		pages = append(pages, page)
		_ = json.NewEncoder(w).Encode(map[string]any{"recenttracks": map[string]any{"track": []any{apiTrack("A", "B", "C", "1")}, "@attr": map[string]string{"page": page, "totalPages": "2"}}})
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	s := Service{DataDirectory: t.TempDir(), Username: "u", APIKey: "k", Client: &Client{HTTP: server.Client(), APIBase: server.URL}}
	_, err := s.Sync(ctx, nil, true, func(progress SyncProgress) {
		if progress.PagesFetched == 1 {
			cancel()
		}
	})
	if err == nil {
		t.Fatal("expected cancellation")
	}
	var checkpoint SyncCheckpoint
	if err := readJSON(filepath.Join(s.DataDirectory, SyncCheckpointFile), &checkpoint, "checkpoint"); err != nil {
		t.Fatal(err)
	}
	if checkpoint.NextPage != 2 || len(pages) != 1 {
		t.Fatalf("checkpoint=%#v pages=%v", checkpoint, pages)
	}
}

func apiTrack(artist, title, album, uts string) map[string]any {
	return map[string]any{"artist": map[string]string{"#text": artist}, "name": title, "album": map[string]string{"#text": album}, "mbid": "mbid", "date": map[string]string{"uts": uts}}
}
