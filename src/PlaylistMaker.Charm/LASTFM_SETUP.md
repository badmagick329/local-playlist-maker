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

After the download, the step `Caching Spotify metadata for linked catalogue tracks` fetches Spotify artist, title and ISRC for catalogue tracks that already have a Spotify link, so matching can use them as aliases. It never links a scrobble or a track to Spotify; `U` does that.

## Check what a sync added

When a sync finishes, the Last.fm screen opens its report. The report covers every cached scrobble played after the latest one cached before the sync; the first sync covers the whole history, and `Rebuild full history` reports only scrobbles after the previous latest one. It shows how many scrobbles were added and how many belong to songs with a Spotify link, then lists the songs whose added scrobbles have none, each with its count of added scrobbles:

- `Unresolved`: no match or no-match decision yet. Export them for review.
- `No match in catalogue`: a no-match decision says the song is not in the library.
- `Matched without a Spotify link`: matched to a track whose Spotify link is ignored or not set yet.

`j/k` scroll the report, `Enter` or `Esc` returns to the actions, and `L` closes the screen. `Last sync report` reopens the newest report under the current matches, so it shrinks as decisions are imported.

Each sync also writes its report, as matched at sync time, to its own file under `lastfm-syncs` in the data directory, named by the sync time in UTC, such as `20261008T155447Z.json`. The newest file supplies `Last successful sync` and the start of `since last sync`; before the first recorded sync, both are unknown. The report is saved even when the Spotify step fails or is cancelled. If the report file cannot be written, the status says so; the scrobbles and matches are still saved.

If `lastfm-scrobbles.jsonl`, `lastfm-matches.json`, `spotify-track-cache.json` or the newest sync report fails to load, the Last.fm screen shows a cache error. Until you fix or remove that file and restart, sync, review export, decision import and reset refuse to run, so a partial cache can never overwrite the saved matches.

The integration is read-only. It never submits scrobbles and does not replace the Last.fm scrobbler. Last.fm events stay separate from `play-history.jsonl`.

Scrobbles with the same normalized artist and title form one identity, and PlaylistMaker matches it to a catalogue track automatically when the names point to one song. A track fits when its artist and title, or its linked Spotify artist and title, normalize to the identity. Only when no track fits does it compare again with spaces removed, so `U-KISS` finds `UKISS`. Several tracks that fit count as one song when they share a Spotify link or ISRC, such as a single and the album that repeats it. Their plays all go to one track: the one with the most videos, then the earliest release, then the lowest track ID. Any other set of fitting tracks leaves the identity unresolved for review.

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

Order defaults to Mix order for generated mixes. Explicitly choosing Shuffle changes the playback order. Manual queues offer Queue order or Shuffle, an optional One video per track setting, and Play first N, where zero means all. With Shuffle, the whole selected queue is shuffled before the manual limit is applied. Version choice resolves multiple queued performances when One video per track is enabled.

Repeat each applies after selection, limiting and ordering. A count of 20 with repeat 2 produces up to 40 plays, with each pair consecutive. Generated queues contain concrete videos; changing Performances later does not replace them. The `o` shortcut plays the existing queue or highlighted media without generating another mix.

## Review unresolved matches

On the `Export unresolved` row, `h/l` choose which unresolved identities to export:

- `all`: every unresolved identity.
- `since last sync`: identities with a scrobble the newest recorded sync added.
- `range`: identities with a scrobble in a typed UTC date range. It accepts `YYYY`, `YYYY-MM`, `YYYY-MM-DD` or `START..END`; Backspace deletes a character and `Ctrl+U` clears the range.

A limited export keeps each case's whole play history, and `review.json` names the period. Each export replaces the previous one's files under `lastfm-review` in the data directory:

- `instructions.md` tells an external matching agent what to do.
- `review.json` contains unresolved identities, ranked catalogue candidates, and catalogue evidence.
- The agent writes `decisions.json`.

Give the directory to the external agent, then choose `Import agent decisions`. PlaylistMaker accepts only the exported case IDs and current catalogue track IDs. An old export ID rejects the document. Invalid rows are skipped; valid match and no-match decisions are saved together. The catalogue may change between export and import: a decision for a case that was resolved in the meantime, or by an earlier import of the same file, is skipped and counted as already resolved.

A match lasts while its track exists, so an automatic match keeps its track when a later duplicate would win the canonical choice. A no-match lasts until the catalogue gains a track that fits the identity, with or without spaces; the identity is then matched again, and becomes unresolved if the fitting tracks are not one song. Other catalogue changes leave no-match decisions in place.

`Reset agent decisions` removes imported decisions and runs exact matching again. It requires a second `Enter` press.
