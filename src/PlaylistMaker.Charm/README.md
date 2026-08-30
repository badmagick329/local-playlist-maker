# PlaylistMaker Charm

This directory contains the only PlaylistMaker application: a native Go terminal UI built with Bubble Tea/Charm. The root [`README.md`](../../README.md) is the canonical setup and usage guide.

From the repository root, copy `sample_config.yaml` to the ignored `config.yaml`, configure the local paths and player commands, then launch:

```powershell
.\scripts\run-charm.ps1
```

From this directory, the application can be checked directly with `--config`:

```powershell
go run ./cmd/playlistmaker-charm --config ../../config.yaml
```

The main controls are `j`/`k` or arrows to move, `Space` to queue, `o` to play, `/` to search, `c` for categories, `s` for sorting, `f` for filters, `p` for playback options, `q` for the queue, `u` for video mappings, `U` for Spotify links, `R` for history refresh, uppercase `L` for Last.fm history, and `?` for help.

Last.fm setup and the external-agent review workflow are documented in [`LASTFM_SETUP.md`](LASTFM_SETUP.md).

Playback prefers Spotify when a track has a Spotify URI, then uses a configured local FLAC player. `--allow-untracked-playback` permits mpv playback when neither source is available. `--disable-history` disables new playback-history sessions without disabling playback tracking. The bundled Lua logger is documented in [`mpv-scripts/README.md`](../../mpv-scripts/README.md).

For old mapping data, set `mediaCatalogFile` in the local config and run:

```powershell
.\scripts\run-charm.ps1 --migrate-mapping .\path\to\old-video-audio-map.json
```

This migrates the old data format and updates resolvable history entries; no other executable is required. `--check` loads the catalogue read-only without launching players, creating sessions, or changing history.

Run the Go checks from this directory:

```powershell
go test ./...
go vet ./...
go build ./cmd/playlistmaker-charm
```

## Category presets

`categoryPresets` is optional and supports up to five ordered presets. In the Categories view, list positions map to keys `0` through `4`. A preset with `include` enables only the listed categories; a preset with `exclude` enables every known category except those listed. Each preset must use exactly one of these fields.

The exact category names are: `Music Video`, `Band Live`, `Performance`,
`Choreography`, `Relay`, `Be Original`, `Fancam`, `Concert`, `Music Show`,
`Remix`, and `Live Audio`.
