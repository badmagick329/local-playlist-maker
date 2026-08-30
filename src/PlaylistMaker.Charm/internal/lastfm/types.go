package lastfm

import (
	"time"

	"playlistmaker/charm/internal/library"
)

const (
	SchemaVersion               = 1
	ScrobblesFile               = "lastfm-scrobbles.jsonl"
	MatchesFile                 = "lastfm-matches.json"
	SpotifyCacheFile            = "spotify-track-cache.json"
	ReviewDirectory             = "lastfm-review"
	SyncCheckpointFile          = "lastfm-sync-checkpoint.json"
	SyncCheckpointScrobblesFile = "lastfm-sync-checkpoint.jsonl"
)

type Scrobble struct {
	SchemaVersion int       `json:"schemaVersion"`
	Artist        string    `json:"artist"`
	Title         string    `json:"title"`
	Album         string    `json:"album"`
	MBID          string    `json:"mbid,omitempty"`
	PlayedAtUTC   time.Time `json:"playedAtUtc"`
}

type AlbumCount struct {
	Name      string `json:"name"`
	PlayCount int    `json:"playCount"`
}
type Identity struct {
	Key              string
	Artist           string
	Title            string
	PlayCount        int
	FirstPlayedAtUTC time.Time
	LastPlayedAtUTC  time.Time
	PlayedAtUTC      []time.Time
	Albums           []AlbumCount
	MBIDs            []string
}

type Match struct {
	SourceKey            string `json:"sourceKey"`
	Artist               string `json:"artist"`
	Title                string `json:"title"`
	Status               string `json:"status"`
	TrackID              string `json:"trackId,omitempty"`
	Provenance           string `json:"provenance"`
	Reason               string `json:"reason"`
	CatalogueFingerprint string `json:"catalogueFingerprint,omitempty"`
	ExportID             string `json:"exportId,omitempty"`
}
type MatchFile struct {
	SchemaVersion int     `json:"schemaVersion"`
	Matches       []Match `json:"matches"`
}

type SpotifyMetadata struct {
	URI         string   `json:"uri"`
	Name        string   `json:"name"`
	Artists     []string `json:"artists"`
	Album       string   `json:"album"`
	ReleaseDate string   `json:"releaseDate"`
	DurationMS  int      `json:"durationMs"`
	ISRC        string   `json:"isrc,omitempty"`
}
type SpotifyCache struct {
	SchemaVersion int               `json:"schemaVersion"`
	Tracks        []SpotifyMetadata `json:"tracks"`
}

type Index struct {
	Scrobbles       []Scrobble
	Identities      map[string]*Identity
	Matches         map[string]Match
	TrackPlays      map[string][]time.Time
	Fingerprint     string
	Spotify         map[string]SpotifyMetadata
	LastSyncUTC     *time.Time
	SpotifyComplete bool
	Error           string
}

type Status struct {
	Configured       bool
	Scrobbles        int
	FirstPlayedAtUTC *time.Time
	LastPlayedAtUTC  *time.Time
	Matched          int
	Unresolved       int
	LastSyncUTC      *time.Time
	SpotifyComplete  bool
	Error            string
	CheckpointPages  int
	CheckpointTotal  int
}

type SyncProgress struct {
	Phase                                                             string
	PagesFetched, TotalPages, Scrobbles, SpotifyCurrent, SpotifyTotal int
	Resumed                                                           bool
}
type SyncResult struct {
	PagesFetched, TotalPages, Scrobbles, Matched, Unresolved int
	SpotifyComplete                                          bool
}

type SyncCheckpoint struct {
	SchemaVersion int    `json:"schemaVersion"`
	SyncID        string `json:"syncId"`
	Username      string `json:"username"`
	FromUnix      *int64 `json:"fromUnix,omitempty"`
	ToUnix        int64  `json:"toUnix"`
	NextPage      int    `json:"nextPage"`
	TotalPages    int    `json:"totalPages"`
}

type checkpointScrobble struct {
	SchemaVersion int      `json:"schemaVersion"`
	SyncID        string   `json:"syncId"`
	Scrobble      Scrobble `json:"scrobble"`
}

type MixMethod int

const (
	WeightedRandom MixMethod = iota
	TopPlayed
	UniformRandom
	Rediscover
)

func (m MixMethod) String() string {
	switch m {
	case TopPlayed:
		return "Top played"
	case UniformRandom:
		return "Uniform random"
	case Rediscover:
		return "Rediscover"
	default:
		return "Weighted random"
	}
}
func (m MixMethod) Next(delta int) MixMethod {
	values := []MixMethod{WeightedRandom, TopPlayed, UniformRandom, Rediscover}
	for i, v := range values {
		if v == m {
			return values[(i+delta+len(values))%len(values)]
		}
	}
	return WeightedRandom
}

type QueueAction int

const (
	ReplaceQueue QueueAction = iota
	AppendQueue
)

func (a QueueAction) String() string {
	if a == AppendQueue {
		return "Append"
	}
	return "Replace"
}

type MixRequest struct {
	Tracks             []library.Track
	Query              library.Query
	Primary, Secondary *library.DateRange
	SecondaryPercent   int
	Count              int
	Method             MixMethod
	Action             QueueAction
	QueuedTrackIDs     map[string]bool
	SelectionStrategy  library.SelectionStrategy
}
type MixResult struct {
	Variants           []library.Variant
	Requested, Created int
}
