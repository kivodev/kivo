# Roku and Android TV

The Android TV app (`clients/android/tv`, with `clients/core` for what the apps share) is the reference the Roku app is built against. Every row below is something the Android TV app does, screen by screen, and what the Roku app does instead. Any change to a TV screen updates this file in the same change.

Status:

- **a**: it was a divergence and is fixed; the Roku app now does what Android TV does.
- **b**: it differs on purpose, the Roku way of doing the same thing (Roku's own video controls and keyboards, the options key `*` for a long press or the Menu key, and so on).
- **c**: not done on Roku, or not possible there; the note says why.
- **same**: it already matched.

Books aren't on the TV: Android TV leaves out audiobook libraries, book rows on Home and books in lists, search and person pages, and so does the Roku app (status a, throughout).

## Sign-in

| Android TV | Roku | Status | Note |
| --- | --- | --- | --- |
| Finds servers on the network, choosing the one found by itself and listing several, with "Enter address" a step away | Enter address only | c | Finding servers needs multicast DNS from BrightScript, which can't be checked without a Roku. |
| "Server address", "Such as 192.168.1.20, nas:8484 or https://kivo.example.com", "Connecting…", "No server found at that address." | The same words | a | |
| A typed address tries HTTPS first, then HTTP | The same | same | Ported from the core. |
| A server the app doesn't work with says which needs an update, with Try again and Change server | Says the server needs a newer Kivo on this Roku | b | One line rather than a screen. |
| The sign-in code in a panel with a QR code of its link, "Sign in to {server}", the server's name under the code | The code with a QR code of its link at the right, drawn by the server, "Sign in to {server}" and the server's name; the link is written out instead if the QR code doesn't load | a | The Roku can't draw a QR code, so the server draws it as a PNG for a waiting code. |
| A code that expires is replaced; one declined says "Sign-in was declined." over its replacement | The same | a | |
| "Couldn't get a sign-in code." with Try again | The same | a | |
| Sign in with password: the username, Next, then the password naming the username; Back from the password returns to the username | The same, in Roku's keyboard dialogs | a | Roku keyboards (b). |
| The saved server unreachable: "Can't reach {server}." retrying, with Retry now and Change server | The same, on the screen that couldn't load | a | Shared `StateView`. |

## The rail (main screen)

| Android TV | Roku | Status | Note |
| --- | --- | --- | --- |
| A rail at the left on every screen: Search, Home, Lists, a line, the pinned libraries, More libraries, Settings | A menu panel opened with Left from Home's first card, in that order | b | Roku apps open a menu rather than keep a rail beside every screen. The order and items were a divergence (a). |
| More libraries lists the unpinned ones | The same | a | |
| The remote's Search key opens Search | No | c | A Roku remote's search goes to Roku's own search, which deep links into the app. |
| Back goes back a page; at Home it asks "Exit Kivo?" with No focused first, then Exit | The same | a | |
| A mark a card or page made that failed says so once (a toast) | A line at the foot of the screen | b | |
| Live events refresh screens in place | Screens read again as they show | c | Live events aren't done on Roku yet. |
| Watch Next programs on the Android TV home screen open their work's page | Deep links from Roku's search and home screen: a movie or episode plays, anything else opens its page | b | Roku's own channel integration in place of Watch Next. |

## Home

| Android TV | Roku | Status | Note |
| --- | --- | --- | --- |
| Rows of kinds Continue Watching, Recently Added and lists only | The same | a | Continue Listening and other kinds were shown before. |
| A list row drops books; a row left empty shows nothing; rows of libraries it doesn't show are left out | The same | a | |
| Continue Watching and a videos library's rows are landscape, the others posters | The same | a | Videos libraries were posters before. |
| A list's row ends in See all, which opens the list | The same | a | |
| A list's row draws each card in its own work's shape | Posters for the whole row | b | A RowList row has one card size. |
| Titles "Continue Watching", "Recently Added in {library}", the list's name | The same | a | |
| Empty Home: "No libraries" (an admin: "Add libraries in the web app…"; others: "Ask an admin for access."), "Your Home is empty" when every row is hidden, else "Nothing here yet" | The same | a | |
| A failed read: "Couldn't load this." with Try again, or unreachable with Retry now and Change server | The same | a | |
| Back away from the first card moves focus to it | The same | a | |
| No heading over the rows | The same | a | Roku showed "Home". |
| Skeleton rows while it loads | Nothing until the rows come | c | No skeletons on Roku yet. |

## Cards (everywhere)

| Android TV | Roku | Status | Note |
| --- | --- | --- | --- |
| An episode, or a season bundled in Recently Added, goes by its series' title; a movie with its edition, "Blade Runner · Final Cut" | The same | a | |
| The line under the title: a show's years or new episodes, a season's episodes, an episode's title and time left, a movie's time left in progress, else the year | The same | a | |
| An episode's code as a badge, an unplayed count or a played check, at most one status badge (Identifying, Unmatched), progress along the foot | The same | a | |
| Without artwork, a titled tile, never a broken image | The same | a | |
| A long press of OK, or Menu, opens the work's menu: Play or Resume, Play from beginning, Mark as watched or unwatched, Remove from the open manual list, Remove from Continue Watching in that row, Go to series (not on its series' page), Go to library (outside a library) | `*` opens the same menu on every screen with cards | a | `*` for the long press (b); only Home had a short menu before. |
| Focus grows a card with a glow and a sheen | Roku's own focus frame | b | |
| A person's round photo | A square photo | b | |
| A collection's card says how many movies or items it has | The same | a | |

## Libraries

| Android TV | Roku | Status | Note |
| --- | --- | --- | --- |
| Sections named by what they list ("Movies", "Shows", "Videos"), then Collections when it has any | The same | a | It said "Titles". |
| The count line, "237 movies · A–Z", which opens Sort | The same, a button in the bar | a | |
| Sorts as the core's `sortsOf` lists them, Runtime for shows too, "Last episode added" | The same | a | |
| Filter with a dot while any is on; the watch status, decade, genre, content rating, resolutions and dynamic ranges the library has; each applies as chosen; Clear filters turns them all off and closes the panel | The same labels, order and behaviour, on a page per filter | b | Roku's menu panel has one list at a time. |
| Empty: "Nothing in this library yet.", "No unwatched movies", "No movies match these filters" with Clear filters | The same | a | |
| The section, sort and filters this device last used in the library open next time | The same | a | |
| The letter rail at the right for the title sort | A–Z in the bar, a list of letters | b | |
| Channel Up and Down page the grid | Roku's rewind and fast-forward page a MarkupGrid | b | |

## Lists, an open list and a collection

| Android TV | Roku | Status | Note |
| --- | --- | --- | --- |
| A shelf per list: its name, Smart list for a smart one, "12 titles", its first works, See all | The same | a | |
| A shelf only for a known kind, Watchlist only with something in it, none for a list with nothing the TV plays | The same | a | |
| Empty: "Nothing in your lists yet." and "Add something to Watchlist from its page." | The same | a | |
| An open list: its name and count, the grid of its works the TV shows, each in its own shape | Its name and count, its works the TV shows in lines of six posters | b | Lines of a RowList for the grid. |
| An empty open list: "Nothing in this list yet.", "Nothing matches these rules." or "These rules no longer work." | The same | a | |
| A manual list's card menu offers Remove from {list} | The same | a | |
| A collection: its name and count over its backdrop, its works as a row of posters | The same | a | It broke them into lines before. |

## Search

| Android TV | Roku | Status | Note |
| --- | --- | --- | --- |
| People first, then Movies, Shows, Episodes, Other videos (videos and extras), then Collections | The same | a | |
| A person's card: "Actor · 23 titles" | The same | a | |
| "Type to search every library.", "No results for “…”." with Clear search | The same | a | |
| Searches once typing pauses about 350 ms, from one letter, 50 results | The same | a | |
| Opens on the last search typed while this server is open, searched again at once | The same | a | |
| The on-screen grid keyboard with voice input | Roku's MiniKeyboard; voice where the Roku remote has it | b | |

## A person's page

| Android TV | Roku | Status | Note |
| --- | --- | --- | --- |
| Photo and name, then Movies, Shows and Other videos | The same | a | Books, book series and Narrated were shown before. |
| With none: "No titles available" | The same | a | |

## A title's page (movie, video, series, episode)

| Android TV | Roku | Status | Note |
| --- | --- | --- | --- |
| A season opens its series on it | The same | a | |
| The details line: a movie's year, runtime, ★ rating, content rating and two genres; a series' years ("2016 to 2019"), seasons, rating and content rating; an episode's "S3 · E4 · Feb 4, 2017 · 44m · ★ 8.5 · TV-14" | The same | a | |
| An episode's series' name over its title | The same | a | As a label; Go to series is in `*`. |
| Play, or "Resume 12:34", and Play from beginning beside Resume | The same | a | It said "Resume". |
| A series' Play is Next Up, "Play S1 E2", with "S3 · E4 · Reload · 30 min left" under it | The same | a | |
| "This file is missing" in Play's place when no version can play | The same | a | |
| Version, Audio and Subtitles pickers; Audio & Subtitles with Automatic heading each column | Audio & Subtitles opens Audio, Subtitles and Version, with Default and "Remembered for this show" | b | One menu panel; it saves a pick for the whole show as the web does (#164). |
| Search subtitles… in Audio & Subtitles | No | c | Subtitle search isn't done on Roku yet. |
| Watchlist: Add to Watchlist or Remove from Watchlist | The same | a | Roku added a work to any list from `*` before; Android TV has only Watchlist. |
| Mark as watched or unwatched; a series asks "Mark 12 episodes as watched?" with Cancel first | The same | a | |
| ⋯: Go to series for an episode, Go to library | `*` on the hero | a | `*` for ⋯ (b). |
| Rows: More like this, Trailers (the trailer files in the work's folder, then YouTube's), Cast (actors, up to 20, with their characters), Extras (the others), Collections | The same, without YouTube trailers | a | The order and Cast were different before. YouTube trailers: c, a Roku app can't play them. |
| A series' seasons as pills over one season's row of episodes, opening on Next Up | A row per season, "Season 3 · 10 episodes" or "Specials", opening on Next Up | b | Rows in place of the pills on Roku. |
| An episode's page has its season's row, opening on it | The same | a | |
| An episode card: OK on the thumbnail plays it, its text opens its page; the Play keys play it | OK opens its page; Play plays it | b | One stop per card on Roku. |
| A hero that holds its place while the page is read, from the card | The page shows once read | c | No skeletons on Roku yet. |

## The player

| Android TV | Roku | Status | Note |
| --- | --- | --- | --- |
| Controls that show on any input: the scrub bar with chapter ticks, previews, the time left and when it ends; back 10 and forward 30 | Roku's Video node and its own controls, with BIF previews | b | |
| Audio & Subtitles, Playback Settings (Quality, Version, Speed, Stream info), previous and next episode buttons | `*` opens Playback Settings: Audio, Subtitles, Dialogue boost, Quality, Version, Stream info, Previous episode, Next episode | a | The menu was "Options" without Quality, the dialogue boost or the episodes. |
| Speed | No | c | Roku's Video node has no playback rate the app can set. |
| Quality: Auto, Original and the rungs below the file's bitrate, those above the server's cap for the play left out; it lasts for this video | The same | a | |
| The dialogue boost, kept on this device for later plays | The same | a | |
| Search subtitles… | No | c | As on the title's page. |
| Skip Intro, Skip Recap, Skip Credits | The same | same | |
| Up next: "UP NEXT", "Playing in 10 s", Play now with Watch credits at the credits, else Hide; "Still watching?" after three in a row | The same | a | It said "Play Now", "Watch Credits" and "Keep Watching". |
| The play ended elsewhere: "Playback was stopped." | The same | a | |
| Playback stopped: "Couldn't play this." or "Stream limit reached" (with the other plays), the reason, Try again, Play another version, Back; an admin sees the problem's code | The same | a | It said one line before. |
| Subtitles in Kivo's style, libass for styled ones | Roku's own caption rendering | b | Roku draws captions as the user's Roku settings say. |

## Settings

| Android TV | Roku | Status | Note |
| --- | --- | --- | --- |
| Sections beside their settings: Account, Playback, Audio, Subtitles, Quality, Display, Home, Libraries, About | The same sections but Display | a | It was one list. Display: b, Roku sets the display itself. |
| Account: "Signed in to {server}", Switch account, Change server, Link a device and Sign out, each sign-out asking first | The same | a | |
| Link a device: the code, "Allow {device} to sign in as {user}?", whether it's on the home network, the warning, Don't allow first | The same, without the platform and when it asked | a | |
| Playback: Play next episode automatically, Remember choices per show | The same | a | |
| Audio: Audio language ("Original language"), then this TV's passthrough, channels and dialogue boost | Audio language and the dialogue boost | b | Passthrough and channels are Roku's own audio settings. |
| Subtitles: Show subtitles (Off, Only forced, Foreign audio only, Always), the language only while they show ("Server default"), then size and background | The same, without size and background | b | Caption style is Roku's own setting. |
| Quality: Local network quality, Remote quality, "Limited to … by the server." | The same | a | |
| Home: the rows the TV shows, each shown or not, Move up and Move down, Reset Home | The same | a | |
| Libraries: the TV's libraries pinned or not, Move up and Move down past the next the TV lists | The same | a | |
| About: app version, server version, the server's address, Credits, Open source licenses | The same but Open source licenses | b | The Roku app ships no third-party code. |
| A row whose change failed: "Couldn't save." with the reason | The same | a | |
