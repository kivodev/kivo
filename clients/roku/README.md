# Kivo for Roku

The Roku app: a SceneGraph app in BrighterScript, compiled to the zip a Roku sideloads. Roku runs no web code, so it shares no code with the TV web build; it shares the contract and the tables instead, each generated or packaged from the one source:

- `source/generated/api.bs`: a function per API operation returning the request to send, and an enum per named string enum, written from `server/internal/api/openapi.yaml` by `make generate` in `server/` (`internal/api/internal/brsgen`). `make check-generated` fails while it's stale.
- `source/generated/tokens.bs`: the dark theme's colours and the TV type scale and layout, written from `shared/tokens.json` by `./gradlew :tokens:rokuTokens` in `clients/`. `:tokens:test` fails while it's stale.
- `display/`: `shared/display`'s words and language tables, packaged as they are (`bsconfig.json`) and read by `source/shared/display.bs`.
- `images/`: the channel poster and splash, rendered from `shared/brand` by `shared/brand/render.sh` (needs `rsvg-convert`).

It finds the server at a typed address, HTTPS first, as the other apps do, and signs in by linking, with a code to approve on a signed-in device. Books aren't on the TV, as on Android TV: audiobook libraries, book rows and books in lists and search are left out. Then:

- Home: the user's rows. Left from a row's first card opens the menu (Search, each library, Lists, Settings); * on a card marks it watched or takes it off Continue Watching.
- A title's page: Play, Resume or Play from beginning, Audio & Subtitles (for an episode, a pick is kept for the whole show, as the other apps keep it), a show's episodes by season, More like this, extras, collections and the cast, each person with their own page. * marks it watched and adds it to a list.
- Libraries: a grid per library that reads a page at a time around the focus, with tabs (titles and collections), sort, filters and A to Z.
- Lists, search (titles, people and collections) and settings: quality at home and away, the account's audio and subtitle preferences, remembering choices per show, autoplay, the server and signing out.
- The player: Roku's Video node and its own controls. * opens audio, subtitles, version and playback info; Skip shows in an intro, recap or credits marker; up next counts down near an episode's end as the user's autoplay has it, asking "Still watching?" after three in a row; the seek bar shows previews from the server's BIF file. A play says what the Roku decodes, asked of the device (`source/shared/capabilities.bs`), so the server sends the file where it can, else HLS; the player reports its position every ten seconds, stops the play when it ends or Back leaves, and retries as a transcode once when playback fails.
- Deep links from Roku's search and home screen, at launch or while running: a movie or an episode plays, anything else opens its page. Back on Home asks before the app closes.

Every request runs on its own `ApiTask`.

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

- Trailers: they're YouTube links, which a Roku app can't play itself.
- A QR code on the link screen: it needs a QR encoder on the Roku, which couldn't be checked against a reference here; the link and the code show.
- Live events from the server, the "Still watching?" run counting the remote's keys inside the video player, and the other apps' admin pages.

None of it has run on a Roku yet. The first sideload should check: images (the server serves WebP), HLS with fMP4 segments, a version in parts, Back from the player, subtitle and audio switches inside an HLS stream, Skip and up next, BIF previews while seeking, a deep link, and the exit dialog.
