# PlaylistMaker Charm

Charm plays audible video through mpv. Spotify Connect is the preferred tracking player, with foobar2000 as the local fallback.

Copy `sample_config.yaml` to the ignored `config.yaml`, then run:

```powershell
.\scripts\run-charm.ps1
```

Tracks and video relationships live in the app-managed media catalogue. Press `u` to link videos to catalogue tracks. Press `U` to authenticate with Spotify and review missing Spotify links. Spotify is contacted only for authentication, link updates, playback preflight, or recovery of a previous session.

See [Spotify setup](SPOTIFY_SETUP.md) for the short dashboard, configuration, login, and linking guide.

Playback prefers Spotify when a track has a Spotify URI. If Spotify is unavailable, a track with an existing local FLAC uses foobar2000. An item with neither source is rejected before mpv starts unless the launcher includes:

```powershell
.\scripts\run-charm.ps1 -AllowUntrackedPlayback
```

History can be disabled without disabling tracking:

```powershell
.\scripts\run-charm.ps1 -DisableHistory
```

For the one-time mapping migration, first change the local config from `mappingFile` to `mediaCatalogFile`, then run the built executable with `--migrate-mapping` and the old mapping path. PlaylistMaker creates untouched `.pre-catalogue-backup` copies of the mapping and history files, writes stable track IDs into the catalogue and resolvable history events, and reports unresolved history entries.

`--check` loads the catalogue without launching players, creating sessions, or changing history.

Run the Go checks from `src/PlaylistMaker.Charm`:

```powershell
go test ./...
go vet ./...
```

## Category presets

`categoryPresets` is optional and supports up to five ordered presets. In the
Categories view, list positions map to keys `0` through `4`. A preset with
`include` enables only the listed categories; a preset with `exclude` enables
every known category except those listed. Each preset must use exactly one of
these fields.

The exact category names are: `Music Video`, `Band Live`, `Performance`,
`Choreography`, `Relay`, `Be Original`, `Fancam`, `Concert`, `Music Show`,
`Remix`, and `Live Audio`.

The development launcher reads `config.yaml` from the repository root.
Direct binary launches read `config.yaml` beside the executable unless
`--config` supplies another path. The local `config.yaml` is ignored; use
`sample_config.yaml` for the documented configuration shape.
