# Kivo for LG TV

The LG TV app: the web client's TV build packaged into a webOS web app. It is the build the Samsung TV app packages (`clients/tizen`), so nothing of the app loads from the server but data, artwork and media, and it asks for the server's address, signs in with an app session and keeps its token the same way. It is a test package for Developer Mode; it isn't in LG's store.

## Package

```
bash package.sh
```

This runs `npm run build:packaged` in `server/web`, copies `dist-packaged` into `www/` beside `appinfo.json` and the icons, takes out the script tag for Samsung's `webapis`, which an LG TV doesn't have, and writes `build/com.kivo.player_<version>_all.ipk` with LG's `ares-package` (`npm install -g @webos-tools/cli`). To package a build made already, give its folder: `bash package.sh ../../server/web/dist-packaged`.

`www/` and `build/` are build output and aren't committed.

## webOS

The web client tells an LG TV by its user agent and by the `PalmSystem` global, and needs little of webOS itself.

- **Playback.** The video element plays through the TV's own media pipeline, which plays more than it answers for, so the capability report declares MKV and DTS on an LG TV without asking (`server/web/src/features/player/capabilities.ts`).
- **HDR.** The display's dynamic range says whether the panel shows HDR; the capability report leaves HDR out when it says no (`server/web/src/tv/webos.ts`).
- **Back.** `disableBackHistoryAPI` in `appinfo.json` sends the remote's Back to the page as a key, as on a Samsung TV, rather than through the page's history.
- **Exit** is closing the window, which ends the app.

On a 2024 LG OLED (webOS 24, Chromium 132) these were checked by hand: module scripts load from `file:`, a server's answer without CORS headers is readable, sign-in and the arrows work, and original MKV files play with H.264 and HEVC video to 4K HDR10 and AAC, AC-3, E-AC-3 and DTS audio, seeking and switching audio tracks. Not checked: that DTS is heard and HDR shows on the panel, Dolby Vision, TrueHD, and the remote's media keys.

## Develop on a TV

1. On the TV, install LG's Developer Mode app, sign in with an LG developer account (one with an email and password; an account made with Google sign-in is refused), and turn on Dev Mode Status. The TV restarts.
2. `ares-setup-device` adds the TV by its IP address, port 9922 and user `prisoner`; `ares-novacom --device <name> --getkey` fetches its key with the passphrase the Developer Mode app shows.
3. Install: `ares-install --device <name> build/com.kivo.player_<version>_all.ipk`.
4. Launch it: `ares-launch --device <name> com.kivo.player`.

## Debug on a TV

`ares-inspect --device <name> --app com.kivo.player` opens Chrome DevTools on the app's page. The TV also lists the page at `http://<tv-ip>:9998/json`.
