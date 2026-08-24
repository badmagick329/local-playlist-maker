package tracksession

import (
	"context"
	"fmt"
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
	active           tracking.Player
	activeProvider   string
}

func (r *Runtime) Prepare(ctx context.Context, deviceName string, entries []Entry) error {
	needsSpotify := false
	for _, entry := range entries {
		needsSpotify = needsSpotify || entry.Track.SpotifyURI != ""
	}
	if needsSpotify && r.Spotify != nil {
		if err := r.Spotify.Preflight(ctx, deviceName); err == nil {
			r.spotifyAvailable = true
		}
	}
	for _, entry := range entries {
		if entry.Track.SpotifyURI != "" && r.spotifyAvailable || entry.Track.LocalAudioPath != "" || r.AllowUntracked {
			continue
		}
		return fmt.Errorf("%s - %s has no tracking route after Spotify preflight", entry.Track.Artist, entry.Track.Title)
	}
	return nil
}

func (r *Runtime) Load(ctx context.Context, position int, track tracking.Track) {
	r.stop(ctx)
	if track.SpotifyURI != "" && r.spotifyAvailable {
		if err := r.Spotify.Start(ctx, track); err == nil {
			r.active, r.activeProvider = r.Spotify, "spotify"
			r.diagnose(position, track.TrackID, "spotify", "", "")
			return
		} else {
			r.spotifyAvailable = false
			r.diagnose(position, track.TrackID, "spotify", "", err.Error())
		}
	}
	if track.LocalAudioPath != "" && r.Local != nil {
		if err := r.Local.Start(ctx, track); err == nil {
			r.active, r.activeProvider = r.Local, "foobar"
			reason := ""
			if track.SpotifyURI != "" {
				reason = "spotify unavailable"
			}
			r.diagnose(position, track.TrackID, "foobar", reason, "")
			return
		} else {
			r.diagnose(position, track.TrackID, "foobar", "", err.Error())
		}
	}
	r.active, r.activeProvider = tracking.Noop{}, "untracked"
	r.diagnose(position, track.TrackID, "untracked", "no tracking provider available", "")
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
