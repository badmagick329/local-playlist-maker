# PlaylistMaker

PlaylistMaker launches a selected music-video playlist in mpv and the matching FLAC files in an audio player so the listens can still be scrobbled. Video-to-audio relationships come from the JSON mapping files configured in `config.yaml`.

## Applications

Two interfaces currently live side by side:

- `PlaylistMaker.Console` is the original fzf interface and remains the legacy fallback.
- `PlaylistMaker.Tui` is the full-screen, track-centred interface built with Terminal.Gui.

Both use the same configuration, mappings, cache, playback history, and mpv logger.

## Setup

1. Install the .NET 10 SDK. The legacy application remains on .NET 9, while the TUI requires .NET 10.
2. Copy `sample_config.yaml` to the ignored local file `config.yaml` and fill in the paths for the machine.
3. Build everything:

```powershell
dotnet build PlaylistMaker.sln
```

Run the new interface from the repository root:

```powershell
.\scripts\run-tui.ps1
```

The legacy interface remains available separately:

```powershell
dotnet run --project .\src\PlaylistMaker.Console\PlaylistMaker.csproj
```

## TUI workflow

- The TUI starts in navigation mode. Press `/` to enter search mode, type a fuzzy query normally, then press `Enter` or `Esc` to return to navigation mode.
- Use arrows, `J`/`K`, or `Ctrl+J`/`Ctrl+K` to navigate. Use `Enter` or `L` to expand a track, `H` to collapse it, and `Space` to add or remove its selected video from the queue.
- Press `C` to focus categories, use `J`/`K` and `Space` to change them, then press `C` or `Esc` to return to tracks.
- Use `Tab` to move between filters, results, details, and queue panes.
- Use `Delete` and `Alt+Up`/`Alt+Down` to edit the queue.
- Press `Ctrl+Enter` to launch the queue, `Ctrl+O` for playback options, `Ctrl+R` to reload mappings/history, and `F1` for help.

The initial filter shows official music videos only. The Filters menu enables performances, music shows, remixes, live videos, and date ranges. On narrow terminals, the same filters, details, and queue remain available as menu dialogs.

## Playback history

When playback history is enabled, the TUI shows play/skip totals, last-played time, and recent outcomes for tracks and individual video versions. Existing JSONL records are read without being rewritten; percentages are clamped for display and old stopped events at 90% or above are treated as completed.

The application bundles its mpv Lua logger. If mpv's installed copy is missing or outdated, the TUI offers an explicit Install/Update action. It never overwrites the script silently. Manual installation instructions are in [`mpv-scripts/README.md`](mpv-scripts/README.md).

Raw history remains in `data/play-history.jsonl`. PlaylistMaker-launched playback normally produces a `started` event followed by one terminal event for the same entry; these are one playback lifecycle, not duplicate plays.

## Tests

```powershell
dotnet test PlaylistMaker.sln
```

For a read-only startup check against the active mappings and history:

```powershell
dotnet run --project .\src\PlaylistMaker.Tui\PlaylistMaker.Tui.csproj -- --check
```
