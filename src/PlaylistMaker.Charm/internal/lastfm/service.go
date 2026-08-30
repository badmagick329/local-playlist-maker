package lastfm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"playlistmaker/charm/internal/library"
	"playlistmaker/charm/internal/spotify"
	"playlistmaker/charm/internal/videoname"
)

type SpotifyLookup interface {
	Track(context.Context, string) (spotify.Track, error)
}
type Service struct {
	DataDirectory, Username, APIKey string
	Client                          *Client
	Spotify                         SpotifyLookup
	Random                          RandomSource
	index                           Index
}
type RandomSource interface{ Intn(int) int }

func (s *Service) Configured() bool        { return s.Username != "" && s.APIKey != "" }
func (s *Service) path(name string) string { return filepath.Join(s.DataDirectory, name) }
func SourceKey(artist, title string) string {
	return videoname.Normalize(artist) + "\x00" + videoname.Normalize(title)
}
func dedupeKey(v Scrobble) string {
	return fmt.Sprintf("%d\x00%s\x00%s\x00%s", v.PlayedAtUTC.Unix(), videoname.Normalize(v.Artist), videoname.Normalize(v.Title), videoname.Normalize(v.Album))
}

func (s *Service) Load(tracks []library.Track) ([]library.Track, error) {
	scrobbles, err := readScrobbles(s.path(ScrobblesFile))
	if err != nil {
		s.index = Index{Error: err.Error()}
		return tracks, err
	}
	var mf MatchFile
	if err = readJSON(s.path(MatchesFile), &mf, "Last.fm matches"); err != nil {
		s.index = Index{Scrobbles: scrobbles, Error: err.Error()}
		return tracks, err
	}
	if mf.SchemaVersion != 0 && mf.SchemaVersion != SchemaVersion {
		return tracks, fmt.Errorf("Last.fm matches schemaVersion is %d, want %d", mf.SchemaVersion, SchemaVersion)
	}
	var cache SpotifyCache
	if err = readJSON(s.path(SpotifyCacheFile), &cache, "Spotify track cache"); err != nil {
		return tracks, err
	}
	if cache.SchemaVersion != 0 && cache.SchemaVersion != SchemaVersion {
		return tracks, fmt.Errorf("Spotify track cache schemaVersion is %d, want %d", cache.SchemaVersion, SchemaVersion)
	}
	s.index = buildIndex(scrobbles, mf.Matches, cache.Tracks)
	if info, statErr := os.Stat(s.path(ScrobblesFile)); statErr == nil {
		value := info.ModTime().UTC()
		s.index.LastSyncUTC = &value
	}
	s.index.SpotifyComplete = true
	for _, track := range tracks {
		if track.SpotifyURI != "" {
			if _, ok := s.index.Spotify[track.SpotifyURI]; !ok {
				s.index.SpotifyComplete = false
				break
			}
		}
	}
	s.resolve(tracks)
	return s.Attach(tracks), nil
}

func buildIndex(scrobbles []Scrobble, matches []Match, spotifyValues []SpotifyMetadata) Index {
	idx := Index{Scrobbles: scrobbles, Identities: map[string]*Identity{}, Matches: map[string]Match{}, TrackPlays: map[string][]time.Time{}, Spotify: map[string]SpotifyMetadata{}}
	type spelling struct {
		artist, title string
		count         int
	}
	spellings := map[string]map[string]*spelling{}
	albums := map[string]map[string]int{}
	mbids := map[string]map[string]bool{}
	for _, v := range scrobbles {
		k := SourceKey(v.Artist, v.Title)
		id := idx.Identities[k]
		if id == nil {
			id = &Identity{Key: k}
			idx.Identities[k] = id
		}
		id.PlayedAtUTC = append(id.PlayedAtUTC, v.PlayedAtUTC)
		sk := v.Artist + "\x00" + v.Title
		if spellings[k] == nil {
			spellings[k] = map[string]*spelling{}
		}
		if spellings[k][sk] == nil {
			spellings[k][sk] = &spelling{artist: v.Artist, title: v.Title}
		}
		spellings[k][sk].count++
		if albums[k] == nil {
			albums[k] = map[string]int{}
		}
		if v.Album != "" {
			albums[k][v.Album]++
		}
		if mbids[k] == nil {
			mbids[k] = map[string]bool{}
		}
		if v.MBID != "" {
			mbids[k][v.MBID] = true
		}
	}
	for k, id := range idx.Identities {
		sort.Slice(id.PlayedAtUTC, func(i, j int) bool { return id.PlayedAtUTC[i].Before(id.PlayedAtUTC[j]) })
		id.PlayCount = len(id.PlayedAtUTC)
		id.FirstPlayedAtUTC = id.PlayedAtUTC[0]
		id.LastPlayedAtUTC = id.PlayedAtUTC[len(id.PlayedAtUTC)-1]
		var best *spelling
		for _, v := range spellings[k] {
			if best == nil || v.count > best.count || v.count == best.count && (v.artist+"\x00"+v.title) < (best.artist+"\x00"+best.title) {
				best = v
			}
		}
		id.Artist, id.Title = best.artist, best.title
		for n, c := range albums[k] {
			id.Albums = append(id.Albums, AlbumCount{Name: n, PlayCount: c})
		}
		sort.Slice(id.Albums, func(i, j int) bool {
			if id.Albums[i].PlayCount != id.Albums[j].PlayCount {
				return id.Albums[i].PlayCount > id.Albums[j].PlayCount
			}
			return id.Albums[i].Name < id.Albums[j].Name
		})
		for v := range mbids[k] {
			id.MBIDs = append(id.MBIDs, v)
		}
		sort.Strings(id.MBIDs)
	}
	for _, v := range matches {
		idx.Matches[v.SourceKey] = v
	}
	for _, v := range spotifyValues {
		idx.Spotify[v.URI] = v
	}
	return idx
}

func CatalogueFingerprint(tracks []library.Track) string {
	copyTracks := append([]library.Track(nil), tracks...)
	sort.Slice(copyTracks, func(i, j int) bool { return copyTracks[i].ID < copyTracks[j].ID })
	h := sha256.New()
	for _, v := range copyTracks {
		fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s\x00%s\n", v.ID, v.Artist, v.Title, v.ReleaseDateLabel, v.SpotifyURI)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (s *Service) resolve(tracks []library.Track) {
	fingerprint := CatalogueFingerprint(tracks)
	s.index.Fingerprint = fingerprint
	existing := map[string]bool{}
	local := map[string]map[string]bool{}
	aliases := map[string]map[string]bool{}
	for _, t := range tracks {
		existing[t.ID] = true
		k := SourceKey(t.Artist, t.Title)
		if local[k] == nil {
			local[k] = map[string]bool{}
		}
		local[k][t.ID] = true
		if t.SpotifyURI != "" {
			if md, ok := s.index.Spotify[t.SpotifyURI]; ok {
				for _, artist := range md.Artists {
					k = SourceKey(artist, md.Name)
					if aliases[k] == nil {
						aliases[k] = map[string]bool{}
					}
					aliases[k][t.ID] = true
				}
			}
		}
	}
	for key, id := range s.index.Identities {
		if old, ok := s.index.Matches[key]; ok {
			if old.Status == "match" && existing[old.TrackID] {
				continue
			}
			if old.Status == "no_match" && old.CatalogueFingerprint == fingerprint {
				continue
			}
			delete(s.index.Matches, key)
		}
		ids := map[string]bool{}
		for trackID := range local[key] {
			ids[trackID] = true
		}
		for trackID := range aliases[key] {
			ids[trackID] = true
		}
		if len(ids) == 1 {
			var trackID string
			for trackID = range ids {
			}
			s.index.Matches[key] = Match{SourceKey: key, Artist: id.Artist, Title: id.Title, Status: "match", TrackID: trackID, Provenance: "auto", Reason: "unique exact normalized alias"}
		}
	}
	s.rebuildTrackPlays()
}
func (s *Service) rebuildTrackPlays() {
	s.index.TrackPlays = map[string][]time.Time{}
	for key, m := range s.index.Matches {
		if m.Status == "match" {
			if id := s.index.Identities[key]; id != nil {
				s.index.TrackPlays[m.TrackID] = append(s.index.TrackPlays[m.TrackID], id.PlayedAtUTC...)
			}
		}
	}
	for k := range s.index.TrackPlays {
		sort.Slice(s.index.TrackPlays[k], func(i, j int) bool { return s.index.TrackPlays[k][i].Before(s.index.TrackPlays[k][j]) })
	}
}

func (s *Service) saveMatches() error {
	values := make([]Match, 0, len(s.index.Matches))
	for _, v := range s.index.Matches {
		values = append(values, v)
	}
	sort.Slice(values, func(i, j int) bool { return values[i].SourceKey < values[j].SourceKey })
	return writeJSON(s.path(MatchesFile), MatchFile{SchemaVersion: SchemaVersion, Matches: values})
}
func (s *Service) Attach(tracks []library.Track) []library.Track {
	result := append([]library.Track(nil), tracks...)
	for i := range result {
		plays := s.index.TrackPlays[result[i].ID]
		if len(plays) > 0 {
			first, last := plays[0], plays[len(plays)-1]
			result[i].LastFM = library.LastFMSummary{PlayedCount: len(plays), FirstPlayedAtUTC: &first, LastPlayedAtUTC: &last}
		}
	}
	return result
}

func (s *Service) RefreshCatalogue(tracks []library.Track) ([]library.Track, error) {
	s.resolve(tracks)
	if len(s.index.Identities) == 0 {
		return s.Attach(tracks), nil
	}
	if err := s.saveMatches(); err != nil {
		return tracks, err
	}
	return s.Attach(tracks), nil
}
func (s *Service) Status() Status {
	v := Status{Configured: s.Configured(), Scrobbles: len(s.index.Scrobbles), LastSyncUTC: s.index.LastSyncUTC, SpotifyComplete: s.index.SpotifyComplete, Error: s.index.Error}
	var checkpoint SyncCheckpoint
	if readJSON(s.path(SyncCheckpointFile), &checkpoint, "Last.fm sync checkpoint") == nil && checkpoint.SchemaVersion == SchemaVersion && checkpoint.Username == s.Username && checkpoint.NextPage > 1 {
		v.CheckpointPages = checkpoint.NextPage - 1
		v.CheckpointTotal = checkpoint.TotalPages
	}
	if len(s.index.Scrobbles) > 0 {
		first, last := s.index.Scrobbles[0].PlayedAtUTC, s.index.Scrobbles[len(s.index.Scrobbles)-1].PlayedAtUTC
		v.FirstPlayedAtUTC = &first
		v.LastPlayedAtUTC = &last
	}
	for k := range s.index.Identities {
		if m, ok := s.index.Matches[k]; ok && (m.Status == "match" || m.Status == "no_match") {
			if m.Status == "match" {
				v.Matched++
			}
		} else {
			v.Unresolved++
		}
	}
	return v
}

func (s *Service) Sync(ctx context.Context, tracks []library.Track, full bool, report func(SyncProgress)) (SyncResult, error) {
	if !s.Configured() {
		return SyncResult{}, fmt.Errorf("Last.fm username and API key are not configured")
	}
	client := s.Client
	if client == nil {
		client = &Client{}
	}
	var from *int64
	old := s.index.Scrobbles
	if !full && len(old) > 0 {
		v := old[len(old)-1].PlayedAtUTC.Unix()
		from = &v
	}
	checkpoint, collected, resumed, err := s.prepareSyncCheckpoint(client.now().Unix(), from)
	if err != nil {
		return SyncResult{}, err
	}
	totalPages := checkpoint.TotalPages
	if totalPages < 1 {
		totalPages = 1
	}
	if report != nil && resumed {
		report(SyncProgress{Phase: "fetching", PagesFetched: checkpoint.NextPage - 1, TotalPages: totalPages, Scrobbles: len(collected), Resumed: true})
	}
	for page := checkpoint.NextPage; page <= totalPages; page++ {
		if err := ctx.Err(); err != nil {
			return SyncResult{}, err
		}
		value, err := client.RecentTracks(ctx, s.Username, s.APIKey, page, checkpoint.FromUnix, checkpoint.ToUnix)
		if err != nil {
			return SyncResult{}, err
		}
		if value.TotalPages > 0 {
			totalPages = value.TotalPages
		}
		if err := appendCheckpointScrobbles(s.path(SyncCheckpointScrobblesFile), checkpoint.SyncID, value.Scrobbles); err != nil {
			return SyncResult{}, err
		}
		collected = append(collected, value.Scrobbles...)
		checkpoint.NextPage = page + 1
		checkpoint.TotalPages = totalPages
		if err := writeJSON(s.path(SyncCheckpointFile), checkpoint); err != nil {
			return SyncResult{}, err
		}
		if report != nil {
			report(SyncProgress{Phase: "fetching", PagesFetched: page, TotalPages: totalPages, Scrobbles: len(collected), Resumed: resumed})
		}
	}
	merged := collected
	if !full {
		merged = append(append([]Scrobble(nil), old...), collected...)
	}
	seen := map[string]bool{}
	unique := merged[:0]
	for _, v := range merged {
		k := dedupeKey(v)
		if !seen[k] {
			seen[k] = true
			unique = append(unique, v)
		}
	}
	sort.Slice(unique, func(i, j int) bool { return unique[i].PlayedAtUTC.Before(unique[j].PlayedAtUTC) })
	if err := writeScrobbles(s.path(ScrobblesFile), unique); err != nil {
		return SyncResult{}, err
	}
	s.clearSyncCheckpoint()
	priorMatches := make([]Match, 0, len(s.index.Matches))
	for _, v := range s.index.Matches {
		priorMatches = append(priorMatches, v)
	}
	priorSpotify := make([]SpotifyMetadata, 0, len(s.index.Spotify))
	for _, v := range s.index.Spotify {
		priorSpotify = append(priorSpotify, v)
	}
	s.index = buildIndex(unique, priorMatches, priorSpotify)
	s.resolve(tracks)
	_ = s.saveMatches()
	enrichErr := s.enrichSpotify(ctx, tracks, report)
	spotifyComplete := enrichErr == nil
	s.index.SpotifyComplete = spotifyComplete
	now := client.now().UTC()
	s.index.LastSyncUTC = &now
	st := s.Status()
	result := SyncResult{PagesFetched: totalPages, TotalPages: totalPages, Scrobbles: len(unique), Matched: st.Matched, Unresolved: st.Unresolved, SpotifyComplete: spotifyComplete}
	if errors.Is(enrichErr, context.Canceled) {
		return result, enrichErr
	}
	return result, nil
}

func (s *Service) prepareSyncCheckpoint(to int64, from *int64) (SyncCheckpoint, []Scrobble, bool, error) {
	var checkpoint SyncCheckpoint
	if err := readJSON(s.path(SyncCheckpointFile), &checkpoint, "Last.fm sync checkpoint"); err != nil {
		return SyncCheckpoint{}, nil, false, err
	}
	if checkpoint.SchemaVersion != 0 {
		if checkpoint.SchemaVersion != SchemaVersion {
			return SyncCheckpoint{}, nil, false, fmt.Errorf("Last.fm sync checkpoint schemaVersion is %d, want %d", checkpoint.SchemaVersion, SchemaVersion)
		}
		if checkpoint.Username == s.Username && sameUnix(checkpoint.FromUnix, from) && checkpoint.SyncID != "" && checkpoint.ToUnix > 0 && checkpoint.NextPage > 0 {
			values, err := readCheckpointScrobbles(s.path(SyncCheckpointScrobblesFile), checkpoint.SyncID)
			if err != nil {
				return SyncCheckpoint{}, nil, false, err
			}
			return checkpoint, values, true, nil
		}
	}
	syncID, err := randomExportID()
	if err != nil {
		return SyncCheckpoint{}, nil, false, err
	}
	checkpoint = SyncCheckpoint{SchemaVersion: SchemaVersion, SyncID: syncID, Username: s.Username, FromUnix: cloneUnix(from), ToUnix: to, NextPage: 1, TotalPages: 1}
	if err := writeJSON(s.path(SyncCheckpointFile), checkpoint); err != nil {
		return SyncCheckpoint{}, nil, false, err
	}
	if err := atomicWrite(s.path(SyncCheckpointScrobblesFile), func(*os.File) error { return nil }); err != nil {
		return SyncCheckpoint{}, nil, false, err
	}
	return checkpoint, nil, false, nil
}

func sameUnix(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
func cloneUnix(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
func (s *Service) clearSyncCheckpoint() {
	_ = os.Remove(s.path(SyncCheckpointFile))
	_ = os.Remove(s.path(SyncCheckpointScrobblesFile))
}

func (s *Service) enrichSpotify(ctx context.Context, tracks []library.Track, report func(SyncProgress)) error {
	total := 0
	for _, t := range tracks {
		if t.SpotifyURI != "" {
			total++
		}
	}
	current := 0
	for _, t := range tracks {
		if t.SpotifyURI == "" {
			continue
		}
		current++
		if report != nil {
			report(SyncProgress{Phase: "spotify", SpotifyCurrent: current, SpotifyTotal: total})
		}
		if _, ok := s.index.Spotify[t.SpotifyURI]; ok {
			continue
		}
		if s.Spotify == nil {
			return fmt.Errorf("Spotify metadata unavailable")
		}
		v, err := spotify.RetryRateLimit(ctx, nil, func() (spotify.Track, error) { return s.Spotify.Track(ctx, t.SpotifyURI) })
		if err != nil {
			return err
		}
		artists := make([]string, len(v.Artists))
		for i := range v.Artists {
			artists[i] = v.Artists[i].Name
		}
		s.index.Spotify[t.SpotifyURI] = SpotifyMetadata{URI: v.URI, Name: v.Name, Artists: artists, Album: v.Album.Name, ReleaseDate: v.Album.ReleaseDate, DurationMS: v.DurationMS, ISRC: v.ExternalIDs["isrc"]}
		if err := s.saveSpotify(); err != nil {
			return err
		}
	}
	s.resolve(tracks)
	return s.saveMatches()
}
func (s *Service) saveSpotify() error {
	values := make([]SpotifyMetadata, 0, len(s.index.Spotify))
	for _, v := range s.index.Spotify {
		values = append(values, v)
	}
	sort.Slice(values, func(i, j int) bool { return values[i].URI < values[j].URI })
	return writeJSON(s.path(SpotifyCacheFile), SpotifyCache{SchemaVersion: SchemaVersion, Tracks: values})
}
func (s *Service) ResetAgentDecisions(tracks []library.Track) error {
	for k, v := range s.index.Matches {
		if v.Provenance == "agent" {
			delete(s.index.Matches, k)
		}
	}
	s.resolve(tracks)
	return s.saveMatches()
}

func normalizedCredit(artists []string) string { return strings.Join(artists, ", ") }
