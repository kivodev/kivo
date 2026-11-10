// Package metadata describes works and keeps their artwork.
//
// It fetches provider data (TMDb, Audible and Audnexus, and the Anime-Lists
// mapping) through the egress client. It also reads the NFO files and
// artwork next to the media, and a book's folders and tags. It normalizes
// all of this into catalog, and stores re-encoded images of works and of the
// people they credit.
//
// It never changes which work a file belongs to. The identify package
// decides that, using the searches here and BookTitle.
package metadata
