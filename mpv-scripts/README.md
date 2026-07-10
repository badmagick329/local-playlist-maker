# PlaylistMaker mpv script

`playlistmaker-history.lua` is the version-controlled source for PlaylistMaker's playback logger.

## Install on Windows

Copy it to mpv's user scripts directory, creating the directory if needed:

```powershell
$mpvScripts = Join-Path $env:APPDATA 'mpv\scripts'
New-Item -ItemType Directory -Force -Path $mpvScripts
Copy-Item .\playlistmaker-history.lua (Join-Path $mpvScripts 'playlistmaker-history.lua') -Force
```

For this machine, the destination is:

`C:\Users\uzair\AppData\Roaming\mpv\scripts\playlistmaker-history.lua`

Restart mpv after copying. The script stays inactive unless PlaylistMaker starts mpv with its per-session options, so it does not log videos opened normally in mpv.
