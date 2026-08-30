package lastfm

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"playlistmaker/charm/internal/library"
	"playlistmaker/charm/internal/videoname"
)

type Candidate struct {
	TrackID  string   `json:"trackId"`
	Score    int      `json:"score"`
	Evidence []string `json:"evidence"`
}
type ReviewSource struct {
	Key              string       `json:"key"`
	Artist           string       `json:"artist"`
	Title            string       `json:"title"`
	PlayCount        int          `json:"playCount"`
	FirstPlayedAtUTC time.Time    `json:"firstPlayedAtUtc"`
	LastPlayedAtUTC  time.Time    `json:"lastPlayedAtUtc"`
	Albums           []AlbumCount `json:"albums"`
	MBIDs            []string     `json:"mbids"`
}
type ReviewCase struct {
	CaseID     string       `json:"caseId"`
	Source     ReviewSource `json:"source"`
	Candidates []Candidate  `json:"candidates"`
}
type ReviewSpotify struct {
	URI         string `json:"uri"`
	URL         string `json:"url"`
	Artist      string `json:"artist"`
	Title       string `json:"title"`
	Album       string `json:"album"`
	ReleaseDate string `json:"releaseDate"`
	DurationMS  int    `json:"durationMs"`
	ISRC        string `json:"isrc"`
}
type ReviewVideo struct {
	Filename string           `json:"filename"`
	Category library.Category `json:"category"`
	Date     string           `json:"date"`
}
type ReviewTrack struct {
	TrackID     string        `json:"trackId"`
	Artist      string        `json:"artist"`
	Title       string        `json:"title"`
	ReleaseDate string        `json:"releaseDate"`
	Spotify     ReviewSpotify `json:"spotify"`
	Videos      []ReviewVideo `json:"videos"`
}
type Review struct {
	SchemaVersion        int           `json:"schemaVersion"`
	ExportID             string        `json:"exportId"`
	CatalogueFingerprint string        `json:"catalogueFingerprint"`
	GeneratedAtUTC       time.Time     `json:"generatedAtUtc"`
	Cases                []ReviewCase  `json:"cases"`
	Catalogue            []ReviewTrack `json:"catalogue"`
}
type Decision struct {
	CaseID   string  `json:"caseId"`
	Decision string  `json:"decision"`
	TrackID  *string `json:"trackId,omitempty"`
	Reason   string  `json:"reason"`
}
type Decisions struct {
	SchemaVersion int        `json:"schemaVersion"`
	ExportID      string     `json:"exportId"`
	Decisions     []Decision `json:"decisions"`
}
type ImportResult struct{ Matched, NoMatch, NeedsHuman, Missing, Invalid int }

func CaseID(sourceKey string) string {
	sum := sha256.Sum256([]byte(sourceKey))
	return "lfm_" + hex.EncodeToString(sum[:12])
}
func randomExportID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *Service) ExportReview(tracks []library.Track, now time.Time) (string, error) {
	exportID, err := randomExportID()
	if err != nil {
		return "", err
	}
	review := Review{SchemaVersion: SchemaVersion, ExportID: exportID, CatalogueFingerprint: CatalogueFingerprint(tracks), GeneratedAtUTC: now.UTC(), Cases: []ReviewCase{}, Catalogue: []ReviewTrack{}}
	for key, id := range s.index.Identities {
		if m, ok := s.index.Matches[key]; ok && (m.Status == "match" || m.Status == "no_match") {
			continue
		}
		review.Cases = append(review.Cases, ReviewCase{CaseID: CaseID(key), Source: ReviewSource{Key: key, Artist: id.Artist, Title: id.Title, PlayCount: id.PlayCount, FirstPlayedAtUTC: id.FirstPlayedAtUTC, LastPlayedAtUTC: id.LastPlayedAtUTC, Albums: nonNilAlbums(id.Albums), MBIDs: nonNilStrings(id.MBIDs)}, Candidates: rankCandidates(*id, tracks, s.index.Spotify)})
	}
	sort.Slice(review.Cases, func(i, j int) bool { return review.Cases[i].CaseID < review.Cases[j].CaseID })
	for _, t := range tracks {
		rt := ReviewTrack{TrackID: t.ID, Artist: t.Artist, Title: t.Title, ReleaseDate: t.ReleaseDateLabel, Videos: []ReviewVideo{}}
		if md, ok := s.index.Spotify[t.SpotifyURI]; ok {
			rt.Spotify = ReviewSpotify{URI: md.URI, URL: spotifyURL(md.URI), Artist: normalizedCredit(md.Artists), Title: md.Name, Album: md.Album, ReleaseDate: md.ReleaseDate, DurationMS: md.DurationMS, ISRC: md.ISRC}
		}
		for _, v := range t.Variants {
			rt.Videos = append(rt.Videos, ReviewVideo{Filename: v.Filename, Category: v.Category, Date: v.DateLabel})
		}
		sort.Slice(rt.Videos, func(i, j int) bool { return rt.Videos[i].Filename < rt.Videos[j].Filename })
		review.Catalogue = append(review.Catalogue, rt)
	}
	sort.Slice(review.Catalogue, func(i, j int) bool { return review.Catalogue[i].TrackID < review.Catalogue[j].TrackID })
	dir := s.path(ReviewDirectory)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	instructions := `# Last.fm matching review

Read review.json and consider only its unresolved cases. Select only a trackId present in the exported catalogue. Use the supplied metadata and web research when useful. Choose match only when the identity is clear, no_match only when the song is absent from the exported catalogue, and needs_human when uncertainty remains.

Write decisions.json without modifying instructions.md or review.json. Copy schemaVersion and exportId exactly. Each decision needs caseId, decision, and a brief evidence-based reason. A match also needs trackId; the other decisions must not include it.
`
	if err := atomicWrite(filepath.Join(dir, "instructions.md"), func(f *os.File) error { _, err := f.WriteString(instructions); return err }); err != nil {
		return "", err
	}
	if err := writeJSON(filepath.Join(dir, "review.json"), review); err != nil {
		return "", err
	}
	return dir, nil
}

func rankCandidates(id Identity, tracks []library.Track, cache map[string]SpotifyMetadata) []Candidate {
	result := make([]Candidate, 0, len(tracks))
	sourceArtist, sourceTitle := videoname.Normalize(id.Artist), videoname.Normalize(id.Title)
	for _, t := range tracks {
		artist, title := videoname.Normalize(t.Artist), videoname.Normalize(t.Title)
		artistForward, _ := library.FuzzyScore(artist, sourceArtist)
		artistReverse, _ := library.FuzzyScore(sourceArtist, artist)
		titleForward, _ := library.FuzzyScore(title, sourceTitle)
		titleReverse, _ := library.FuzzyScore(sourceTitle, title)
		score := artistForward + artistReverse + titleForward + titleReverse
		evidence := []string{}
		if artist == sourceArtist {
			score += 10000
			evidence = append(evidence, "artist agrees")
		}
		if title == sourceTitle {
			score += 20000
			evidence = append(evidence, "title agrees")
		}
		if md, ok := cache[t.SpotifyURI]; ok {
			for _, a := range md.Artists {
				if videoname.Normalize(a) == sourceArtist {
					score += 5000
					evidence = append(evidence, "Spotify artist agrees")
				}
			}
			if videoname.Normalize(md.Name) == sourceTitle {
				score += 10000
				evidence = append(evidence, "Spotify title agrees")
			}
		}
		if score > 0 {
			result = append(result, Candidate{TrackID: t.ID, Score: score, Evidence: nonNilStrings(evidence)})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Score != result[j].Score {
			return result[i].Score > result[j].Score
		}
		return result[i].TrackID < result[j].TrackID
	})
	if len(result) > 10 {
		result = result[:10]
	}
	return result
}

func (s *Service) ImportDecisions(tracks []library.Track) (ImportResult, error) {
	dir := s.path(ReviewDirectory)
	var review Review
	if err := readJSON(filepath.Join(dir, "review.json"), &review, "Last.fm review"); err != nil {
		return ImportResult{}, err
	}
	if review.SchemaVersion != SchemaVersion {
		return ImportResult{}, fmt.Errorf("Last.fm review schemaVersion is unsupported")
	}
	if review.CatalogueFingerprint != CatalogueFingerprint(tracks) {
		return ImportResult{}, fmt.Errorf("Last.fm review catalogue fingerprint is stale")
	}
	b, err := os.ReadFile(filepath.Join(dir, "decisions.json"))
	if err != nil {
		return ImportResult{}, fmt.Errorf("read Last.fm decisions: %w", err)
	}
	var document Decisions
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return ImportResult{}, fmt.Errorf("parse Last.fm decisions: %w", err)
	}
	if trailingErr := decoder.Decode(&struct{}{}); trailingErr != io.EOF {
		return ImportResult{}, fmt.Errorf("parse Last.fm decisions: trailing content")
	}
	if document.SchemaVersion != SchemaVersion {
		return ImportResult{}, fmt.Errorf("Last.fm decisions schemaVersion is unsupported")
	}
	if document.ExportID != review.ExportID {
		return ImportResult{}, fmt.Errorf("Last.fm decisions exportId does not match review")
	}
	cases := map[string]ReviewCase{}
	for _, v := range review.Cases {
		cases[v.CaseID] = v
	}
	trackIDs := map[string]bool{}
	for _, t := range tracks {
		trackIDs[t.ID] = true
	}
	seen := map[string]bool{}
	occurrences := map[string]int{}
	for _, decision := range document.Decisions {
		occurrences[decision.CaseID]++
	}
	result := ImportResult{}
	valid := []Match{}
	for _, d := range document.Decisions {
		c, ok := cases[d.CaseID]
		invalid := !ok || occurrences[d.CaseID] != 1 || strings.TrimSpace(d.Reason) == ""
		seen[d.CaseID] = true
		if !invalid {
			switch d.Decision {
			case "match":
				invalid = d.TrackID == nil || *d.TrackID == "" || !trackIDs[*d.TrackID]
				if !invalid {
					valid = append(valid, Match{SourceKey: c.Source.Key, Artist: c.Source.Artist, Title: c.Source.Title, Status: "match", TrackID: *d.TrackID, Provenance: "agent", Reason: d.Reason, ExportID: review.ExportID})
					result.Matched++
				}
			case "no_match":
				invalid = d.TrackID != nil
				if !invalid {
					valid = append(valid, Match{SourceKey: c.Source.Key, Artist: c.Source.Artist, Title: c.Source.Title, Status: "no_match", Provenance: "agent", Reason: d.Reason, CatalogueFingerprint: review.CatalogueFingerprint, ExportID: review.ExportID})
					result.NoMatch++
				}
			case "needs_human":
				invalid = d.TrackID != nil
				if !invalid {
					result.NeedsHuman++
				}
			default:
				invalid = true
			}
		}
		if invalid {
			result.Invalid++
		}
	}
	for id := range cases {
		if !seen[id] {
			result.Missing++
		}
	}
	for _, v := range valid {
		s.index.Matches[v.SourceKey] = v
	}
	s.rebuildTrackPlays()
	if len(valid) > 0 {
		if err := s.saveMatches(); err != nil {
			return ImportResult{}, err
		}
	}
	return result, nil
}
func spotifyURL(uri string) string {
	if strings.HasPrefix(uri, "spotify:track:") {
		return "https://open.spotify.com/track/" + strings.TrimPrefix(uri, "spotify:track:")
	}
	return ""
}
func nonNilStrings(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}
func nonNilAlbums(v []AlbumCount) []AlbumCount {
	if v == nil {
		return []AlbumCount{}
	}
	return v
}
