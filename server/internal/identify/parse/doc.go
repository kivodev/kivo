// Package parse reads titles, years, seasons, episodes and IDs from
// library-relative paths, following the common media naming conventions.
//
// It is pure: it has no I/O, no clock and no mutable state, so the same
// path, kind and date always give the same Guess. Every input returns a
// Guess, and nothing panics.
//
// Go's RE2 engine has no lookaround or backreferences, so a rule that needs
// them captures the neighbouring text and checks it in its extract function.
package parse
