package tracksession

import (
	"context"
	"fmt"
	"strings"
	"time"

	"playlistmaker/charm/internal/tracking"
)

type SpotifyPlayer interface {
	tracking.Player
	Preflight(context.Context, string) error
}

type Runtime struct {
	Spotify          SpotifyPlayer
	Local            tracking.Player
	AllowUntracked   bool
	DiagnosticsPath  string
	spotifyAvailable bool
	spotifyPreflight error
	active           tracking.Player
	activeProvider   string
}

func (r *Runtime) Prepare(ctx context.Context, deviceName string, entries []Entry) error {
	r.spotifyAvailable = false
	r.spotifyPreflight = nil
	needsSpotify := false
	for _, entry := range entries {
		needsSpotify = needsSpotify || entry.Track.SpotifyURI != ""
	}
	if needsSpotify {
		if r.Spotify == nil {
			r.spotifyPreflight = fmt.Errorf("Spotify tracking is not configured")
		} else if err := r.Spotify.Preflight(ctx, deviceName); err == nil {
			r.spotifyAvailable = true
		} else {
			r.spotifyPreflight = err
		}
	}
	for _, entry := range entries {
		if entry.Track.SpotifyURI != "" && r.spotifyAvailable || entry.Track.LocalAudioPath != "" && r.Local != nil || r.AllowUntracked {
			continue
		}
		if entry.Track.SpotifyURI != "" && r.spotifyPreflight != nil {
			return fmt.Errorf("%s - %s Spotify preflight failed: %w", entry.Track.Artist, entry.Track.Title, r.spotifyPreflight)
		}
		return fmt.Errorf("%s - %s has no tracking route after Spotify preflight", entry.Track.Artist, entry.Track.Title)
	}
	return nil
}

func (r *Runtime) Load(ctx context.Context, position int, track tracking.Track) error {
	r.stop(ctx)
	failures := []string{}
	if track.SpotifyURI != "" && r.spotifyAvailable {
		if err := r.Spotify.Start(ctx, track); err == nil {
			r.active, r.activeProvider = r.Spotify, "spotify"
			r.diagnose(position, track.TrackID, "spotify", "", "")
			return nil
		} else {
			r.spotifyAvailable = false
			r.diagnose(position, track.TrackID, "spotify", "", err.Error())
			failures = append(failures, "Spotify: "+err.Error())
		}
	} else if track.SpotifyURI != "" && r.spotifyPreflight != nil {
		r.diagnose(position, track.TrackID, "spotify", "", r.spotifyPreflight.Error())
		failures = append(failures, "Spotify: "+r.spotifyPreflight.Error())
	}
	if track.LocalAudioPath != "" && r.Local != nil {
		if err := r.Local.Start(ctx, track); err == nil {
			r.active, r.activeProvider = r.Local, "foobar"
			reason := strings.Join(failures, "; ")
			r.diagnose(position, track.TrackID, "foobar", reason, "")
			return nil
		} else {
			r.diagnose(position, track.TrackID, "foobar", "", err.Error())
			failures = append(failures, "foobar: "+err.Error())
		}
	}
	reason := strings.Join(failures, "; ")
	if reason == "" {
		reason = "no tracking provider available"
	}
	if !r.AllowUntracked {
		r.diagnose(position, track.TrackID, "untracked", reason, "untracked playback is disabled")
		return fmt.Errorf("%s - %s cannot be tracked: %s", track.Artist, track.Title, reason)
	}
	r.active, r.activeProvider = tracking.Noop{}, "untracked"
	r.diagnose(position, track.TrackID, "untracked", reason, "")
	return nil
}

func (r *Runtime) End(ctx context.Context) { r.stop(ctx) }

func (r *Runtime) Close(ctx context.Context) {
	r.stop(ctx)
	if r.Spotify != nil {
		_ = r.Spotify.Close(ctx)
	}
	if r.Local != nil {
		_ = r.Local.Close(ctx)
	}
}

func (r *Runtime) stop(ctx context.Context) {
	if r.active != nil {
		_ = r.active.Stop(ctx)
	}
	r.active, r.activeProvider = nil, ""
}

func (r *Runtime) diagnose(position int, trackID, provider, fallback, message string) {
	if r.DiagnosticsPath == "" {
		return
	}
	_ = AppendDiagnostic(r.DiagnosticsPath, Diagnostic{EventAtUTC: time.Now().UTC(), PlaylistPosition: position, TrackID: trackID, Provider: provider, FallbackReason: fallback, Error: message})
}
