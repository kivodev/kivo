// Package detect decides episodes' intro and credits markers one season at
// a time, from the season's files' fingerprints. An intro is the opening
// the season's episodes share. Credits are the ending they share, or the
// text on black that ends an episode.
//
// The package reads no media itself. The markers package fingerprints the
// files, keeps their fingerprints, and stores what Decide decides.
package detect

import (
	"time"

	"github.com/mmcdole/kivo/internal/id"
)

// The kinds of marker detection finds.
const (
	Intro   = "intro"
	Credits = "credits"
)

// RulesVersion is the version of the rules that decide markers from
// fingerprints. Bump it when the rules decide differently, so every season
// is decided again from its fingerprints.
const RulesVersion = 1

// These constants place a fingerprint match on the file's timeline.
//
// A fingerprint point stands for the audio of a few frames from its time
// on. So a shared stretch matches from a little before it starts until a
// little before it ends. The leads shift both ends by amounts measured
// against episodes whose chapters mark their intros.
//
// An intro that starts within zeroSnap of the start of the file is moved
// to start at 0. A boundary within snapWithin of a chapter's start, or of
// the edge of a silence, moves to it.
const (
	introStartLead   = 850 * time.Millisecond
	introEndLead     = 1300 * time.Millisecond
	creditsStartLead = 400 * time.Millisecond
	creditsEndLead   = 1300 * time.Millisecond
	zeroSnap         = 2 * time.Second
	snapWithin       = 2 * time.Second
)

// introLimits and creditsLimits bound the length of a run of audio that
// episodes share. introLimits applies to an intro, and creditsLimits to
// the audio of ending credits.
var (
	introLimits   = matchLimits{min: 12 * time.Second, max: 180 * time.Second}
	creditsLimits = matchLimits{min: minCredits, max: maxCredits}
)

// File is an episode's file, as Decide sees it.
type File struct {
	ID id.ID
	// Episode is the episode's place among the season's episodes that have
	// files, counting from 0. All versions of an episode share it.
	Episode     int
	Part, Parts int // the file's part number in its version, and the version's number of parts
	Duration    time.Duration
	// Chapters are where the file's chapters start.
	Chapters []time.Duration
	// Fingerprint is the file's fingerprint. It is nil until the file is
	// fingerprinted.
	Fingerprint *Fingerprint
}

// Marker is a file's marker of one kind: its span, whether credits are
// final, and a short reason for the decision.
type Marker struct {
	Kind       string
	Start, End time.Duration
	Final      bool
	Reason     string
}

// guess is a file's marker of one kind, as a rule finds it.
type guess struct {
	span
	final  bool
	reason string
}

func (g guess) marker(kind string) Marker {
	return Marker{Kind: kind, Start: g.start, End: g.end, Final: g.final, Reason: g.reason}
}

// Decide decides each file's markers from its season's files'
// fingerprints.
func Decide(files []File) map[id.ID][]Marker {
	intros, ends := season(prints(files, func(fp *Fingerprint) *Window { return fp.Head }), introLimits), endingOf(files)
	out := map[id.ID][]Marker{}
	for _, f := range files {
		if g := f.intro(intros); g != nil {
			out[f.ID] = append(out[f.ID], g.marker(Intro))
		}
		if g := f.credits(ends); g != nil {
			out[f.ID] = append(out[f.ID], g.marker(Credits))
		}
	}
	return out
}

// prints returns the season's fingerprints of one window.
func prints(files []File, of func(*Fingerprint) *Window) []print {
	var out []print
	for _, f := range files {
		if f.Fingerprint == nil {
			continue
		}
		if w := of(f.Fingerprint); w != nil && len(w.Points) > 0 {
			out = append(out, print{file: f.ID, episode: f.Episode, points: w.points()})
		}
	}
	return out
}
