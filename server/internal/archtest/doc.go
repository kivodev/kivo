// Package archtest enforces the architecture's boundaries. It checks that:
//   - no component imports one in a higher layer, and no component cycle
//     forms;
//   - only egress calls out;
//   - pure packages stay pure;
//   - each component writes only the tables and columns it owns;
//   - every exported sentinel error is a fault, and error messages follow
//     the rules;
//   - no deferred closure wraps an error;
//   - api fires no side effect of a change itself;
//   - only store begins, commits or rolls back a transaction;
//   - query uses only catalog's values, never anything of catalog that
//     reaches the database.
//
// The checks run as ordinary tests, so `go test ./...` fails on a violation.
package archtest
