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
- `/` searches; `c`, `s`, and `f` open categories, sorting, and filters; `p` opens playback and mixes.
- `q` opens the queue; `Shift+J`/`Shift+K` reorder; `Delete` removes; `C` clears.
- `m` on a video row opens the catalogue picker to change its track link; Enter saves and Esc cancels.
- `u` updates video mappings; `U` updates Spotify links; `R` refreshes local history; uppercase `L` opens Last.fm sync and matching; `?` opens help; `Ctrl+Q` quits.

## Playback tracking and history

PlaylistMaker prefers Spotify when a track has a Spotify URI. Otherwise, a configured local FLAC can be opened in foobar2000. Without either source, playback is rejected unless `--allow-untracked-playback` is supplied. The application installs its bundled mpv Lua script automatically; see [`mpv-scripts/README.md`](mpv-scripts/README.md) for details.

Broken local audio links produce a library warning and block affected playback until repaired, even if Spotify is linked. Press `u` to rescan and review missing or renamed audio links, then choose replacement audio. Relinking an unclaimed file preserves the existing track ID and history. The catalogue picker loads candidates once per opening and filters in memory after a 150 ms typing pause. `Ctrl+U` clears its search. It shows release date, album, and the selected source; matching titles list earliest known releases first. Intentionally absent local links are allowed. See [Spotify setup](src/PlaylistMaker.Charm/SPOTIFY_SETUP.md) for authentication, playback completion, and timeout behaviour.

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

The mpv loop regression tests run a synthetic silent clip and skip when mpv is unavailable. A passing test suite with those tests skipped does not verify mpv event behaviour.

## Documentation

- [Architecture](ARCHITECTURE.md): catalogue identity, playback process ownership, and the two history stores.
- [Category presets](src/PlaylistMaker.Charm/README.md#category-presets): configuration semantics and source of category names.
- [Spotify setup](src/PlaylistMaker.Charm/SPOTIFY_SETUP.md) and [Last.fm history](src/PlaylistMaker.Charm/LASTFM_SETUP.md): integration guides.
- [Portable fixtures](testdata/library/README.md): loader test data.

Ignored `notes/` and `.ignore/` hold local plans, feedback, and handovers, including material from the former C# implementation. They are retained as historical context, not maintained documentation.
