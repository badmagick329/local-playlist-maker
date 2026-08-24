# PlaylistMaker mpv script

The Go application embeds its canonical mpv tracking script from
`src/PlaylistMaker.Charm/internal/mpvscript/playlistmaker-history.lua`. It
installs that exact version under the configured data directory and passes the
path explicitly to mpv for every playback.
