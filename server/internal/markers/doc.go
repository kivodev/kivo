// Package markers finds episodes' intro and credits markers, and gives a
// play its chapters and markers.
//
// The fingerprint-markers job reads each episode file's audio and pictures
// into a fingerprint. Fingerprints are kept in cache/markers/ under the
// file's ID and generation. The detect-markers job stores in media_marker
// what the detect package decides from a season's fingerprints.
//
// A play's markers come from three layers. An admin's markers win over the
// stored ones, and stored markers win over the ones the chapters give.
//
// The package never serves HTTP. The api and playback packages serve what
// it returns.
package markers
