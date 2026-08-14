# PlaylistMaker Charm

This is the additive Bubble Tea frontend. It communicates with `PlaylistMaker.Bridge`, which reuses the existing C# library, configuration, playback, and history services.

Run from the repository root:

```powershell
.\scripts\run-charm.ps1
```

`Ctrl+Enter` launches the real queued videos through the existing mpv/foobar workflow. The queue is cleared after a successful launch and retained when launch fails.

Playback history is disabled by default in the Charm development launcher so test sessions do not pollute the permanent log. Pass `-EnableHistory` when history should be recorded:

```powershell
.\scripts\run-charm.ps1 -EnableHistory
```

The launcher rebuilds the Go frontend or .NET bridge only when its source files are newer than the cached executable.

The isolated synthetic performance harness remains available through `scripts/run-charm-spike.ps1`; it never accesses local application data or players.

The footer displays rolling p95 timings for the application's update and view work. These numbers exclude the terminal's final output flush, so the practical test is whether rapid held-key navigation and typing visibly keep up in Windows Terminal.

Key flows:

- `j`/`k` or arrows navigate.
- `Ctrl+U`/`Ctrl+D` move one visible page; `gg`/`G` jump to the top or bottom.
- `h`/`l` collapse and expand.
- `Space` queues in navigation mode.
- `/` enters search mode; `Space` then inserts a space.
- `c`, `s`, and `q` open categories, sorting, and queue overlays.
- `Esc` closes a mode. Only `Ctrl+Q` quits.
