// Package archtest enforces the architecture's boundaries. It checks that:
//   - no component imports one in a higher layer, and no component cycle
//     forms;
//   - api and query reach other components only through their root
//     packages;
//   - only egress calls out;
//   - pure packages stay pure;
//   - each component writes only the tables and columns it owns;
//   - only store begins, commits or rolls back a transaction.
//
// The checks run as ordinary tests, so `go test ./...` fails on a violation.
package archtest
