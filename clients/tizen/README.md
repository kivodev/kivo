# Kivo for Samsung TV

The Samsung TV app: the web client's TV build packaged into a Tizen web app. Its `www/` is the packaged TV build, so nothing of the app loads from the server but data, artwork and media. It asks for the server's address, signs in with an app session and keeps its token, and talks to the server across origins from its `file:` page. It is the only Samsung TV app, for the store and for Developer Mode alike.

## Package

```
bash package.sh
```

This runs `npm run build:packaged` in `server/web` (the TV build with `VITE_KIVO_TV=1 VITE_KIVO_PACKAGED=1`, and `KIVO_TIZEN=1` for Samsung's `webapis` script, written to `dist-packaged`, beside the `dist` the server embeds), copies it into `www/`, and writes an unsigned `build/kivo-tizen.wgt` of `config.xml`, `icon.png` and `www/`. To package a build made already, give its folder: `bash package.sh ../../server/web/dist-packaged`. Before packaging it checks that `www/index.html` has a Content-Security-Policy meta without `'unsafe-inline'` or `'unsafe-eval'` and loads no script or stylesheet from another origin, which Samsung refuses, that `config.xml` carries the same policy, and that the page loads Samsung's `webapis` script.

`www/` and `build/` are build output and aren't committed.

A TV only installs a package signed with a Samsung certificate. Sign it with Tizen Studio or the `tizen package` CLI and the author and distributor certificates: your own for Developer Mode, the project's for a store release.

## The page

`www/index.html` is the packaged build's own. Besides the app's script and stylesheet it carries:

- The CSP meta, the server's policy with the server allowed where it was `'self'`.
- `<script src="$WEBAPIS/webapis/webapis.js"></script>`, before the app's script. Samsung's `webapis` object, which says whether the panel is 1080p (`webapis.productinfo`), exists only on a page that includes it; the TV resolves `$WEBAPIS` to its own copy of the file, so nothing is fetched from a server. The `tizen` object needs no script. In a browser the script fails to load and the app goes on without the 1080p cap.

## Check the built page

```
cd server/web && npm run e2e:packaged-file
```

This builds `dist-packaged` and opens its `index.html` from a `file:` URL in headless Chromium, as the TV opens the package's page, with `--allow-file-access-from-files` so the page can load the module scripts and stylesheet beside it (`playwright.packaged-file.config.ts`, `e2e/packaged-file`). It checks that the address screen shows at its hash route, that the scripts and stylesheet load with nothing refused by the page's policy but two refusals the app expects (zod's `Function` test and react-aria's touch-action style), that typing an address sends the probe to it, and that the build leaves out the web app manifest and its icons. Chromium enforces CORS for a `file:` page where the TV doesn't, so the probe's mocked answer allows any origin. `npm run e2e:packaged` drives the app against the dev server instead.

On a Tizen 9 TV these were checked by hand: module scripts load from `file:`, a server's answer without CORS headers is readable, no `Origin` header is sent, and hash routes work.

## Tizen APIs

The web client's Tizen code is in `server/web/src/tv/`, and does nothing on a page without the APIs (the TV flag's dev server in a browser, the tests).

- **Exit** is `tizen.application.getCurrentApplication().exit()` (`exit.ts`), which needs no privilege.
- **Media keys.** Play, Pause, Play/Pause, Stop, Fast forward, Rewind, Previous, Next and Info are registered with `tizen.tvinputdevice.registerKey` once, as the app starts (`tizen.ts`), and go to the video player or the docked book, whichever is open. Back and the arrows arrive unasked. Privilege `http://tizen.org/privilege/tv.inputdevice`.
- **Panel.** Tizen lays a web app out at 1920×1080 whatever the panel, so `screen.width` says 1080p on a 4K TV. `webapis.productinfo.isUdPanelSupported()` says whether the panel is 1080p, which caps video at 20 Mb/s in the capability report (`fullHdPanel` in `tizen.ts`). Privilege `http://developer.samsung.com/privilege/productinfo`.
- **HDR.** `webapis.avinfo.isHdrTvSupport()` says whether the panel shows HDR; the capability report leaves HDR out when it says no (`hdrPanel` in `tizen.ts`). No privilege.
- **AVPlay.** File delivery (direct play of the original file) plays through `webapis.avplay`, Samsung's own player, on the TV's media pipeline: MKV, HEVC Main 10 and HDR at 4K, and every audio track of the file (`server/web/src/features/player/avplay.ts`). Streams stay on hls.js. AVPlay draws through an `<object type="application/avplayer">`, which the page's policy allows with `object-src 'self'`, on a plane beneath the page, which is transparent while it plays. No privilege on TVs from 2015 on.
- **Screensaver.** `webapis.appcommon.setScreenSaver` keeps it off while a video plays, and lets it come back when paused (`keepScreenOn` in `tizen.ts`). No privilege.

## Develop on a TV

1. On the TV, open Apps, press 12345 on the remote, and turn on Developer Mode. Enter your computer's IP address.
2. Restart the TV.
3. `sdb connect <tv-ip>`
4. Sign and install: `tizen package -t wgt -s <profile> -- build/` then `tizen install -n kivo-tizen.wgt -t <tv-name> -- build/`.
5. Launch it: `tizen run -p kivoplayer.Kivo -t <tv-name>`.

## Debug on a TV

A script error is also sent to the server's log once signed in: an admin sees it on the Logs page as `client: error reported`, with the route, the top of the stack and the TV's engine. For anything more, attach Chrome DevTools to the TV:

1. With the app installed in Developer Mode (above), launch it in debug mode: `sdb shell 0 debug kivoplayer.Kivo`. It prints `port: <port>`.
2. Forward that port: `sdb forward tcp:9222 tcp:<port>`.
3. In Chrome on the computer, open `chrome://inspect`, add `localhost:9222` under Configure, and choose the app's page under Remote Target. Or open `http://localhost:9222` and follow its link.

Tizen Studio's Run As › Tizen Web Application (Debug) does the same and opens DevTools itself.
