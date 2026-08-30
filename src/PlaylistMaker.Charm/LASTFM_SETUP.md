# Last.fm history

## Configure access

Create a Last.fm API account at <https://www.last.fm/api/account/create>. Add the username whose listening history you want to import and the API key to `config.yaml`:

```yaml
lastfmUsername: "your-lastfm-username"
lastfmApiKey: "your-api-key"
```

Both fields are optional. Leave both blank to disable network access. Cached history, matching, and period mixes still work while offline.

## Import listening history

Press uppercase `L` to open the Last.fm screen. `Sync new plays` downloads the full history when no cache exists. Later runs request only scrobbles at or after the latest cached timestamp. `Rebuild full history` downloads every page again.

PlaylistMaker stores completed scrobbles in `lastfm-scrobbles.jsonl` under the configured data directory. It skips Last.fm's currently playing entry. Each downloaded page is checkpointed. A failed or cancelled download leaves the existing cache untouched, and `Sync new plays` or `Rebuild full history` resumes the compatible checkpoint instead of starting again.

The integration is read-only. It never submits scrobbles and does not replace the Last.fm scrobbler. Last.fm events stay separate from `play-history.jsonl`.

## Build a period mix

Choose `Build period mix` from the Last.fm screen. The primary period accepts `YYYY`, `YYYY-MM`, `YYYY-MM-DD`, or `START..END`. Leave it blank to use all cached history. Add a secondary period and percentage to blend two periods.

Date ranges include both endpoints and apply to scrobble timestamps. The builder only uses tracks in the current filtered view and videos allowed by the active category and date filters. It uses the current version-choice setting when it adds each track to the queue.

## Review unresolved matches

`Export unresolved matches` writes these files under `lastfm-review` in the data directory:

- `instructions.md` tells an external matching agent what to do.
- `review.json` contains unresolved identities, ranked catalogue candidates, and catalogue evidence.
- The agent writes `decisions.json`.

Give the directory to the external agent, then choose `Import agent decisions`. PlaylistMaker accepts only the exported case IDs and current catalogue track IDs. A changed catalogue or old export ID rejects the document. Invalid rows are skipped; valid match and no-match decisions are saved together.

`Reset agent decisions` removes imported decisions and runs exact matching again. It requires a second `Enter` press.
