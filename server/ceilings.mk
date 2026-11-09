# The ceilings make ceilings holds the code-health counts to. A count may fall
# but not rise; when one falls, lower its ceiling here to match.

# Functions nothing reachable from cmd/kivo calls (deadcode).
DEADCODE_MAX     := 0
# Groups of Go clones of 50 tokens or more (dupl -t 50). previews, subtitles
# and markers each keep their own copy of the per-file result protocol
# (settle, pending, left, failed, retry, current, commit), kept alike on
# purpose, so reading one teaches all three. previews' and subtitles' are
# identical; markers' has fail, for failures only, in place of settle, and
# its pending and left find nothing without Chromaprint. previews' and
# subtitles' prunes forget the results lost from the cache alike.
# metadata and subtitles each declare the egress client's Get and Post as
# the interface their tests stand in for, as consumers own their interfaces.
# api's handlers for a library's collections and an audiobooks library's
# authors and book series read alike, each a page with its letters,
# differing in their generated types only; dupl counts the three twice.
# api's thin handlers that call a service and answer with no body (delete
# an API key, a session, a download, remove from continue watching, run a
# task) read alike in two groups, and so do its get and rename handlers
# for a book series and a person, each with its own generated request and
# response types, which no shared helper would make clearer. A list's
# writes, renaming and deleting one and adding or removing a work, read
# alike the same way.
DUPL_MAX         := 29
# Go functions with a cyclomatic complexity over 15 (gocyclo).
GOCYCLO_MAX      := 45
# Go functions with a cognitive complexity over 20 (gocognit). identify's
# search is one of them at 21: its searches in turn, then the lookup.
GOCOGNIT_MAX     := 53
# api handlers, the operations' and the media routes', with a cognitive
# complexity over 15 (gocognit). A handler parses its request in place,
# calls the service that owns the answer and converts it: a few optional
# fields, a validation and an error check each, which fits in 15 without
# moving its parsing away from it. Past that, it's doing the use case's
# logic, which belongs in a service.
API_HANDLERS_MAX := 0
# Lines in clones of 50 tokens or more (jscpd): the Go code, the SQL queries
# and the web client. The Go count holds the same per-feature copies as
# dupl's, kept alike on purpose, with the start of an item that reads its
# file and how it settled, and the prune that forgets lost results, and
# the SQL count their result rows' queries.
# The Go count also holds query's Browse and nameList, which check the
# library and read its letters alike.
# The Go count holds a list's writes, which read alike (see DUPL_MAX).
# The web count holds General and Network settings' identical imports, and
# Edit and Edit book's forms, which open alike with the title field.
JSCPD_GO_MAX     := 222
JSCPD_SQL_MAX    := 10
JSCPD_WEB_MAX    := 96
# The web client's unused files, dependencies, exports and types (knip).
KNIP_MAX         := 0
# Import cycles in the web client (madge). The player's engine loads the
# TV's AVPlay engine with a dynamic import, to keep it out of the web
# bundle, and avplay.ts implements engine.ts's Engine, so the two import
# each other, though not as the page loads.
MADGE_CYCLES_MAX := 1
# The apps' calls on the generated API or the HTTP client, which belong in
# the core's session.
APP_API_CALLS_MAX := 0
# The apps building what the core's session builds: its connection, live
# events, repositories, sign-in, discovery, stores and player controller.
APP_WIRING_MAX    := 0
# Server URLs the apps build themselves rather than take from the core.
APP_API_URLS_MAX  := 2
# Copies of the "no answer" rule (a 502 to 504 without a problem is the
# server unreached) outside the core's RequestProblem.
UNANSWERED_MAX    := 0
# Swift's references to Kotlin file classes (XxxKt.), which a file rename
# breaks.
SWIFT_KT_MAX      := 0
# Waits in the apps' video players: the core's VideoPlayer runs the player's
# timed rules (Skip, up next, autoplay), and these are the screens' own: the
# controls' hiding on each app, the TV's scrub commit, the engine's position
# polling, the phone's turn to landscape and its double-tap ripple, and the
# Android media session's short hold on a player between a play's players.
APP_PLAYER_TIMERS_MAX := 7
# Numbers the apps format themselves: the core's Display words bitrates,
# sizes, runtimes, time left, clocks, episode codes, quality choices and
# problems, and these are the rest: counts, ratings and speeds.
APP_FORMATS_MAX       := 5
