// Package previews makes each file's preview thumbnails. They are sheets of
// scrub previews and a manifest naming them, made ahead of time as each
// library's setting says. Making a file's previews is a task item keyed by
// the file. The previews live in cache/previews/ under the file's ID and
// generation, so a move keeps them and a changed file never reads stale
// ones. A recorded result is only a hint, and the result on disk is the
// truth. The package never serves HTTP. api and playback serve what it
// returns.
package previews
