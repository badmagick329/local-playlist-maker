package lastfm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"playlistmaker/charm/internal/library"
)

func TestReviewExportAndDecisionImportRoundTrip(t *testing.T) {
	tracks := []library.Track{testTrack("one", "Catalogue", "Song"), testTrack("two", "Other", "Track")}
	s := Service{DataDirectory: t.TempDir()}
	s.index = buildIndex([]Scrobble{{Artist: "Source", Title: "Song", Album: "Album", MBID: "m", PlayedAtUTC: time.Unix(1, 0)}, {Artist: "Absent", Title: "None", PlayedAtUTC: time.Unix(2, 0)}, {Artist: "Maybe", Title: "Unsure", PlayedAtUTC: time.Unix(3, 0)}}, nil, nil)
	s.resolve(tracks)
	dir, err := s.ExportReview(tracks, time.Unix(99, 0), nil)
	if err != nil {
		t.Fatal(err)
	}
	var review Review
	if err := readJSON(filepath.Join(dir, "review.json"), &review, "review"); err != nil {
		t.Fatal(err)
	}
	if review.SchemaVersion != 1 || review.GeneratedAtUTC.Unix() != 99 || len(review.Cases) != 3 || len(review.Catalogue) != 2 || review.Catalogue[0].Videos[0].Filename == "" {
		t.Fatalf("review=%#v", review)
	}
	one := "one"
	decisions := Decisions{SchemaVersion: 1, ExportID: review.ExportID, Decisions: []Decision{{CaseID: review.Cases[0].CaseID, Decision: "match", TrackID: &one, Reason: "clear"}, {CaseID: review.Cases[1].CaseID, Decision: "no_match", Reason: "absent"}, {CaseID: review.Cases[2].CaseID, Decision: "needs_human", Reason: "uncertain"}, {CaseID: "unknown", Decision: "match", TrackID: &one, Reason: "bad"}}}
	if err := writeJSON(filepath.Join(dir, "decisions.json"), decisions); err != nil {
		t.Fatal(err)
	}
	result, err := s.ImportDecisions(tracks)
	if err != nil {
		t.Fatal(err)
	}
	if result.Matched != 1 || result.NoMatch != 1 || result.NeedsHuman != 1 || result.Invalid != 1 {
		t.Fatalf("result=%#v", result)
	}
	before := len(tracks)
	if len(tracks) != before {
		t.Fatal("catalogue mutated")
	}
}

func TestDecisionImportRejectsStaleEnvelopeAndSkipsInvalidRows(t *testing.T) {
	tracks := []library.Track{testTrack("one", "A", "B")}
	s := Service{DataDirectory: t.TempDir()}
	s.index = buildIndex([]Scrobble{{Artist: "X", Title: "Y", PlayedAtUTC: time.Unix(1, 0)}}, nil, nil)
	s.resolve(tracks)
	dir, _ := s.ExportReview(tracks, time.Now(), nil)
	var review Review
	_ = readJSON(filepath.Join(dir, "review.json"), &review, "review")
	bad := Decisions{SchemaVersion: 1, ExportID: "stale"}
	_ = writeJSON(filepath.Join(dir, "decisions.json"), bad)
	if _, err := s.ImportDecisions(tracks); err == nil {
		t.Fatal("stale export accepted")
	}
	bad.ExportID = review.ExportID
	missing := "missing"
	bad.Decisions = []Decision{{CaseID: review.Cases[0].CaseID, Decision: "match", TrackID: &missing, Reason: "x"}, {CaseID: review.Cases[0].CaseID, Decision: "no_match", Reason: "duplicate"}}
	_ = writeJSON(filepath.Join(dir, "decisions.json"), bad)
	result, err := s.ImportDecisions(tracks)
	if err != nil {
		t.Fatal(err)
	}
	if result.Invalid != 2 {
		t.Fatalf("result=%#v", result)
	}
}

func TestDecisionImportAppliesToCasesStillUnresolvedAfterCatalogueChanges(t *testing.T) {
	tracks := []library.Track{testTrack("one", "A", "B")}
	s := Service{DataDirectory: t.TempDir()}
	s.index = buildIndex([]Scrobble{{Artist: "Absent", Title: "Song", PlayedAtUTC: day(1)}, {Artist: "Later", Title: "Added", PlayedAtUTC: day(2)}, {Artist: "Fuzzy", Title: "Name", PlayedAtUTC: day(3)}, {Artist: "Twice", Title: "Added", PlayedAtUTC: day(4)}}, nil, nil)
	s.resolve(tracks)
	dir, err := s.ExportReview(tracks, time.Now(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var review Review
	if err := readJSON(filepath.Join(dir, "review.json"), &review, "review"); err != nil {
		t.Fatal(err)
	}
	caseFor := func(artist, title string) string { return CaseID(SourceKey(artist, title)) }
	one, fuzzy := "one", "fuzzy"
	decisions := Decisions{SchemaVersion: 1, ExportID: review.ExportID, Decisions: []Decision{
		{CaseID: caseFor("Absent", "Song"), Decision: "no_match", Reason: "absent"},
		{CaseID: caseFor("Later", "Added"), Decision: "match", TrackID: &one, Reason: "wrong guess"},
		{CaseID: caseFor("Fuzzy", "Name"), Decision: "match", TrackID: &fuzzy, Reason: "same song, spelled differently"},
		{CaseID: caseFor("Twice", "Added"), Decision: "no_match", Reason: "absent at export"},
	}}
	if err := writeJSON(filepath.Join(dir, "decisions.json"), decisions); err != nil {
		t.Fatal(err)
	}
	changed := append(tracks, testTrack("later", "Later", "Added"), testTrack("fuzzy", "Fuzzy Band", "Name"), testTrack("twice-a", "Twice", "Added"), testTrack("twice-b", "Twice", "Added"))
	if _, err := s.RefreshCatalogue(changed); err != nil {
		t.Fatal(err)
	}
	result, err := s.ImportDecisions(changed)
	if err != nil {
		t.Fatalf("catalogue change rejected import: %v", err)
	}
	if result.Matched != 1 || result.NoMatch != 2 || result.AlreadyResolved != 1 || result.Invalid != 0 {
		t.Fatalf("result=%#v", result)
	}
	if m := s.index.Matches[SourceKey("Later", "Added")]; m.Provenance != "auto" || m.TrackID != "later" {
		t.Fatalf("decision overwrote a case resolved after export: %#v", m)
	}
	if m := s.index.Matches[SourceKey("Fuzzy", "Name")]; m.TrackID != "fuzzy" {
		t.Fatalf("match to a track added after export not applied: %#v", m)
	}
	if s.index.Matches[SourceKey("Absent", "Song")].Status != "no_match" {
		t.Fatal("no-match not applied")
	}
	if !s.unresolved(SourceKey("Twice", "Added")) {
		t.Fatal("no-match kept although the catalogue gained its identity")
	}
	again, err := s.ImportDecisions(changed)
	if err != nil || again.AlreadyResolved != 3 || again.Matched+again.NoMatch != 1 {
		t.Fatalf("re-import=%#v err=%v", again, err)
	}
}

type sequenceRandom struct {
	values []int
	index  int
}

func (r *sequenceRandom) Intn(n int) int {
	v := r.values[r.index%len(r.values)] % n
	r.index++
	return v
}

func TestMixMethodsInclusivePeriodsBlendShortageAndQueueActions(t *testing.T) {
	tracks := []library.Track{testTrack("a", "A", "A"), testTrack("b", "B", "B"), testTrack("c", "C", "C"), testTrack("d", "D", "D")}
	s := Service{Random: &sequenceRandom{values: []int{0, 1, 0, 0}}}
	s.index.TrackPlays = map[string][]time.Time{"a": {day(1), day(2), day(3)}, "b": {day(2), day(4)}, "c": {day(5)}, "d": {day(10)}}
	query := library.Query{Enabled: map[library.Category]bool{library.MusicVideo: true}}
	primary, _ := library.ParseDateRange("2026-01-01..2026-01-05")
	secondary, _ := library.ParseDateRange("2026-01-10")
	for _, method := range []MixMethod{TopPlayed, WeightedRandom, UniformRandom, Rediscover} {
		result, err := s.BuildMix(MixRequest{Tracks: tracks, Query: query, Primary: primary, Count: 3, Method: method, SelectionStrategy: library.DefaultSelection})
		if err != nil || result.Created != 3 {
			t.Fatalf("method %s result=%#v err=%v", method, result, err)
		}
		seen := map[string]bool{}
		for _, v := range result.Variants {
			if seen[v.TrackID] {
				t.Fatalf("method %s duplicated %s", method, v.TrackID)
			}
			seen[v.TrackID] = true
		}
	}
	blend, err := s.BuildMix(MixRequest{Tracks: tracks, Query: query, Primary: primary, Secondary: secondary, SecondaryPercent: 50, Count: 4, Method: TopPlayed})
	if err != nil || blend.Created != 4 {
		t.Fatalf("blend=%#v err=%v", blend, err)
	}
	if blend.Variants[1].TrackID != "d" {
		t.Fatalf("secondary not evenly interleaved: %#v", blend.Variants)
	}
	appendResult, err := s.BuildMix(MixRequest{Tracks: tracks, Query: query, Primary: primary, Count: 3, Method: TopPlayed, Action: AppendQueue, QueuedTrackIDs: map[string]bool{"a": true}})
	if err != nil || appendResult.Created != 2 {
		t.Fatalf("append=%#v err=%v", appendResult, err)
	}
}

func TestMixRespectsCurrentFilterAndBlankAllHistory(t *testing.T) {
	a, b := testTrack("a", "A", "A"), testTrack("b", "B", "B")
	b.Variants[0].Category = library.Concert
	s := Service{}
	s.index.TrackPlays = map[string][]time.Time{"a": {day(1)}, "b": {day(2)}}
	query := library.Query{Enabled: map[library.Category]bool{library.MusicVideo: true}}
	result, err := s.BuildMix(MixRequest{Tracks: []library.Track{a, b}, Query: query, Count: 10, Method: TopPlayed})
	if err != nil || result.Created != 1 || result.Variants[0].TrackID != "a" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}
func day(day int) time.Time { return time.Date(2026, 1, day, 12, 0, 0, 0, time.UTC) }

func TestReviewJSONHasDeterministicArrayOrdering(t *testing.T) {
	s := Service{DataDirectory: t.TempDir()}
	s.index = buildIndex([]Scrobble{{Artist: "Z", Title: "Z", PlayedAtUTC: day(2)}, {Artist: "A", Title: "A", PlayedAtUTC: day(1)}}, nil, nil)
	tracks := []library.Track{testTrack("z", "Z", "Z"), testTrack("a", "A", "A")}
	s.resolve(nil)
	dir, err := s.ExportReview(tracks, time.Unix(1, 0), nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "review.json"))
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(b, &value); err != nil {
		t.Fatal(err)
	}
	catalogue := value["catalogue"].([]any)
	if catalogue[0].(map[string]any)["trackId"] != "a" {
		t.Fatal("catalogue not sorted")
	}
}
