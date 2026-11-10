// Package playback decides how each track of a play is delivered, runs
// plays, and authorizes the media requests they make. It also decides
// downloads, prepares them as jobs, and serves them. It never writes
// user_state directly. Service runs the plays and Downloads the downloads,
// and both plan the same way.
package playback
