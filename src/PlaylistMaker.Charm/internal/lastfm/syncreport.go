package lastfm

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"playlistmaker/charm/internal/library"
)

// SyncLogDirectory holds one report per sync. The newest file is the sync
// state: when the last sync ran and where the cache ended before it, which the
// scrobbles file alone cannot say once later syncs have merged into it.
const SyncLogDirectory = "lastfm-syncs"

const syncLogLayout = "20060102T150405Z"

// ErrSyncLogNotSaved marks a sync whose scrobbles and matches were saved but
// whose report file was not, so the next sync cannot tell which scrobbles this
// one added.
var ErrSyncLogNotSaved = errors.New("Last.fm sync report was not saved")

// SyncReport describes the scrobbles a sync added: every cached scrobble
// played after AfterUTC, the latest one cached before the sync. AfterUTC is nil
// when the cache was empty, so the first sync reports the whole history.
type SyncReport struct {
	SchemaVersion int        `json:"schemaVersion"`
	SyncedAtUTC   time.Time  `json:"syncedAtUtc"`
	Full          bool       `json:"full"`
	AfterUTC      *time.Time `json:"afterUtc,omitempty"`
	Added         int        `json:"added"`
	// Linked counts added scrobbles whose song has a Spotify link; the lists
	// hold the songs whose added scrobbles have none.
	Linked        int          `json:"linked"`
	Unresolved    []ReportSong `json:"unresolved"`
	NoMatch       []ReportSong `json:"noMatch"`
	NoSpotifyLink []ReportSong `json:"noSpotifyLink"`
}

// ReportSong is one identity in a sync report. Scrobbles counts only the
// scrobbles the sync added.
type ReportSong struct {
	Artist         string `json:"artist"`
	Title          string `json:"title"`
	Scrobbles      int    `json:"scrobbles"`
	TrackID        string `json:"trackId,omitempty"`
	SpotifyIgnored bool   `json:"spotifyIgnored,omitempty"`
}

func ScrobbleTotal(songs []ReportSong) int {
	total := 0
	for _, v := range songs {
		total += v.Scrobbles
	}
	return total
}

// report classifies the identities played after the boundary under the current
// matches, so a report rebuilt later reflects decisions imported since.
func (s *Service) report(tracks []library.Track, syncedAt time.Time, full bool, after *time.Time) SyncReport {
	r := SyncReport{SchemaVersion: SchemaVersion, SyncedAtUTC: syncedAt.UTC(), Full: full, AfterUTC: after, Unresolved: []ReportSong{}, NoMatch: []ReportSong{}, NoSpotifyLink: []ReportSong{}}
	byID := map[string]library.Track{}
	for _, t := range tracks {
		byID[t.ID] = t
	}
	for key, id := range s.index.Identities {
		added := len(id.PlayedAtUTC)
		if after != nil {
			added -= sort.Search(len(id.PlayedAtUTC), func(i int) bool { return id.PlayedAtUTC[i].After(*after) })
		}
		if added == 0 {
			continue
		}
		r.Added += added
		song := ReportSong{Artist: id.Artist, Title: id.Title, Scrobbles: added}
		m, ok := s.index.Matches[key]
		switch {
		case !ok:
			r.Unresolved = append(r.Unresolved, song)
		case m.Status == "no_match":
			r.NoMatch = append(r.NoMatch, song)
		default:
			track := byID[m.TrackID]
			if track.SpotifyURI != "" && !track.SpotifyIgnored {
				r.Linked += added
				continue
			}
			song.TrackID, song.SpotifyIgnored = m.TrackID, track.SpotifyIgnored
			r.NoSpotifyLink = append(r.NoSpotifyLink, song)
		}
	}
	for _, songs := range [][]ReportSong{r.Unresolved, r.NoMatch, r.NoSpotifyLink} {
		sort.Slice(songs, func(i, j int) bool {
			if songs[i].Scrobbles != songs[j].Scrobbles {
				return songs[i].Scrobbles > songs[j].Scrobbles
			}
			return songs[i].Artist+"\x00"+songs[i].Title < songs[j].Artist+"\x00"+songs[j].Title
		})
	}
	return r
}

func (s *Service) saveSyncReport(r SyncReport) error {
	return writeJSON(filepath.Join(s.path(SyncLogDirectory), r.SyncedAtUTC.Format(syncLogLayout)+".json"), r)
}

// readLatestSyncReport returns nil when no sync has been recorded. Files are
// named by sync time, so the greatest name is the newest; names in any other
// form are not reports and are left alone.
func readLatestSyncReport(dir string) (*SyncReport, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read Last.fm sync reports: %w", err)
	}
	latest := ""
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		if _, err := time.Parse(syncLogLayout, strings.TrimSuffix(name, ".json")); err == nil && name > latest {
			latest = name
		}
	}
	if latest == "" {
		return nil, nil
	}
	var r SyncReport
	if err := readJSON(filepath.Join(dir, latest), &r, "Last.fm sync report "+latest); err != nil {
		return nil, err
	}
	if r.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("Last.fm sync report %s schemaVersion is %d, want %d", latest, r.SchemaVersion, SchemaVersion)
	}
	return &r, nil
}

// LastSyncReport rebuilds the newest sync's report under the current matches.
func (s *Service) LastSyncReport(tracks []library.Track) (SyncReport, bool) {
	last := s.index.LastSync
	if last == nil {
		return SyncReport{}, false
	}
	return s.report(tracks, last.SyncedAtUTC, last.Full, last.AfterUTC), true
}

// LastSyncPeriod is the review-export period holding the scrobbles the newest
// sync added.
func (s *Service) LastSyncPeriod() (*library.DateRange, error) {
	last := s.index.LastSync
	if last == nil {
		return nil, fmt.Errorf("no Last.fm sync is recorded yet")
	}
	period := &library.DateRange{Label: "since last sync", End: time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)}
	if last.AfterUTC != nil {
		period.Start = last.AfterUTC.Add(time.Nanosecond)
	}
	return period, nil
}
