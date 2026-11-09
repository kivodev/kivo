# TV manual checklist

Playwright's TV project drives the TV build with arrow keys against the mock API; it can't tell how a real TV's browser engine, remote, decoders and memory behave. Run this on a real TV before each release that changes the TV build or the player.

## Minimum engine

The TV build targets Chromium 108 and newer, which is Samsung's 2024 TVs (Tizen 8.0) and later. The web client uses Tailwind v4 and CSS cascade layers (`@layer`), which Tizen 6.5's Chromium 85 doesn't support, so the page loads unstyled there. Samsung's web engine specification pairs each year with an engine:

| Year | Tizen | Chromium |
| --- | --- | --- |
| 2022 | 6.5 | M85 |
| 2023 | 7.0 | M94 |
| 2024 | 8.0 | M108 |
| 2025 | 9.0 | M120 |

Test the supported minimum on a 2024 TV when one is at hand, and on the TV below for every release. A 2022 or 2023 Samsung TV is expected to fail the first launch: record how (unstyled, blank, a script error) and don't count it as a regression. webOS 6 (2021 LG) is Chromium 87, below the line; webOS 24 (2024) and newer are Chromium 108 and newer.

- [ ] webOS minimum: on an LG TV with webOS 24 or newer, the TV build on the dev server (`VITE_KIVO_TV=1 npm run dev -- --host`), opened in the TV's browser, loads styled and reaches the sign-in screen. Record the webOS version and its Chromium version, and whether an older webOS loads the page.

Note the TV's model, firmware and engine at the top of each run.

Use a library with a direct-play MP4 (H.264 and AAC), an MKV with embedded SubRip, ASS and PGS tracks and chapters named Intro and Credits, an HEVC 10-bit HDR file, a file that needs a video transcode, an episode with a next episode, and a library of a few thousand works for the grid.

## Install

- [ ] Developer Mode is on: Apps, press 12345 on the remote, turn on Developer Mode and enter the computer's IP address, then restart the TV.
- [ ] `sdb connect <tv-ip>` finds the TV.
- [ ] `bash clients/tizen/package.sh` writes the packaged app's `build/kivo-tizen.wgt`.
- [ ] The package is signed with a Samsung certificate (author and distributor), and an unsigned package is refused by the TV.
- [ ] `tizen install` puts it on the TV, and `tizen run -p kivoplayer.Kivo -t <tv-name>` launches it.
- [ ] Kivo shows in the TV's app list with its icon and name.

## Address screen

- [ ] The first launch asks for the server's address. `<server>:8484` (no scheme) on a server with a certificate the TV trusts, `http://<server>:8484/` and a reverse-proxied `http://<host>/kivo` each reach that server's sign-in screen; `javascript:alert(1)` says "That isn't an address.".
- [ ] An address with no scheme on an HTTP-only server doesn't open: the address screen says it couldn't connect securely and suggests typing `http://`. The same address with `http://` says "Connecting without encryption" before it opens, Open then opens it, and the next launch opens it straight away with no note. On a server that serves HTTPS with a certificate the TV trusts, the address with no scheme opens over https with no note.
- [ ] The probe reads the server's info from the app's page with no CORS allowance on the server: the TV sends no `Origin` header, and the answer, with no CORS headers, is readable. If every Kivo server says "isn't a Kivo server", record from the remote inspector's Network tab whether the probe carried an `Origin` and whether its answer was blocked.
- [ ] The address of a router's admin page, or any other web server, says it isn't a Kivo server and doesn't open it.
- [ ] A server that's starting, or a NAS whose disks have spun down, shows "Waking the server..." and then opens, rather than failing.
- [ ] Restarting the app opens the saved address straight away, with no address screen.
- [ ] A wrong address or a server that's switched off brings the address screen back with the error, the old address still in the field, and a correct address then works.
- [ ] The address field takes text from the on-screen keyboard and the remote's digits, and Back from the address screen exits the app.
- [ ] A server on https with a valid certificate works; an address with or without `https://` on one with a self-signed certificate says it couldn't connect securely, names the certificate and suggests `http://`, and doesn't fall back to http.
- [ ] Exit: on Home, Back to Exit Kivo?, then Exit, leaves for the TV's home screen, with no address screen left on screen. Repeat after opening a work, going back with Back, and moving between a few rail destinations, so the app's own history has entries before and after the current one.

## Packaged app

The packaged app (`clients/tizen`) holds the TV build in its `.wgt` and talks to the server with an app session's token; nothing of it loads from the server but data, artwork and media. Run this section on the 2024 Tizen 8 TV as well as the test TV, since the Origin and CORS checks are what the server's rules rely on at the minimum engine. Every section of this checklist runs on the packaged app.

- [ ] `build/kivo-tizen.wgt` installs with `tizen install`, and the app launches from the TV's app list without a server configured and shows the address screen, with no page fetched from anywhere (the remote inspector's Network tab shows only the package's own files).
- [ ] The package's `index.html` has a CSP meta with no `unsafe-inline` or `unsafe-eval`, and the app runs with no CSP violation in the console across sign-in, Home, a work, the player and Settings. `config.xml` has `required_version="8.0"`, `access origin="*"` and no `allow-navigation`, and `www/` holds only the packaged build's files.
- [ ] Every request from the app sends no `Origin` header, and the server's answers carry no CORS headers and are read: sign-in, Home, a work, artwork, the events socket and a play all work. Record the Tizen and Chromium versions this held on. If a request is blocked, record its `Origin` and the console's error: that is the reason to put a server allowance back.
- [ ] Sign-in by code: the TV shows a code and QR code, Allow on a phone signs it in, and the TV's session in the web client's Where you're signed in is "Samsung TV (Tizen N)" with the TV's platform, listed once; signing in again by password replaces it rather than adding a second.
- [ ] No cookie: the remote inspector's Application tab shows no `kivo_session` or `kivo_client` cookie for the server, and every API request carries `Authorization: Bearer`.
- [ ] The token persists: close the app from the TV's Home key and reopen it, put the TV in standby for ten minutes and wake it, and restart the server; each time the app comes back to Home signed in with no address or sign-in screen between.
- [ ] Settings › About shows the app's version and the server's version.
- [ ] Sign out: Settings › Sign out shows the sign-in screen with the address kept, and the session is gone from Where you're signed in. Revoking the TV's session from the web client signs the TV out within 30 seconds, to the sign-in screen. Change server shows the address screen with the saved address in the field, and the session is gone; with the server switched off, Change server still shows the address screen.
- [ ] Exit: on Home, Back to Exit Kivo?, then Exit, leaves for the TV's home screen at once, through the Tizen API; the same after 50 screens opened (rail destinations, works, Back). Back on the address screen and on the sign-in screen exits the app.
- [ ] Media keys: in the player, Play (415), Pause (19), Play/Pause (Tizen 10252), Stop (413), Fast forward (417), Rewind (412), Previous (10232), Next (10233) and Info (457) each act once, Previous and Next playing the previous or next episode. With a book docked, Play/Pause pauses and resumes it on any screen, and Previous and Next move it by chapter.
- [ ] Panel: on a 4K TV, a 4K file the TV decodes plays as Direct Play; on a 1080p TV the capability report in the server's play diagnostics caps video at 20 Mb/s.
- [ ] Error reports from the packaged app show on the Logs page with the app's version as the build, the route's template and the TV's engine.
- [ ] Deep use: open 50 screens, play a film, seek, leave the player and return; routes work throughout (the URL bar in the remote inspector shows `index.html#/...`), and nothing navigates away from the package's page.

## Error reports

- [ ] Signed in, an error on the TV page shows on the admin Logs page, at Warning, as `client: error reported`, with the route's template (no IDs or query), the top stack frames, the build and the TV's Tizen and Chromium versions. A forced failure: block one of the app's chunks from the remote inspector's Network tab (Block request URL), then open that screen.
- [ ] Repeating the error sends it once, and a loop of errors adds no more than a few lines a minute.
- [ ] Signed out, nothing is sent.
- [ ] With the remote inspector: launch the packaged app in debug mode (`clients/tizen/README.md`, Debug on a TV), and DevTools attaches to its page.

## Sign-in

- [ ] Signed out, the TV shows a code and a QR code at once, the time left counts down, and a new code appears when it runs out.
- [ ] Scanning the QR code with a phone opens /link; Allow signs the TV in within a few seconds and shows Home. Denying shows "Sign-in was declined" and a new code.
- [ ] Entering the code at Link a device on a signed-in browser also signs the TV in.
- [ ] Sign in with password: Left, Right, Up and Down move over the grid keyboard, OK types, the field follows the focused field (Username, then Password), Clear and the delete key work, and a wrong password shows its error.
- [ ] After a restart the TV is still signed in.
- [ ] Settings › Sign out asks first with Cancel focused; Back cancels, and Sign out returns to the sign-in screen. The TV's session is gone from the web client's Signed-in devices.

## The remote and Back

Check each screen with the arrows, OK and the Back key (Tizen 10009).

- [ ] Home: focus starts on the first card; Down and Up change rows and return to the card left; Right past the last card reaches See all; the focused card is clearly raised.
- [ ] Back on Home moves focus to the first card, then asks Exit Kivo? with No focused; No or Back stays, Exit leaves the app.
- [ ] A work: focus starts on Play; Right reaches the My List and watched buttons and the menu; Back returns to the card it came from.
- [ ] Rail: Left from the screen's left edge opens it over a dimmed screen, entries are named, Right closes it onto the card it left, and Back in the rail on another screen goes Home.
- [ ] A library: Down goes deep and the grid scrolls with focus, the letter rail picks a letter, Up reaches the section buttons, Sort and Filter; a sort and a filter apply and Back closes them onto their button.
- [ ] Search: the grid keyboard types, Right reaches the results, OK opens one, and Back returns to the same result.
- [ ] Settings: every row is reachable, OK flips a switch, About shows the version, the server's address and its QR code.
- [ ] A dialog traps the arrows; Back closes it and focus returns to where it was.
- [ ] Back never leaves the app except from Exit Kivo?, and never shows the browser's own history.

## Playback

- [ ] The MP4 plays as Direct Play and resumes at its saved position; Stream info shows Direct Play.
- [ ] The MKV plays; a file the TV can't decode (HEVC 10-bit HDR on a TV that can't, or a DTS track) transcodes, and Stream info says Transcode with its reason.
- [ ] A far seek (an hour ahead) plays within a few seconds at the right place.
- [ ] A burned-in ASS track and a burned-in PGS track show at the right size and timing, with no second copy drawn by the page.
- [ ] A SubRip track chosen from the menu shows and follows seeks; Off hides it.
- [ ] Media keys: Play (415), Pause (19), Play/Pause (Tizen 10252), Stop (413), Fast forward (417) and Rewind (412) do what they say, and each press does it once. The packaged app registers them as it starts; to check the registration holds, start the app fresh, play a direct-play film, and press Play/Pause on the physical remote: it should pause and then resume the film, not do nothing or show the TV's own media bar. Then Stop, Fast forward, Rewind and Info. Record any key that doesn't arrive, and check the player still works with OK, the arrows and Back.
- [ ] Info (457) opens and closes the stream info over everything, including a menu.
- [ ] OK with the controls hidden shows them and doesn't pause; Left and Right seek 10 seconds, held they scrub faster and seek once on release; Back hides the controls, then leaves.
- [ ] Skip Intro and Up next take focus, and OK on them acts once.
- [ ] Smaller buffers: with the network cut for 10 seconds mid-play, playback stalls and recovers on its own, and the page doesn't reload; at the TV's lowest memory the player doesn't restart on its own.
- [ ] Leaving the player stops the play (it's gone from the admin Now playing page within seconds).

## Display and performance

- [ ] Overscan: with the TV's picture size set to its default, no card, title, rail icon or button is cut off at any edge, and nothing is lost with "Fit to screen" on.
- [ ] Focus is visible on every control from the sofa, on cards, buttons, switches, chips, keyboard keys and menu items, and never sits off screen.
- [ ] Text is readable at 3 metres; posters and backdrops aren't blurry.
- [ ] Moving focus is smooth: no visible lag on a press, and a held arrow in a grid keeps up.
- [ ] Memory while browsing a large grid: scroll the few-thousand-work library from top to bottom and back twice, then open and leave a work ten times; the app stays responsive, images keep loading, and the TV doesn't restart the app or show an out-of-memory error.
- [ ] Cold start to Home takes under 10 seconds on the home network, and Home shows rows without a blank screen.
- [ ] Leaving the app with the TV's Home key and returning keeps the signed-in session and returns to a working screen.
