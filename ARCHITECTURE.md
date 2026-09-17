# Architecture

PlaylistMaker is a Go application under `src/PlaylistMaker.Charm`. The directory name reflects its history; the current launcher and build use Go. The root [README](README.md) covers running and checking it.

## Catalogue and identity

`catalog` owns stable track IDs. A track can have local audio, a Spotify URI, and multiple video variants. Relinking media should preserve the track ID because queues, playback history, and Last.fm matches refer to it. `pathid` compares media paths case-insensitively even in tests on case-sensitive systems.

`native.Loader` builds the UI library from the catalogue and cached FLAC tags. Loading does not discover new video files. `updater` scans configured folders and presents mappings for confirmation; `spotifylink` manages Spotify matches separately. Cached tags are not proof that the linked file still exists. The loader checks local paths and playback rechecks them before launching.

## Playback processes

`playback` creates a manifest and starts mpv plus a hidden instance of the same executable with `--track-session`. The embedded Lua script writes session events and local history. `tracksession.Runner` consumes those events, and its queue controls Spotify or the configured local player.

The helper outlives the UI and mpv when a qualifying Spotify play still needs to finish. The shared tracking lock serializes helpers so a later video cannot interrupt an earlier song's remaining playback. Each repeat is a distinct queue occurrence; video position or song identity alone cannot identify a play. Startup failure, completion and cancellation release ownership. Tracking failures suspend the queue instead of discarding it. A confirmed deliberate Spotify pause releases shared ownership after one minute; a retry or changed ceiling reacquires it. Kernel-held lease sidecars serialize shared ownership and prevent duplicate helpers for one manifest. They unlock on process death; ownership JSON is informational. Persistent sidecars avoid stale-PID unlink races. A live PID alone does not prove playback is progressing.

The Lua/helper protocol separates user intent from the actual mpv pause property. Session and occurrence IDs, increasing intent sequences and status revisions correlate controls and acknowledgements. System-imposed pause changes are suppressed as user input. The status file carries a one-second event-loop heartbeat; mpv enforces a persistent hold if it stops changing for 15 seconds. Space/Play requests recovery, while Ctrl+Shift+U explicitly opts out of tracking. Windows notifications supplement the hold. The TUI prioritizes active holds across sessions.

Private checkpoints preserve active/pending occurrences, completed-video qualification, current video identity, event offset, pause ceiling and Spotify observation evidence. External commands are preceded by a checkpoint; an interrupted transaction is ambiguous rather than permission to replay. A restarted helper can adopt a confirmed still-paused Spotify occurrence only after checking its durable ownership record and non-regressed position. End/missing/uncertain states stay blocked. Failed detached checkpoints survive stale-session cleanup for review.

Spotify recovery resumes the owned occurrence without a URI or position command and verifies advancing progress. A freshly identified early pause can recover before the first playing sample; a fresh stopped endpoint after a short-tail resume can verify completion between polls. Device takeovers withhold command permission independently of retained near-end evidence. Deliberate pause ceilings use normalized media position, never wall-clock watched duration. Earlier qualified plays drain in order during a pause. A completed tracking occurrence remains completed even while its video continues. Spotify playback verification does not establish Last.fm acceptance.

Spotify request acceptance is separate from observed playback. Connect can report a different release ID, title, or duration from the requested track. Confirmation and completion waits are bounded; a failed attempt is not evidence of a saved scrobble. See [Spotify setup](src/PlaylistMaker.Charm/SPOTIFY_SETUP.md) for timing, fallback, and error behaviour.

## Two independent histories

`history` records local video outcomes in `play-history.jsonl`. Repeated plays are distinguished by session, entry, and play ID. `lastfm` imports completed scrobbles into its own cache and matches them to catalogue track IDs. Merging these stores would double-count playback and confuse video completion with Last.fm acceptance. Spotify's integration or the local player's scrobbler submits scrobbles; PlaylistMaker never does.

The [Last.fm guide](src/PlaylistMaker.Charm/LASTFM_SETUP.md) describes checkpointed imports and validated external-agent decisions. Generated review instructions belong with each export, not in repository agent instructions.
