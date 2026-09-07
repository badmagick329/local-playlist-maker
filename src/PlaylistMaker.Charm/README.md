# PlaylistMaker Charm

See the root [README](../../README.md) for setup, controls, migration, and Go checks; [architecture](../../ARCHITECTURE.md) for catalogue and playback boundaries.

Integration guides: [Spotify](SPOTIFY_SETUP.md) and [Last.fm](LASTFM_SETUP.md).

## Category presets

`categoryPresets` supports up to five ordered presets. Their positions map to keys `0` through `4` in the Categories view. Each preset uses either `include` to enable only the listed categories, or `exclude` to enable all other categories.

Category names come from `internal/library/library.go`. Configuration examples are in [`sample_config.yaml`](../../sample_config.yaml).
