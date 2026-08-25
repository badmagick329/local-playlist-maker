# Spotify setup

PlaylistMaker uses Spotify Connect as its preferred tracking player. mpv remains the audible video player. PlaylistMaker does not change Spotify volume; mute Spotify yourself before synchronized playback if you want it silent.

## What you need

- A Spotify Premium account. Spotify requires Premium for Web API playback control.
- The Spotify desktop app, or another active Spotify Connect device.
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

## 2. Configure PlaylistMaker

Open your ignored local `config.yaml` and fill in these values:

```yaml
spotifyClientId: "your-client-id"
spotifyDeviceName: "the device name shown by Spotify Connect"
spotifyRedirectUri: "http://127.0.0.1:43827/callback"
```

The device name must match one available Spotify Connect device. Capitalization does not matter, but the name must otherwise match and must be unique. Volume-control support is not required. Spotify's Web API device name may differ from the label shown in the Spotify Connect picker; if the match fails, PlaylistMaker lists the available API names and types.

For the easiest setup, open the Spotify desktop app on this computer and use the name shown for it in Spotify's device picker. Leave Spotify running while using PlaylistMaker.

## 3. Sign in and link tracks

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

## 4. Test playback

Keep the configured Spotify device open, queue a linked video, and play it. PlaylistMaker should:

1. Leave Spotify at whatever volume you want (mute it manually if you want silent tracking).
2. Start the linked Spotify track from the beginning.
3. Play the video's audible sound through mpv.
4. Pause Spotify when playback ends. PlaylistMaker does not change or restore Spotify volume.

If Spotify is unavailable, PlaylistMaker uses the configured foobar source when local audio exists.

## Quick fixes

- **Spotify login rejects the callback:** confirm the dashboard and `config.yaml` both use `http://127.0.0.1:43827/callback` exactly.
- **No device found:** open Spotify on that device, play and pause something, then try again.
- **More than one device found:** give the target device a unique name and update `spotifyDeviceName`.
- **403 errors:** confirm the app owner has Premium. If signing in with another Spotify account, add that account under the app's **Users Management** settings. Spotify explains this in its [development-mode guide](https://developer.spotify.com/documentation/web-api/concepts/quota-modes).
- **You need to sign in again:** close PlaylistMaker, remove `spotify-auth.json` from the configured data directory, restart, and press `U`.
