# Spotify setup

PlaylistMaker uses Spotify Connect as its preferred tracking player. mpv remains the audible video player. PlaylistMaker does not change Spotify volume, so the tracking device has to be silent on its own: librespot started with `--initial-volume 0` is, and the Spotify desktop app needs muting by hand.

## What you need

- A Spotify Premium account. Spotify requires Premium for Web API playback control.
- A Spotify Connect device to track on: the Spotify desktop app (recommended), librespot, or another device.
- Your own app in the [Spotify Developer Dashboard](https://developer.spotify.com/dashboard).

## 1. Create the Spotify app

1. Open the [Spotify Developer Dashboard](https://developer.spotify.com/dashboard) and sign in with your Spotify account.
2. Choose **Create app**.
3. Enter any name and description, then select **Web API**.
4. Add this redirect URI exactly:

   ```text
   http://127.0.0.1:43827/callback
   ```

   Use `127.0.0.1`, not `localhost`. Spotify requires the redirect URI used during login to match the registered value.
5. Save the app and copy its **Client ID**.

PlaylistMaker uses Spotify's PKCE login flow, so it does not need the Client Secret. See Spotify's [PKCE guide](https://developer.spotify.com/documentation/web-api/tutorials/code-pkce-flow) and [redirect URI rules](https://developer.spotify.com/documentation/web-api/concepts/redirect_uri).

## 2. Set up the tracking device

PlaylistMaker plays each song on one Spotify Connect device, the one `spotifyDeviceName` names. The recommended device is the Spotify desktop app. [librespot](https://github.com/librespot-org/librespot), an open-source Spotify Connect receiver written in Rust, is the alternative, but librespot 0.8.0 currently plays nothing: Spotify refuses its audio keys and it logs `error audio key 0 1` for every track ([librespot#1649](https://github.com/librespot-org/librespot/issues/1649)).

### Spotify desktop app (recommended)

Open the Spotify desktop app on this computer and set `spotifyDeviceName` to the name shown for it in Spotify's device picker. Mute it if you want silent tracking, and leave it running while using PlaylistMaker.

### librespot (alternative)

1. Install Rust with [rustup](https://rustup.rs/).
2. Install librespot:

   ```powershell
   cargo install librespot --locked
   ```

   Keep `--locked`. Without it, the build fails in librespot-core's build script on a vergen-lib version mismatch.
3. Start librespot for the first time with a one-time browser sign-in:

   ```powershell
   librespot --name PlaylistMaker --initial-volume 0 --enable-oauth --cache "$env:LOCALAPPDATA\librespot"
   ```

   Sign in to Spotify in the browser that opens. librespot saves the credentials in the cache folder.
4. Start it the same way from then on, without `--enable-oauth`; the cached credentials sign it in:

   ```powershell
   librespot --name PlaylistMaker --initial-volume 0 --cache "$env:LOCALAPPDATA\librespot"
   ```

Set `spotifyDeviceName` to the `--name` value, here `PlaylistMaker`. Spotify's Web API lists the device as `PlaylistMaker (Speaker)`. Keep the librespot terminal open while tracking; closing it removes the device. `--initial-volume 0` keeps librespot silent, so there is nothing to mute. A freshly started librespot needs no priming: PlaylistMaker can start the first play on it directly. Plays on librespot scrobble through Spotify's Last.fm integration like plays on any other Spotify device.

## 3. Configure PlaylistMaker

Open your ignored local `config.yaml` and fill in these values:

```yaml
spotifyClientId: "your-client-id"
spotifyDeviceName: "PlaylistMaker"
spotifyRedirectUri: "http://127.0.0.1:43827/callback"
```

The device name must match one available Spotify Connect device. Capitalization does not matter, but the name must otherwise match and must be unique. Volume-control support is not required. Spotify's Web API device name may differ from the label shown in the Spotify Connect picker; if the match fails, PlaylistMaker lists the available API names and types.

## 4. Sign in and link tracks

1. Start PlaylistMaker:

   ```powershell
   .\scripts\run-charm.ps1
   ```

2. Press `U` to open **Update Spotify links**.
3. Your browser will open. Sign in to Spotify and approve the requested playback permissions.
4. Return to PlaylistMaker after the browser says login is complete.

PlaylistMaker stores the token at `dataDirectory/spotify-auth.json`. This file is local and ignored by Git.

The update view scans catalogue tracks without Spotify links. Unique high-confidence matches save automatically. For anything needing review:

- `h` / `l` changes the suggested candidate.
- `Enter` confirms it.
- `/` searches Spotify or accepts a pasted Spotify track URL.
- `s` skips it for this scan.
- `i` ignores it in future scans.
- `U` or `Esc` closes the view and reloads the catalogue.

## 5. Test playback

Keep the configured Spotify device running, queue a linked video, and play it. PlaylistMaker should:

1. Leave the device's volume as it is (silent for librespot at volume 0 or a muted desktop app).
2. Start the linked Spotify track from the beginning, inside its album.
3. Play the video's audible sound through mpv.
4. Let the Spotify song finish when its video reaches the end, then start the next queued tracking play. Videos continue while Spotify catches up, and completed plays finish even after mpv closes.

Each video play, including a loop of the same video, has its own tracking queue entry. Skipping a video removes that occurrence if it is queued, or pauses it if Spotify is playing it. It does not interrupt an earlier video's song that is still finishing. PlaylistMaker does not change or restore Spotify volume.

Each song starts inside its album, at that song. Spotify accepts a request to play a single track on its own and then never starts it, so PlaylistMaker always sends the album as the playback context. A track whose Spotify metadata has no album fails to start. Because the album is the context, Spotify moves on to the album's next track when the song ends. When PlaylistMaker counts the song complete and sees the configured device playing another track from that album, it pauses the device, so the next track does not keep playing and scrobble to Last.fm. Playback on another device, in another context, or already stopped is left alone, because someone else has taken over. If that pause fails, the song still counts as complete and the helper logs `completion follow-on pause failed: …`.

PlaylistMaker handles repetitions itself, so it turns Spotify repeat off before each play. A device is idle when nothing is playing, another device is active, or no item is loaded, as on a freshly started librespot, and an idle device rejects that request with a 403. In that case PlaylistMaker starts the play anyway and confirms repeat is off after playback starts; the play still completes only once repeat is observed off. Any other repeat failure blocks the start.

Closing a video after at least 50% watched lets Spotify finish that song. This uses the configured playback history minimum watched percentage. Reaching the video's end also qualifies, even when the performance is shorter than the Spotify song. A separately launched video's tracking waits for the previous song to finish. Earlier closes remain skips.

If Spotify is unavailable, PlaylistMaker uses the configured foobar source when local audio exists.

## Pause, catch-up and recovery

Pausing mpv retains the queue. Earlier videos that completed can finish their Spotify plays in order. When tracking reaches the paused video's occurrence, Spotify plays naturally up to its paused media position and then pauses. It pauses immediately when already ahead; it never seeks backward. An unknown position holds the occurrence, and a zero ceiling avoids starting it. Seeking while paused changes the ceiling. A Spotify song that finishes before that ceiling remains completed and is not replayed on video resume.

The ceiling uses video media position, subtracting a positive demuxer start offset under the same rule as video duration. It is not musical synchronization: live and album versions can differ. Normal Spotify polling is up to five seconds apart; catch-up polls at most one second apart and down to 250 ms near the ceiling. With timely API responses, the intended overshoot is about one second plus API latency and the helper's 100 ms event-loop interval. Spotify rate limits take precedence, so this is a target tolerance, not a guaranteed bound during service delays.

Spotify-only pauses trigger in-place recovery on observation. Twelve seconds without advancing progress also triggers recovery. mpv holds while recovery verifies progress; accepting a resume request alone does not release it. Recovery allows three resume commands, at least eight seconds apart, within 30 seconds. Changing user intent does not permit unrelated songs or devices to be overwritten. Missing playback is retried within the same budget; a confirmed takeover requires intervention.

An interruption can occur before the first playing observation. In-place recovery then requires an accepted start request, matching durable ownership, a response timestamp at or after that request, and a position consistent with the initial confirmation window. Retry does not widen that position window to admit an older repeat. A resume response must advance before the video hold releases; a deliberate user hold also suspends the initial confirmation budget.

A device takeover blocks queue advancement even after near-end progress. That progress evidence is retained, but it does not authorise a command on the previous device. For a deliberately paused short tail that finishes between polls after resume, a matching stopped endpoint can verify completion: its position must advance to the known duration, its timestamp must be newer than the paused sample and at or after the resume request, and enough time must have elapsed to play the tail. A stationary pause, stale/missing timestamp, or unproven endpoint never triggers another resume at the endpoint.

At the natural end of a song with nothing after it in the context, such as an album's last track, librespot, and occasionally the desktop app, rewinds to position 0 and pauses on the same track instead of reporting a stop. PlaylistMaker counts this rewind-to-start endpoint as completion when the play had reached near its end and then jumped back to 2 seconds or less, and the next stopped sample shows the same track paused near the start, with repeat off, on the configured device, and with no deliberate hold. The song is not resumed or replayed, and the helper logs `completion decision: accepted rewind-to-start endpoint after reaching …`. A rewind that keeps playing past 2 seconds, or a backward jump from mid-song, goes through the recovery handling above.

Spotify must initially confirm the requested play and repeat-off state within 30 seconds. A song starting with its video holds mpv until the play is confirmed. A song starting late behind an already-playing video, or behind a later video, gets five seconds to confirm while mpv keeps playing and shows a waiting message; if it is still unconfirmed, mpv holds. Completion retains the remaining-duration-plus-60-seconds budget, excluding confirmed deliberate holds. Catch-up still spends that budget. HTTP requests have ten-second limits; deadline decisions happen between requests, so an in-flight request can add up to ten seconds to detection. Rate-limit backoff cannot extend recovery or completion deadlines. Confirming a ceiling pause has the same 30-second recovery budget, with pause commands at least five seconds apart.

mpv displays persistent status and enforces the hold. Space/Play requests retry or resume; `p` toggles user pause intent, including during recovery. `Ctrl+Shift+U` explicitly continues without tracking for this session. Closing or skipping retains the existing qualification rules. A user pause during recovery prevents automatic video resume. The TUI also shows current session status. Windows notification-area balloons supplement blocked/health incidents; Windows settings may suppress them and correctness never depends on delivery.

The helper publishes a heartbeat from its event loop every second. mpv holds after 15 seconds without a changed heartbeat, even with the TUI closed. A slow request sequence may conservatively trigger this protection. Play can restart a dead helper; a still-running unresponsive helper must be closed before it can be replaced. No replacement steals its live lock.

Blocked sessions retain a private checkpoint of active/pending occurrence IDs and Spotify completion evidence. On retry after a helper restart, only a previously confirmed occurrence with a matching durable ownership record, matching device/song, and a non-regressed paused position away from the song's end can be adopted. A missing song, uncertain start, end position or changed ownership remains blocked for intervention; it is never automatically replayed. These checkpoints are retained after failed detached runs. They are local recovery records, not Last.fm acceptance evidence.

A confirmed deliberate pause releases the shared tracking lock after one minute. The session retains its queue and reacquires ownership on resume or a changed ceiling. If another session has replaced the Spotify song, the old session blocks instead of overwriting it. Automatic recovery is bounded after mpv closes; exhausted detached recovery exits and releases ownership while retaining the checkpoint. Preflight/start failures known to precede a Spotify play can still use local fallback. An uncertain Spotify play request cannot switch providers mid-occurrence.

## Quick fixes

- **Spotify login rejects the callback:** confirm the dashboard and `config.yaml` both use `http://127.0.0.1:43827/callback` exactly.
- **No device found:** for the desktop app, open Spotify on that device, play and pause something, then try again; for librespot, check that it is still running in its terminal.
- **More than one device found:** give the target device a unique name and update `spotifyDeviceName`.
- **403 errors:** confirm the app owner has Premium. If signing in with another Spotify account, add that account under the app's **Users Management** settings. Spotify explains this in its [development-mode guide](https://developer.spotify.com/documentation/web-api/concepts/quota-modes).
- **You need to sign in again:** close PlaylistMaker, remove `spotify-auth.json` from the configured data directory, restart, and press `U`.
