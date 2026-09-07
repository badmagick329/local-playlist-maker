# Repository guidance

- For catalogue, history, or playback changes, read [ARCHITECTURE.md](ARCHITECTURE.md) for identity and process boundaries.
- Keep Last.fm imports separate from local playback history. PlaylistMaker does not submit scrobbles.
- Edit the embedded mpv script under `src/PlaylistMaker.Charm/internal/mpvscript`; installed copies in the data directory are generated.
- Keep machine configuration, media catalogues, credentials, and listening history out of commits. Use the portable fixtures for tests.
- Use [README.md](README.md) for build and checks. For Spotify or Last.fm integration work, follow its linked setup guides. Ignored `notes/` and `.ignore/` contain historical task material, not current specifications.
