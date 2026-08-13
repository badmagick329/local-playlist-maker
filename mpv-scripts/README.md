# PlaylistMaker mpv script

`playlistmaker-history.lua` is the version-controlled source for PlaylistMaker's playback logger.

The new TUI embeds this file and checks the installed copy. When playback history is enabled, use **Setup → Install/update mpv history script** (or accept the startup prompt) to install it explicitly.

## Install on Windows

Copy it to mpv's user scripts directory, creating the directory if needed:

```powershell
$mpvScripts = Join-Path $env:APPDATA 'mpv\scripts'
New-Item -ItemType Directory -Force -Path $mpvScripts
Copy-Item .\playlistmaker-history.lua (Join-Path $mpvScripts 'playlistmaker-history.lua') -Force
```

Restart mpv after copying. The script stays inactive unless PlaylistMaker starts mpv with its per-session options, so it does not log videos opened normally in mpv.

The path is derived from `%APPDATA%`; no machine-specific user directory should be committed to documentation or configuration examples.
