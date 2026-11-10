// Package api exposes HTTP and WebSocket, enforces authentication and access,
// and aggregates health. It runs no SQL and holds no domain rules, though a
// handler may combine reads from several services into one response.
package api
