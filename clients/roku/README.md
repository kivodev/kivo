# Kivo for Roku

The Roku app: a SceneGraph app in BrighterScript, compiled to the zip a Roku sideloads. Roku runs no web code, so it shares no code with the TV web build; it shares the contract and the tables instead, each generated or packaged from the one source:

- `source/generated/api.bs`: a function per API operation returning the request to send, and an enum per named string enum, written from `server/internal/api/openapi.yaml` by `make generate` in `server/` (`internal/api/internal/brsgen`). `make check-generated` fails while it's stale.
- `source/generated/tokens.bs`: the dark theme's colours and the TV type scale and layout, written from `shared/tokens.json` by `./gradlew :tokens:rokuTokens` in `clients/`. `:tokens:test` fails while it's stale.
- `display/`: `shared/display`'s words and language tables, packaged as they are (`bsconfig.json`) and read by `source/shared/display.bs`.
- `images/`: the channel poster and splash, rendered from `shared/brand` by `shared/brand/render.sh` (needs `rsvg-convert`).

It finds the server at a typed address, HTTPS first, as the other apps do; signs in by linking, with a code to approve on a signed-in device; and shows Home, a title's page with a show's episodes by season, and plays in Roku's Video node. A play says what the Roku decodes, asked of the device (`source/shared/capabilities.bs`), so the server sends the file where it can, else HLS; the player reports its position every ten seconds, stops the play when it ends or Back leaves, and retries as a transcode once when playback fails. Every request runs on its own `ApiTask`.

## Build

```
npm ci
npm run build
```

This compiles the app with BrighterScript (`bsc`), checks it with bslint, and writes `build/kivo-roku.zip`. `npm run check` compiles the app and its tests without packaging, as CI does.

## Sideload

Turn on developer mode on the Roku (Home three times, Up twice, Right, Left, Right, Left, Right), open the address it shows in a browser and upload `build/kivo-roku.zip`, or let `bsc` do it: `npx bsc --deploy --host <roku-ip> --password <dev-password>`.

## Tests

The Rooibos tests in `tests/` hold the track words to the shared vectors (`shared/display/vectors/tracks.json`), which the web client's and the core's tests also read, and read typed addresses as the core does. They run only on a Roku: `npm run build:tests` writes `build/kivo-roku-tests.zip`, which prints its results to the Roku's debug console (port 8085) when sideloaded. CI compiles them but can't run them.

## Not done yet

Search, the libraries' grids, track menus in the player, books, live events, and a QR code for the link. None of it has run on a Roku yet; check on one first: images (the server serves WebP), HLS with fMP4 segments, a version in parts, and the remote's Back from the player.
