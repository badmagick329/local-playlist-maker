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

Press `p` and choose `Listening period` in the Mix row. The primary period accepts `YYYY`, `YYYY-MM`, `YYYY-MM-DD`, or `START..END`. Leave it blank to use all cached history. Add a secondary period and percentage to blend two periods.

Date ranges include both endpoints and apply to scrobble timestamps. The builder only uses tracks in the current filtered view and videos allowed by the active category and date filters. It uses the current version-choice setting when it adds each track to the queue.

## Playback presets and controls

The `p` panel contains manual queue playback, Familiar songs/unseen performances, Balanced rotation, Forgotten favourites, Current obsessions, and listening-period mixes. Move with `j/k`, change choices with `h/l`, and use digits or Backspace for numbers. Period dates accept typed dates and ranges; `Ctrl+U` clears a date field. Press `o` or select Play to replace the queue with a generated mix and launch it, or press `a` or select Add to queue to append without repeating tracks already queued. Save settings remembers choices for this application session without building a queue. Escape cancels edits.

Familiar songs/unseen performances requires at least three cached Last.fm plays and an eligible video with no counted local play. It never falls back to a watched video. Song selection is weighted by Last.fm play count.

Balanced rotation targets 40% recent favourites, 40% older favourites, and 20% rarely played tracks. Recent favourites have at least two scrobbles in the last 30 days; older favourites have at least three overall and fewer than two in that window. The remaining tracks form the rare pool, including tracks without matched scrobbles. Favourites are weighted by recent or total plays respectively; rare tracks have equal weight. Empty pools contribute their slots to the other pools. Selection avoids consecutive artists within a pool when possible.

Forgotten favourites requires at least 10 Last.fm plays across five distinct UTC dates, with no scrobbles or counted local plays in the last six calendar months. Any local attempt in the last 30 days, including a skip on another performance of the song, excludes it temporarily. Queued entries that never started do not count as attempts. Eligible songs are sampled without replacement with logarithmic familiarity weights, so an old heavy rotation does not overwhelm the mix. The cooldown expires automatically; skips never create a permanent dislike.

Current obsessions uses the latest cached scrobble date across the whole history, shown in the panel as History through. Its recent window is the 14 UTC calendar days ending on that date, compared with the preceding 30 days. Songs must have plays on at least two dates in the recent window and a positive increase in daily listening rate. Mix order ranks that increase, rather than lifetime popularity or percentage growth from a zero baseline. History outside the active library filters still determines the reference date. Missing history or no qualifying songs leaves the existing queue untouched.

Generated mixes select one video per track. Performances chooses that video, except the familiar/unseen preset fixes it to Unseen only. Track count limits the generated selection; fewer tracks are returned when the filters or history leave too few candidates. All mixes respect the current filtered library and eligible video categories and dates.

Order defaults to Mix order for generated mixes. Explicitly choosing Shuffle changes the playback order. Manual queues offer Queue order or Shuffle, an optional One video per track setting, and Play first N, where zero means all. The manual limit takes the first selected entries before shuffling. Version choice resolves multiple queued performances when One video per track is enabled.

Repeat each applies after selection, limiting and ordering. A count of 20 with repeat 2 produces up to 40 plays, with each pair consecutive. Generated queues contain concrete videos; changing Performances later does not replace them. The `o` shortcut plays the existing queue or highlighted media without generating another mix.

## Review unresolved matches

`Export unresolved matches` writes these files under `lastfm-review` in the data directory:

- `instructions.md` tells an external matching agent what to do.
- `review.json` contains unresolved identities, ranked catalogue candidates, and catalogue evidence.
- The agent writes `decisions.json`.

Give the directory to the external agent, then choose `Import agent decisions`. PlaylistMaker accepts only the exported case IDs and current catalogue track IDs. A changed catalogue or old export ID rejects the document. Invalid rows are skipped; valid match and no-match decisions are saved together.

`Reset agent decisions` removes imported decisions and runs exact matching again. It requires a second `Enter` press.
