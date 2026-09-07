# Architecture

PlaylistMaker is a Go application under `src/PlaylistMaker.Charm`. The directory name reflects its history; the current launcher and build use Go. The root [README](README.md) covers running and checking it.

## Catalogue and identity

`catalog` owns stable track IDs. A track can have local audio, a Spotify URI, and multiple video variants. Relinking media should preserve the track ID because queues, playback history, and Last.fm matches refer to it. `pathid` compares media paths case-insensitively even in tests on case-sensitive systems.

`native.Loader` builds the UI library from the catalogue and cached FLAC tags. Loading does not discover new video files. `updater` scans configured folders and presents mappings for confirmation; `spotifylink` manages Spotify matches separately. Cached tags are not proof that the linked file still exists. The loader checks local paths and playback rechecks them before launching.

## Playback processes

`playback` creates a manifest and starts mpv plus a hidden instance of the same executable with `--track-session`. The embedded Lua script writes session events and local history. `tracksession.Runner` consumes those events, and its queue controls Spotify or the configured local player.

The helper outlives the UI and mpv when a qualifying Spotify play still needs to finish. The shared tracking lock serializes helpers so a later video cannot interrupt an earlier song's remaining playback. Each repeat is a distinct queue occurrence; video position or song identity alone cannot identify a play. Startup failure, completion, cancellation, and terminal tracking failure must release ownership. A live PID alone does not prove playback is progressing.

Spotify request acceptance is separate from observed playback. Connect can report a different release ID, title, or duration from the requested track. Confirmation and completion waits are bounded; a failed attempt is not evidence of a saved scrobble. See [Spotify setup](src/PlaylistMaker.Charm/SPOTIFY_SETUP.md) for timing, fallback, and error behaviour.

## Two independent histories

`history` records local video outcomes in `play-history.jsonl`. Repeated plays are distinguished by session, entry, and play ID. `lastfm` imports completed scrobbles into its own cache and matches them to catalogue track IDs. Merging these stores would double-count playback and confuse video completion with Last.fm acceptance. Spotify's integration or the local player's scrobbler submits scrobbles; PlaylistMaker never does.

The [Last.fm guide](src/PlaylistMaker.Charm/LASTFM_SETUP.md) describes checkpointed imports and validated external-agent decisions. Generated review instructions belong with each export, not in repository agent instructions.
