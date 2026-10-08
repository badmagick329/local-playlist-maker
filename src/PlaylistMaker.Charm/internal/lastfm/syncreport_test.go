package lastfm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"playlistmaker/charm/internal/library"
	"playlistmaker/charm/internal/spotify"
)

// recentTracksServer answers every page with the given scrobbles.
func recentTracksServer(t *testing.T, tracks ...any) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"recenttracks": map[string]any{"track": tracks, "@attr": map[string]string{"page": "1", "totalPages": "1"}}})
	}))
	t.Cleanup(server.Close)
	return server
}

func reportTracks() []library.Track {
	linked := testTrack("linked", "Linked", "Song")
	linked.SpotifyURI = "spotify:track:linked"
	ignored := testTrack("ignored", "Ignored", "Song")
	ignored.SpotifyIgnored = true
	return []library.Track{linked, ignored, testTrack("unlinked", "Unlinked", "Song")}
}

func TestSyncReportsOnlyAddedScrobblesAndSavesOneFilePerSync(t *testing.T) {
	dir := t.TempDir()
	if err := writeScrobbles(filepath.Join(dir, ScrobblesFile), []Scrobble{{Artist: "Lovelyz", Title: "WoW!", Album: "A", PlayedAtUTC: time.Unix(100, 0)}, {Artist: "Ignored", Title: "Song", Album: "A", PlayedAtUTC: time.Unix(50, 0)}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(dir, MatchesFile), MatchFile{SchemaVersion: SchemaVersion, Matches: []Match{{SourceKey: SourceKey("Absent", "Song"), Status: "no_match", Provenance: "agent"}}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(dir, SpotifyCacheFile), SpotifyCache{SchemaVersion: SchemaVersion, Tracks: []SpotifyMetadata{{URI: "spotify:track:linked", Name: "Song", Artists: []string{"Linked"}}}}); err != nil {
		t.Fatal(err)
	}
	server := recentTracksServer(t,
		apiTrack("Lovelyz", "WoW!", "A", "100"), // the boundary play Last.fm returns again
		apiTrack("Lovelyz", "WoW!", "A", "200"),
		apiTrack("Ignored", "Song", "A", "300"),
		apiTrack("Ignored", "Song", "A", "310"),
		apiTrack("Unlinked", "Song", "A", "320"),
		apiTrack("Linked", "Song", "A", "400"),
		apiTrack("Absent", "Song", "A", "500"))
	syncedAt := time.Date(2026, 10, 8, 15, 54, 47, 0, time.UTC)
	tracks := reportTracks()
	s := Service{DataDirectory: dir, Username: "u", APIKey: "k", Client: &Client{HTTP: server.Client(), APIBase: server.URL, Clock: func() time.Time { return syncedAt }}}
	if _, err := s.Load(tracks); err != nil {
		t.Fatal(err)
	}
	if s.Status().LastSyncUTC != nil {
		t.Fatal("a sync time was invented before any sync was recorded")
	}
	result, err := s.Sync(context.Background(), tracks, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := result.Report
	if r.AfterUTC == nil || !r.AfterUTC.Equal(time.Unix(100, 0)) || r.Added != 6 || r.Linked != 1 {
		t.Fatalf("report=%#v", r)
	}
	// Lovelyz has two cached scrobbles but only one added; Ignored has an older one.
	if len(r.Unresolved) != 1 || r.Unresolved[0].Title != "WoW!" || r.Unresolved[0].Scrobbles != 1 {
		t.Fatalf("unresolved=%#v", r.Unresolved)
	}
	if len(r.NoMatch) != 1 || r.NoMatch[0].Artist != "Absent" {
		t.Fatalf("noMatch=%#v", r.NoMatch)
	}
	if len(r.NoSpotifyLink) != 2 || r.NoSpotifyLink[0].TrackID != "ignored" || r.NoSpotifyLink[0].Scrobbles != 2 || !r.NoSpotifyLink[0].SpotifyIgnored || r.NoSpotifyLink[1].TrackID != "unlinked" || r.NoSpotifyLink[1].SpotifyIgnored {
		t.Fatalf("noSpotifyLink=%#v", r.NoSpotifyLink)
	}
	if _, err := os.Stat(filepath.Join(dir, SyncLogDirectory, "20261008T155447Z.json")); err != nil {
		t.Fatal(err)
	}

	restarted := Service{DataDirectory: dir}
	if _, err := restarted.Load(tracks); err != nil {
		t.Fatal(err)
	}
	if st := restarted.Status(); st.LastSyncUTC == nil || !st.LastSyncUTC.Equal(syncedAt) {
		t.Fatalf("last sync=%v", st.LastSyncUTC)
	}
	again, ok := restarted.LastSyncReport(tracks)
	if !ok || again.Added != 6 || len(again.Unresolved) != 1 {
		t.Fatalf("rebuilt report=%#v", again)
	}
	period, err := restarted.LastSyncPeriod()
	if err != nil || period.Contains(time.Unix(100, 0)) || !period.Contains(time.Unix(101, 0)) {
		t.Fatalf("period=%#v err=%v", period, err)
	}

	later := syncedAt.Add(time.Hour)
	second := recentTracksServer(t, apiTrack("Absent", "Song", "A", "500"), apiTrack("Lovelyz", "WoW!", "A", "600"))
	s.Client = &Client{HTTP: second.Client(), APIBase: second.URL, Clock: func() time.Time { return later }}
	result, err = s.Sync(context.Background(), tracks, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Report.Added != 1 || !result.Report.AfterUTC.Equal(time.Unix(500, 0)) {
		t.Fatalf("second report=%#v", result.Report)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, SyncLogDirectory))
	if len(entries) != 2 {
		t.Fatalf("sync report files=%d", len(entries))
	}
	restarted = Service{DataDirectory: dir}
	if _, err := restarted.Load(tracks); err != nil {
		t.Fatal(err)
	}
	if st := restarted.Status(); !st.LastSyncUTC.Equal(later) {
		t.Fatalf("newest report not loaded: %v", st.LastSyncUTC)
	}
}

// Rebuilding the whole history still reports from where the cache ended, so
// a rebuild does not list the entire backlog as new.
func TestFullRebuildReportsScrobblesAfterPreviousLatest(t *testing.T) {
	dir := t.TempDir()
	if err := writeScrobbles(filepath.Join(dir, ScrobblesFile), []Scrobble{{Artist: "Old", Title: "Song", Album: "A", PlayedAtUTC: time.Unix(100, 0)}}); err != nil {
		t.Fatal(err)
	}
	server := recentTracksServer(t, apiTrack("Old", "Song", "A", "100"), apiTrack("New", "Song", "A", "200"))
	s := Service{DataDirectory: dir, Username: "u", APIKey: "k", Client: &Client{HTTP: server.Client(), APIBase: server.URL, Clock: func() time.Time { return time.Unix(999, 0) }}}
	if _, err := s.Load(nil); err != nil {
		t.Fatal(err)
	}
	result, err := s.Sync(context.Background(), nil, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Report.Full || result.Report.Added != 1 || len(result.Report.Unresolved) != 1 || result.Report.Unresolved[0].Artist != "New" {
		t.Fatalf("report=%#v", result.Report)
	}
}

type cancellingLookup struct{ cancel context.CancelFunc }

func (c cancellingLookup) Track(ctx context.Context, _ string) (spotify.Track, error) {
	c.cancel()
	return spotify.Track{}, ctx.Err()
}

// Cancelling the Spotify step must still record the report: the scrobbles are
// already cached, so the next sync would start after them.
func TestSyncCancelledDuringSpotifyStepStillSavesReport(t *testing.T) {
	dir := t.TempDir()
	server := recentTracksServer(t, apiTrack("Lovelyz", "WoW!", "A", "200"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := Service{DataDirectory: dir, Username: "u", APIKey: "k", Spotify: cancellingLookup{cancel}, Client: &Client{HTTP: server.Client(), APIBase: server.URL, Clock: func() time.Time { return time.Unix(999, 0) }}}
	if _, err := s.Load(nil); err != nil {
		t.Fatal(err)
	}
	tracks := reportTracks()
	if _, err := s.Sync(ctx, tracks, false, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
	if _, ok := s.LastSyncReport(tracks); !ok {
		t.Fatal("cancelled sync left no report")
	}
	entries, _ := os.ReadDir(filepath.Join(dir, SyncLogDirectory))
	if len(entries) != 1 {
		t.Fatalf("sync report files=%d", len(entries))
	}
}

func TestReviewExportLimitedToPeriod(t *testing.T) {
	s := Service{DataDirectory: t.TempDir()}
	s.index = buildIndex([]Scrobble{
		{Artist: "Old", Title: "Only", PlayedAtUTC: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)},
		{Artist: "Both", Title: "Times", PlayedAtUTC: time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)},
		{Artist: "Both", Title: "Times", PlayedAtUTC: time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)},
		{Artist: "New", Title: "Only", PlayedAtUTC: time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)},
	}, nil, nil)
	period, err := library.ParseDateRange("2026-09-08..2026-10")
	if err != nil {
		t.Fatal(err)
	}
	dir, err := s.ExportReview(nil, time.Unix(1, 0), period)
	if err != nil {
		t.Fatal(err)
	}
	var review Review
	if err := readJSON(filepath.Join(dir, "review.json"), &review, "review"); err != nil {
		t.Fatal(err)
	}
	if review.Period != "2026-09-08..2026-10" || len(review.Cases) != 2 {
		t.Fatalf("period=%q cases=%#v", review.Period, review.Cases)
	}
	for _, c := range review.Cases {
		if c.Source.Artist == "Old" {
			t.Fatal("identity played only before the period was exported")
		}
		if c.Source.Artist == "Both" && c.Source.PlayCount != 2 {
			t.Fatalf("case lost its full history: %#v", c.Source)
		}
	}
}
