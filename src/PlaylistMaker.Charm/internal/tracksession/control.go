package tracksession

import (
	"context"
	"encoding/json"
	"os"
	"time"
)

type recoverablePlayer interface {
	Checkpoint() json.RawMessage
	Restore(context.Context, json.RawMessage) error
}

type controlledPlayer interface {
	Intent(bool, *int)
	TrackingStatus() (string, string)
	Retry()
}

// Status is a lease from the helper's event loop, not a process-liveness claim.
// IntentSequence prevents an old healthy response releasing a newer user hold.
type Status struct {
	SessionID      string `json:"sessionId"`
	OccurrenceID   string `json:"occurrenceId"`
	IntentSequence int    `json:"intentSequence"`
	Revision       int    `json:"revision"`
	Heartbeat      int64  `json:"heartbeat"`
	State          string `json:"state"`
	Message        string `json:"message"`
	Hold           bool   `json:"hold"`
}

func (q *playQueue) intent(e Event) bool {
	if e.PositionMS != nil && *e.PositionMS < 0 {
		return false
	}
	if q.video == nil || e.OccurrenceID != q.video.id || e.IntentSequence <= q.intentSequence {
		return false
	}
	q.intentSequence = e.IntentSequence
	q.paused, q.target = e.Paused, e.PositionMS
	return true
}

func (q *playQueue) status(session string, revision int) Status {
	s := Status{SessionID: session, IntentSequence: q.intentSequence, Revision: revision, Heartbeat: time.Now().UnixMilli(), State: "playing", Message: "Tracking playback"}
	if q.video != nil {
		s.OccurrenceID = q.video.id
	}
	if q.blocked != "" {
		s.State, s.Message, s.Hold = "blocked", q.blocked, true
		return s
	}
	if q.active != nil && q.runtime.activeProvider == "spotify" {
		if p, ok := q.runtime.Spotify.(controlledPlayer); ok {
			s.State, s.Message = p.TrackingStatus()
			s.Hold = s.State == "recovering" || s.State == "pausing" || s.State == "blocked"
			if s.State == "" {
				s.State, s.Message = "starting", "Waiting for Spotify to start the song"
				if s.Hold = !q.inStartGrace(); s.Hold {
					s.Message = "Spotify has not confirmed the song; video held"
				}
			}
		}
	} else if q.idle() {
		s.State, s.Message = "tracking-completed", "Tracking completed"
	}
	if q.paused && !s.Hold && s.State != "catch-up" {
		s.State, s.Message = "user-paused", "Video paused"
	}
	return s
}

// Holding an already-playing video for every late song start pauses it on
// each routine switch. A late start gets a short confirmation window instead;
// a start that is still unconfirmed afterwards holds the video as before.
const (
	startGrace         = 5 * time.Second
	lateStartThreshold = 2 * time.Second
)

func (q *playQueue) inStartGrace() bool {
	a := q.active
	if a == nil || a.trackingStartedAt.IsZero() {
		return false
	}
	late := a != q.video || a.trackingStartedAt.Sub(a.videoStartedAt) >= lateStartThreshold
	return late && q.clock().Sub(a.trackingStartedAt) < startGrace
}

func writeJSON(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return atomicWrite(path, append(data, '\n'), 0600)
}

// Preserve occurrence identity and completion evidence separately from events.
// Restart is conservative: an interrupted command is never automatically replayed.
type savedPlay struct {
	ID                                string
	Position                          int
	Completed                         bool
	VideoStartedAt, TrackingStartedAt time.Time
	VideoDuration                     time.Duration
}
type checkpoint struct {
	Evidence       json.RawMessage
	Offset         int
	Active, Video  string
	Plays          []savedPlay
	Pending        []string
	Paused         bool
	Target         *int
	IntentSequence int
	Provider       string
	InputEnded     bool
}

func (q *playQueue) save(path string, offset int, inputEnded bool) error {
	c := checkpoint{Offset: offset, Paused: q.paused, Target: q.target, IntentSequence: q.intentSequence, Provider: q.runtime.activeProvider, InputEnded: inputEnded}
	if p, ok := q.runtime.Spotify.(recoverablePlayer); ok && q.runtime.activeProvider == "spotify" && q.runtime.active != nil {
		c.Evidence = p.Checkpoint()
	} else {
		c.Evidence = q.evidence
	}
	seen := map[string]bool{}
	add := func(p *queuedPlay) {
		if p != nil && !seen[p.id] {
			seen[p.id] = true
			c.Plays = append(c.Plays, savedPlay{p.id, p.position, p.completed, p.videoStartedAt, p.trackingStartedAt, p.videoDuration})
		}
	}
	add(q.active)
	add(q.video)
	if q.active != nil {
		c.Active = q.active.id
	}
	if q.video != nil {
		c.Video = q.video.id
	}
	for _, p := range q.pending {
		add(p)
		c.Pending = append(c.Pending, p.id)
	}
	return writeJSON(path, c)
}
func (q *playQueue) restore(path string, m Manifest) (int, bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	var c checkpoint
	if err = json.Unmarshal(data, &c); err != nil {
		return 0, false, err
	}
	plays := map[string]*queuedPlay{}
	for _, p := range c.Plays {
		if p.Position < 0 || p.Position >= len(m.Entries) {
			return 0, false, os.ErrInvalid
		}
		plays[p.ID] = &queuedPlay{id: p.ID, position: p.Position, track: m.Entries[p.Position].Track, completed: p.Completed, videoStartedAt: p.VideoStartedAt, trackingStartedAt: p.TrackingStartedAt, videoDuration: p.VideoDuration}
	}
	q.active, q.video = plays[c.Active], plays[c.Video]
	q.evidence = c.Evidence
	q.runtime.activeProvider = c.Provider
	q.suspended = true
	for _, id := range c.Pending {
		if plays[id] == nil {
			return 0, false, os.ErrInvalid
		}
		q.pending = append(q.pending, plays[id])
	}
	q.paused, q.target, q.intentSequence = c.Paused, c.Target, c.IntentSequence
	q.blocked = "Helper restarted; Play checks the saved occurrence before resuming. Ambiguous playback stays held."
	return c.Offset, c.InputEnded, nil
}
