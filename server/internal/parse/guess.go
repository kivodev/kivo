package parse

import (
	"fmt"
	"time"
)

// Kind is a library's declared kind. Parsing depends on it. Only shows look
// for episodes, and other videos take their titles as written.
type Kind string

// The library kinds.
const (
	Movies Kind = "movies"
	Shows  Kind = "shows"
	Videos Kind = "videos"
)

// Date is a calendar date with no time zone. The zero Date means "none".
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

// DateOf returns the calendar date of t in t's location.
func DateOf(t time.Time) Date {
	y, m, d := t.Date()
	return Date{y, m, d}
}

// IsZero reports whether d is the zero Date.
func (d Date) IsZero() bool { return d == Date{} }

// String returns d as YYYY-MM-DD.
func (d Date) String() string { return fmt.Sprintf("%04d-%02d-%02d", d.Year, int(d.Month), d.Day) }

func (d Date) time() time.Time { return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC) }

// ExtraType says what kind of extra a file is. The empty ExtraType means the
// file isn't an extra.
type ExtraType string

// The extra types, named as their file name suffixes are.
const (
	Trailer         ExtraType = "trailer"
	Featurette      ExtraType = "featurette"
	BehindTheScenes ExtraType = "behindthescenes"
	DeletedScene    ExtraType = "deleted"
	Interview       ExtraType = "interview"
	Scene           ExtraType = "scene"
	Short           ExtraType = "short"
	OtherExtra      ExtraType = "other"
)

// ExternalID is a provider ID written into a file or folder name, such as
// {tmdb-438631}. Provider is one of tmdb, tvdb, imdb or anidb.
type ExternalID struct {
	Provider, ID string
}

// Part is a multi-file part token such as cd2 or "part b". parse only reads
// the token. Grouping 2 to 8 files that differ only in this token is
// identify's job.
type Part struct {
	Index int    // 1-based; the letters a to d map to 1 to 4
	Token string // the token as written, such as "cd2"
}

// Guess is what a path says about itself. Fields the path doesn't mention
// are zero.
type Guess struct {
	// Title is the cleaned title, in NFKC form. For shows it is the series
	// title. For other videos it is the whole file name.
	Title string
	// Edition comes from an {edition-…} tag.
	Edition string
	Year    int
	// Season comes from the file name, else the nearest season folder, else
	// it is 1. It is always set in a shows library, and 0 means specials.
	Season int
	// Episodes are the episode numbers within Season, strictly increasing.
	Episodes []int
	// Absolute holds absolute episode numbers, such as anime's
	// "Title - 012", strictly increasing. Identify maps them to seasons.
	// When Absolute is set, Season comes from a season folder or is the
	// default 1.
	Absolute []int
	// AirDate is set for date-based episodes, which are looked up by air
	// date, never by number.
	AirDate Date
	// Special marks a fractional episode such as 12.5, which providers list
	// as a special. Absolute or Episodes then hold the whole-number part.
	Special bool
	// SplitPart names the part of a split episode: "a" for S01E05a, or "1"
	// for "S01E05 - part1".
	SplitPart string
	Part      *Part
	// Extra is set for files in an extras folder below the library folder, or
	// named with an extras suffix such as -trailer.
	Extra ExtraType
	// IDs are the provider IDs in the file name. Folder IDs come from Dir.
	IDs []ExternalID
	// Group is an anime release's leading [Group], and Checksum is its
	// 8-hex-digit CRC. Revision is its v2-style version.
	Group, Checksum string
	Revision        int
	// Rule names the rule that decided the episode or title, for diagnostics.
	Rule string
}

// DirGuess is what a folder name says about itself.
type DirGuess struct {
	Title   string
	Edition string
	Year    int
	// Country is a trailing country code that tells same-named shows apart,
	// such as "The Office (US)". It's left out of Title.
	Country string
	IDs     []ExternalID
	// IsSeason is set for season folders, such as "Season 02" or
	// "Specials". Season is then the season number, with 0 for specials.
	IsSeason bool
	Season   int
	// Extra is set when the name is an extras folder name such as Trailers.
	// Whether the folder counts as extras depends on where it sits, which
	// Parse checks and Dir can't.
	Extra ExtraType
}
