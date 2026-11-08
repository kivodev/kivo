# 0001: Each user arranges their own Home

Status: Proposed

## Context

Home is the same for everyone. `query.Home` builds a fixed order: Continue Watching, Favorites, a Recently Added row for each pinned video library, then Continue Listening, Continue Series and a Recently Added row for each pinned audiobooks library. A user can't hide Continue Watching or Favorites, move a library's row above Continue Watching, or see anything on Home that isn't one of those five kinds.

The only control is the library pin (`LibraryChoice.pinned`). It does two jobs at once: an unpinned library leaves the sidebar and loses its Home row. Someone who wants a library in the sidebar but not on Home, or the reverse, can't have it.

This matters most to people who rarely browse a library and use Home to find something to watch. On one household server, almost all of 2,000 movies were added in a single day, so Recently Added in that library mostly showed old films, while about 96 unplayed films released this year sat further down. In that server's TV library, by contrast, nearly every Recently Added card was an episode that aired in the past month. No fixed set of rows serves both cases, or both kinds of viewer. We considered adding rows for everyone, such as New Episodes and New Movies, but that keeps guessing what people want instead of letting them say.

## Decision

Home becomes a list of rows that each user arranges. The server still decides what goes in each row and keeps building Home; the user decides which rows appear and in what order.

### What a user can do

- Show or hide any row, including Continue Watching and Favorites.
- Put the rows in any order.
- Add a row from a library and a filter: a library, plus the filters its grid already has (played state, genres, years, content rating) and a sort. The user names it, for example "90s comedies, unplayed". Its works come from the same query as the library's grid, so the row and the grid agree.
- Reset to the default Home.

A user who never changes anything sees today's Home.

### Where

Settings › Home & libraries on every client. The web and Samsung TV clients already call the section that; the Apple and Android apps call it Libraries and are renamed to match. The section has two parts:

- **Home rows**: the editor. On phones, tablets and the web, rows are dragged to reorder, switched on or off, and added from a sheet that previews how many works match. On TVs, Edit Home moves the focused row with up and down and shows or hides it with OK. New rows are built on a phone or the web, where choosing filters is easy.
- **Libraries in the sidebar**: today's pins, which now control only the sidebar.

Home ends with a short "Edit Home" link to the same editor.

### Server

The layout is stored per user on the server, so it follows the user to every device.

- A new table holds each user's rows: position, kind, library (for `recentlyAdded` and `custom`), shown or hidden, and for `custom` rows the title and filter.
- `query.Home` builds the user's rows in their order and skips hidden ones. A row that has no works is still skipped, as today. Without a stored layout it builds today's order.
- A `custom` row runs the library listing's query with the saved filter and sort, limited to the row size (20). It doesn't need a new query.
- When the migration runs, each user's layout starts from their current pins: an unpinned library's Recently Added row starts hidden, so no one's Home changes.
- Removing a library removes its rows from every layout.

### Contract

The contract grows by addition, as the rest of it does. A sketch of the changes to `openapi.yaml`:

```yaml
HomeRowKind:
  enum: [continueWatching, continueListening, continueSeries, recentlyAdded, favorites, custom]
  # `custom`: works from a library, filtered and sorted as the user saved it.

HomeRow:
  properties:
    title:
      type: string
      description: For `custom`, the name the user gave the row.
    filter:
      $ref: "#/components/schemas/LibraryFilter"
      description: For `custom`, the filter and sort, to open the library's grid with them.

LibraryFilter:
  type: object
  description: A library grid's filter and sort, as `listLibraryWorks` takes them.
  properties:
    sort: {$ref: "#/components/schemas/WorkSort"}
    played: {$ref: "#/components/schemas/PlayedFilter"}
    genre: {type: array, items: {type: string}}
    yearFrom: {type: integer}
    yearTo: {type: integer}
    contentRating: {type: array, items: {type: string}}

HomeRowChoice:
  type: object
  required: [kind, shown]
  properties:
    id:
      type: string
      description: For `custom`, the row's ID. Leave it out to add a row.
    kind: {$ref: "#/components/schemas/HomeRowKind"}
    library: {$ref: "#/components/schemas/ID"}
    shown: {type: boolean}
    title: {type: string}
    filter: {$ref: "#/components/schemas/LibraryFilter"}

CurrentUser:
  properties:
    home:
      type: array
      description: The user's Home rows in order, shown or hidden.
      items: {$ref: "#/components/schemas/HomeRowChoice"}

CurrentUserUpdate:
  properties:
    home:
      type: array
      description: |
        The user's Home rows, in the order it wants them; it replaces the
        last layout. A row left out, such as a new library's, is shown after
        the listed ones, as an unlisted library is pinned today.
      items: {$ref: "#/components/schemas/HomeRowChoice"}
```

`LibraryChoice.pinned` changes meaning to "shown in the sidebar". That is the one change to existing behavior; the migration above keeps it invisible.

### Order of work

Each step ships on its own.

1. Show, hide and reorder the existing rows, and separate the sidebar from Home: the table, `home` on `CurrentUser` and `CurrentUserUpdate`, `query.Home` following the layout, and the editor on each client.
2. Custom rows: `custom`, `LibraryFilter` and the Add a row sheet.
3. Optionally, built-in rows such as New Episodes and New Movies, offered as presets in Add a row. Unlike custom rows, these need new queries.

## Consequences

- People shape Home around what they actually watch, instead of us guessing for everyone. The default stays as it is today, so nothing changes for anyone who never opens the editor.
- Defaults still matter most, because most people never open a Home editor. Configuration doesn't replace a good default Home; it's for the people who go looking.
- Custom rows are cheap for the server, because they reuse the library grid's query, and they open the grid with the same filter.
- Every client needs an editor, and TVs need a remote-friendly one. Building filters is left to phones and the web.
- Older clients keep working: they skip the `custom` row kind they don't know and ignore the new fields. An older client that unpins a library now changes only the sidebar.
- A login shared by several people on a living-room TV has one layout. Kivo has accounts, not profiles; per-profile Homes would be a separate decision.

Open for review:

- Should a work appear in two rows (a film in both "90s comedies" and "Mystery & Crime"), or only in the higher one?
- A cap on custom rows per user, to bound how much work Home does?
- Are presets (step 3) in scope, or left to custom rows?
