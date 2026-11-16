# Kivo for Roku

The Roku app: a SceneGraph app in BrighterScript, compiled to the zip a Roku sideloads. Roku runs no web code, so it shares no code with the TV web build; it shares the contract and the tables instead, each generated or packaged from the one source:

- `source/generated/api.bs`: a function per API operation returning the request to send, and an enum per named string enum, written from `server/internal/api/openapi.yaml` by `make generate` in `server/` (`internal/api/internal/brsgen`). `make check-generated` fails while it's stale.
- `source/generated/tokens.bs`: the dark theme's colours and the TV type scale and layout, written from `shared/tokens.json` by `./gradlew :tokens:rokuTokens` in `clients/`. `:tokens:test` fails while it's stale.
- `display/`: `shared/display`'s words and language tables, packaged as they are (`bsconfig.json`) and read by `source/shared/display.bs`.
- `images/`: the channel poster and splash, rendered from `shared/brand` by `shared/brand/render.sh` (needs `rsvg-convert`).

The Android TV app is the reference this app is built against, screen by screen; `PARITY.md` lists what it does on each screen and what this app does instead, and any change to a TV screen updates it.

It finds the server at a typed address, HTTPS first, as the other apps do, and signs in with a code to approve on a signed-in device, or with a password. Books aren't on the TV, as on Android TV: audiobook libraries, book rows and books in lists, search and person pages are left out. Then:

- Home: the rows the TV shows, Continue Watching and a videos library's landscape, a list's ending in See all. Left from a row's first card opens the menu in the rail's place (Search, Home, Lists, the pinned libraries, More libraries, Settings).
- `*` on any card opens its menu, as a long press does on Android TV: Play or Resume, Play from beginning, Mark as watched, Remove from the open list or from Continue Watching, Go to series and Go to library.
- A title's page: the details line, Play or Resume with the time, Play from beginning, Audio & Subtitles (for an episode, a pick is kept for the whole show), Watchlist and Watched, then a series' seasons (an episode's own season), More like this, Trailers, Cast, Extras and Collections. `*` on the page goes to the series or the library.
- Libraries: a grid per library that reads a page at a time around the focus, with its sections, the count line that opens Sort, filters and A to Z, remembered per library on this device.
- Lists, an open list, a collection, a person's page and search, in the Android TV app's order and words.
- Settings in Android TV's sections: Account (switch account, change server, link a device, sign out), Playback, Audio, Subtitles, Quality, Home rows, Libraries and About.
- The player: Roku's Video node and its own controls. `*` opens Playback Settings: audio, subtitles, the dialogue boost, quality for this video, version, stream info and an episode's previous and next ones. Skip shows in an intro, recap or credits marker; up next counts down near an episode's end as the user's autoplay has it, asking "Still watching?" after three in a row; the seek bar shows previews from the server's BIF file. A play says what the Roku decodes, asked of the device (`source/shared/capabilities.bs`), so the server sends the file where it can, else HLS; the player reports its position every ten seconds, stops the play when it ends or Back leaves, retries as a transcode once when playback fails, and then says why with Try again, Play another version and Back.
- Deep links from Roku's search and home screen, at launch or while running: a movie or an episode plays, anything else opens its page. Back on Home asks "Exit Kivo?".

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

`PARITY.md` has each one with its reason. In short: finding servers on the network, the QR code on the sign-in screen, subtitle search, playback speed, YouTube trailers, live events, and loading skeletons.

None of it has run on a Roku yet. The first sideload should check: images (the server serves WebP), HLS with fMP4 segments, a version in parts, Back from the player, subtitle and audio switches inside an HLS stream, Skip and up next, BIF previews while seeking, a deep link, the exit dialog, the card badges' sizes, the keyboard dialogs of password sign-in and Link a device, and the settings' two panes.
