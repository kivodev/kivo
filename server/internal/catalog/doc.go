// Package catalog holds what exists: libraries and their folders, files and
// their tracks, versions, works, people, genres, collections, book series
// and external IDs, along with the search index over them.
//
// It calls no providers and starts no other package's work. Its only
// background work is its own task, which removes folders and deletes
// libraries. OpenFolder is its only file system access.
package catalog
