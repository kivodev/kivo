# Kivo Cast receiver

Kivo's Google Cast Web Receiver, for app ID
`2457CC30`. It is a static page served as it is, with no build, at
https://cast.kivo.watch, the receiver URL registered for the app. It holds
no server address and no secret; each cast names its server.

## What it does

A sender's LOAD names the work, the server (its ID and the addresses to try)
and a short-lived cast token. The receiver picks the address that answers as
that server, measures what the device decodes, starts the play itself as the
token's user, and plays it. It sends the play's heartbeats, falls back once
when playback fails, starts a new play for a track, quality or boost change,
and runs up next, so a cast goes on when the phone that started it is gone.
The token goes only in the `Authorization` header; it never appears in a URL,
a log or the media status senders read.

Each API request names the receiver in its identity headers, a header per
field: `Kivo-App` (`kivo-cast`), `Kivo-App-Version` (the version in
`package.json` and `src/config.js`), `Kivo-Platform` (`cast`), and the Cast
device's `Kivo-OS-Version` and `Kivo-Device-Model` from its user agent.

## Files

- `index.html`, `receiver.css`, `lockup.svg`: the page, its screens and the
  logo they show.
- `src/main.js`: starts the framework, with the debug log when asked.
- `src/receiver.js`: loads, plays, heartbeats, track changes, up next, errors.
- `src/api.js`, `src/capabilities.js`, `src/identity.js`, `src/upnext.js`,
  `src/status.js`, `src/format.js`, `src/log.js`, `src/ui.js`,
  `src/config.js`: the parts.
- `src/legacy.js`: protocol 2, for senders and servers that still speak it.
- `test/`: the logic, under a stand-in for the Cast framework.
- `deploy.sh`: publishes the page (below).

The page uses only relative URLs, so it works at any path on any host.

## Debugging

Add `?debug` to the receiver URL (on a registered test device, in the Cast
developer console) to log to the console, Google's CastDebugLogger and
senders on the debug namespace. Tokens and play IDs are left out.

## Tests

```sh
npm test
```

Node 20 or later; no dependencies. CI's `chromecast` job runs them.

## Deploy

```sh
KIVO_CAST_DEPLOY=user@host:path bash deploy.sh
```

`deploy.sh` runs the tests, then copies `index.html`, `receiver.css`,
`lockup.svg` and `src/` with rsync over SSH to the target in
`KIVO_CAST_DEPLOY`, the folder the host serves at the receiver URL over
HTTPS. Files on the target the receiver no longer has are deleted, except
`.htaccess` and `.well-known/`. Arguments go to rsync, so `--dry-run` shows
what would change. The host and its credentials stay out of the repository:
the target comes from the environment and the login from your SSH setup.

Moving the receiver to another URL also changes the server's
`castReceiverOrigin` (in `server/internal/api/cors.go`) and the app's
receiver URL in the Google Cast developer console. The app ID stays the same.
