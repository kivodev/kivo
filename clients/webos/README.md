# Kivo for LG TV

The LG TV app: the web client's packaged TV build, the one the Samsung TV app packages (`clients/tizen`), as a webOS web app. It is a test package for Developer Mode; it isn't in LG's store.

## Package

```
bash package.sh
```

This runs `npm run build:packaged:webos` in `server/web` (the packaged TV build without Samsung's `webapis` script), copies `dist-packaged` into `www/` beside `appinfo.json` and the icons, and writes `build/com.kivo.player_<version>_all.ipk` with LG's `ares-package` (`npm install -g @webos-tools/cli`). To package a build made already, give its folder: `bash package.sh ../../server/web/dist-packaged`.

`www/` and `build/` are build output and aren't committed.

## Install on a TV

1. On the TV, install LG's Developer Mode app, sign in with an LG developer account (one with an email and password; an account made with Google sign-in is refused), and turn on Dev Mode Status. The TV restarts.
2. `ares-setup-device` adds the TV by its IP address, port 9922 and user `prisoner`; `ares-novacom --device <name> --getkey` fetches its key with the passphrase the Developer Mode app shows.
3. Install: `ares-install --device <name> build/com.kivo.player_<version>_all.ipk`.
4. Launch it: `ares-launch --device <name> com.kivo.player`.

## Debug on a TV

`ares-inspect --device <name> --app com.kivo.player` opens Chrome DevTools on the app's page. The TV also lists the page at `http://<tv-ip>:9998/json`.

At startup the app logs one `Kivo HDR check` line to the console, with the display's answer to whether the panel shows HDR, which the app goes by, beside LG's device info for HDR10 and Dolby Vision. Reload the page in DevTools to see it again.
