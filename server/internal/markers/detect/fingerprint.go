package detect

import (
	"time"

	"github.com/mmcdole/kivo/internal/media"
)

// FingerprintVersion is the version of what a file's fingerprint keeps.
// Bump it when the way markers fingerprints a file changes: how it reads,
// the head's and tail's lengths, or which audio track it takes. Bump
// RulesVersion with it, so every file is fingerprinted and decided again.
const FingerprintVersion = 1

// Fingerprint is what fingerprinting a file found, as its result keeps it.
// Head is set if the file is the first part of its version, and Tail if it
// is the last. Both are nil when the file has no audio to fingerprint.
type Fingerprint struct {
	Head *Window `json:"head,omitempty"`
	Tail *Window `json:"tail,omitempty"`
}

// Window is one stretch of a file as fingerprinted.
type Window struct {
	Start     time.Duration         `json:"start"`
	Points    []byte                `json:"points"` // the fingerprint's raw points, little-endian
	Silences  []media.Span          `json:"silences,omitempty"`
	Keyframes []media.KeyframeStats `json:"frames,omitempty"`
}

func (w *Window) points() []uint32 {
	if w == nil {
		return nil
	}
	out := make([]uint32, len(w.Points)/4)
	for i := range out {
		b := w.Points[4*i:]
		out[i] = uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
	}
	return out
}

// PackPoints packs a fingerprint's points in the form a Window keeps them.
func PackPoints(points []uint32) []byte {
	out := make([]byte, 0, 4*len(points))
	for _, p := range points {
		out = append(out, byte(p), byte(p>>8), byte(p>>16), byte(p>>24))
	}
	return out
}
