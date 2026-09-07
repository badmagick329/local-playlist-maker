package tracksession

import (
	"context"
	"errors"
	"time"

	"playlistmaker/charm/internal/tracking"
)

type queuedPlay struct {
	id                string
	position          int
	track             tracking.Track
	completed         bool
	videoStartedAt    time.Time
	trackingStartedAt time.Time
	videoDuration     time.Duration
}

// playQueue separates video progress from Spotify progress. Each occurrence is
// a distinct object, so skipping a later video never stops an earlier song's tail
// or removes another occurrence of a repeated track.
type playQueue struct {
	runtime     *Runtime
	pending     []*queuedPlay
	active      *queuedPlay
	video       *queuedPlay
	statusError string
}

func (q *playQueue) load(ctx context.Context, id string, position int, track tracking.Track) error {
	play := &queuedPlay{id: id, position: position, track: track, videoStartedAt: time.Now()}
	q.video = play
	q.pending = append(q.pending, play)
	return q.advance(ctx)
}

func (q *playQueue) end(ctx context.Context, reason string) {
	play := q.video
	q.video = nil
	if play == nil {
		return
	}
	if reason == "eof" {
		play.completed = true
		play.videoDuration = time.Since(play.videoStartedAt)
		return
	}
	if q.active == play {
		q.runtime.End(ctx)
		q.active = nil
	}
	for i, pending := range q.pending {
		if pending == play {
			q.pending = append(q.pending[:i], q.pending[i+1:]...)
			break
		}
	}
}

func (q *playQueue) tick(ctx context.Context) error {
	if q.active != nil && q.runtime.activeProvider == "spotify" {
		finished, err := q.runtime.Spotify.Finished(ctx)
		if err != nil {
			var terminal *tracking.PlaybackFailure
			if errors.As(err, &terminal) {
				q.runtime.diagnose(q.active.position, q.active.track.TrackID, "spotify", "", err.Error())
				return err
			}
			// A failed status read does not prove playback failed. Keep this play
			// queued for confirmation rather than terminating mpv or losing its tail.
			if q.statusError != err.Error() {
				q.runtime.diagnose(q.active.position, q.active.track.TrackID, "spotify", "", err.Error())
				q.statusError = err.Error()
			}
			return nil
		}
		if finished {
			q.runtime.End(ctx)
			q.active = nil
		}
	}
	// A local fallback can also start late behind Spotify. Give it the video's
	// playback time instead of starting and immediately stopping an ended video.
	if q.active != nil && q.active.completed && q.runtime.activeProvider != "spotify" &&
		(q.runtime.activeProvider == "untracked" || time.Since(q.active.trackingStartedAt) >= q.active.videoDuration) {
		q.runtime.End(ctx)
		q.active = nil
	}
	return q.advance(ctx)
}

func (q *playQueue) advance(ctx context.Context) error {
	for q.active == nil && len(q.pending) > 0 {
		play := q.pending[0]
		q.pending = q.pending[1:]
		if err := q.runtime.Load(ctx, play.position, play.track); err != nil {
			return err
		}
		q.active = play
		q.statusError = ""
		play.trackingStartedAt = time.Now()
		if play.completed && q.runtime.activeProvider == "untracked" {
			q.runtime.End(ctx)
			q.active = nil
		}
	}
	return nil
}

func (q *playQueue) idle() bool { return q.active == nil && len(q.pending) == 0 }
