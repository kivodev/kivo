// Package identify decides which work each version is. It decides from
// folder and file names, NFO files and provider searches.
//
// Identify runs as a library's scan ends. It takes one folder at a time,
// classifies the folder's files into units, and resolves each unit.
// FixMatch, Unmatch and EditWork's ordering re-identify a root work on an
// admin's request.
//
// The package writes only its match decisions. Works, versions and watch
// state change through catalog and userstate. Its queries read catalog's
// tables directly, to get the placements and file states it decides from.
package identify
