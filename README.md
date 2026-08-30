# PlaylistMaker

PlaylistMaker is a self-contained Go application with a Bubble Tea/Charm terminal interface. It selects music-video playlists for mpv and matching audio for Spotify Connect or foobar2000 so playback can be tracked.

## Setup

Install Go 1.25.0 or newer. Copy `sample_config.yaml` to the ignored local file `config.yaml`, then set the video, audio, data, player, and optional Spotify paths and commands for the machine.

The normal launcher builds a cached executable and passes the repository configuration:

```powershell
.\scripts\run-charm.ps1
```

The launcher also accepts `-DisableHistory`, `-AllowUntrackedPlayback`, and additional application arguments. To run directly, build from `src/PlaylistMaker.Charm` or use the Makefile:

```powershell
make build
make run ARGS="--check"
```

## Controls

The main controls are:

- `j`/`k` or arrows move; `h`/`l` or `Enter` collapse or expand a track.
- `Space` queues the current video; `o` plays the queue or highlighted media.
- `/` searches; `c`, `s`, and `f` open categories, sorting, and filters; `p` opens playback options.
- `q` opens the queue; `Shift+J`/`Shift+K` reorder; `Delete` removes; `C` clears.
- `u` updates video mappings; `U` updates Spotify links; `R` refreshes local history; uppercase `L` opens Last.fm history and period mixes; `?` opens help; `Ctrl+Q` quits.

## Playback tracking and history

PlaylistMaker prefers Spotify when a track has a Spotify URI. Otherwise, a configured local FLAC can be opened in foobar2000. Without either source, playback is rejected unless `--allow-untracked-playback` is supplied. The application installs its bundled mpv Lua script automatically; see [`mpv-scripts/README.md`](mpv-scripts/README.md) for details.

Playback history is enabled by `playbackHistoryEnabled` and can be disabled for a run with `--disable-history`. History is read from `data/play-history.jsonl`; PlaylistMaker-launched playback records a `started` event and a terminal event for the same lifecycle.

## Last.fm history

Set `lastfmUsername` and `lastfmApiKey` to import completed scrobbles. PlaylistMaker caches the history for offline use, matches exact artist and title identities to catalogue tracks, and builds queues from one or two listening periods. It never writes to Last.fm and keeps these events separate from local playback history.

Unresolved identities can be exported for an external matching agent and imported through the Last.fm screen. See [`src/PlaylistMaker.Charm/LASTFM_SETUP.md`](src/PlaylistMaker.Charm/LASTFM_SETUP.md) for setup, sync behavior, period dates, and the review workflow.

## Legacy data migration

`--migrate-mapping` converts an old video-to-audio mapping file into the current media catalogue and updates resolvable history entries. This supports old data formats; it does not require another executable.

```powershell
.\scripts\run-charm.ps1 --migrate-mapping .\path\to\old-video-audio-map.json
```

Set `mediaCatalogFile` in `config.yaml` before running the migration. The command reports migrated tracks, videos, updated history events, and unresolved history entries.

## Read-only check

Load the configured catalogue and history without launching players or changing data:

```powershell
.\scripts\run-charm.ps1 --check
```

## Go checks

From `src/PlaylistMaker.Charm`:

```powershell
go test ./...
go vet ./...
go build ./cmd/playlistmaker-charm
```

The optional `categoryPresets` configuration supports up to five ordered presets. Each preset uses either `include` or `exclude`, and keys `0` through `4` select them in the Categories view. The exact category names are documented in [`src/PlaylistMaker.Charm/README.md`](src/PlaylistMaker.Charm/README.md).
