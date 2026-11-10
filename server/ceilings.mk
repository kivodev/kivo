# The ceilings make ceilings holds the code-health counts to. A count may fall
# but not rise; when one falls, lower its ceiling here to match.

# Functions nothing reachable from cmd/kivo calls (deadcode).
DEADCODE_MAX     := 0
# Groups of Go clones of 50 tokens or more (dupl -t 50).
DUPL_MAX         := 26
# Go functions with a cognitive complexity over 20 (gocognit).
GOCOGNIT_MAX     := 53
# api handlers, the operations' and the media routes', with a cognitive
# complexity over 15 (gocognit): past that, the logic belongs in a service.
API_HANDLERS_MAX := 0
# Lines in clones of 50 tokens or more (jscpd): the SQL queries and the web client.
JSCPD_SQL_MAX    := 10
JSCPD_WEB_MAX    := 110
# The web client's unused files, dependencies, exports and types (knip).
KNIP_MAX         := 0
# Import cycles in the web client (madge). The player's engine and avplay.ts
# import each other, avplay.ts through a dynamic import kept out of the web bundle.
MADGE_CYCLES_MAX := 1
# The apps' calls on the generated API or the HTTP client, which belong in the core.
APP_API_CALLS_MAX := 0
# The apps building what the core's session builds (its constructors).
APP_WIRING_MAX    := 0
# Server URLs the apps build themselves rather than take from the core.
APP_API_URLS_MAX  := 2
# Copies of the core's "no answer" rule (502 to 504) outside RequestProblem.
UNANSWERED_MAX    := 0
# Swift's references to Kotlin file classes (XxxKt.), which a file rename breaks.
SWIFT_KT_MAX      := 0
# Waits in the apps' video players; the core's VideoPlayer runs the timed rules.
APP_PLAYER_TIMERS_MAX := 7
# Numbers the apps format themselves rather than through the core's Display.
APP_FORMATS_MAX       := 5
