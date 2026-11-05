package identify

import (
	"slices"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/identify/parse"
)

// file is one present media file of a snapshot.
type file struct {
	ID   id.ID
	Rel  string // relative to its folder, '/'-separated
	Size int64
	Gen  int64
	tags map[string]string // as probing read them; nil before probing
}

// entry is a version to be: its parts in order, and what its name says.
type entry struct {
	parts   []file
	guess   parse.Guess // parsed from the first part's name
	edition string      // a movie version's edition label, from its files' tag
	extra   parse.ExtraType
	title   string // an extra's or other video's own title
	season  int    // for a show's extra, its season folder, or -1 for the series
	dir     string // for another video, the folder that makes its collection
}

func (e *entry) ids() []id.ID {
	out := make([]id.ID, len(e.parts))
	for i, p := range e.parts {
		out[i] = p.ID
	}
	return out
}

// unit is what one decision covers, and it becomes one root work. A unit is a
// movie with its versions and extras, a series with its episodes
// and extras, a book, or one other video. A folder holds one unit, or
// several when loose movies share it.
//
// subject is the key the unit's root work is stored by, in its match row
// and its folder link. It is the folder, or for a loose movie the folder
// with the movie's name and year, or the file when there is neither.
type unit struct {
	subject  string
	kind     catalog.WorkKind // KindMovie, KindSeries, KindVideo or KindBook
	title    string
	year     int
	edition  string             // a movie's label, from its folder's edition tag
	country  string             // a country code the folder names, which breaks ties
	alts     []string           // other titles the names suggest, searched after title
	ids      []parse.ExternalID // from names, nearest first
	nfos     []string           // NFO files that may name it, nearest first
	search   bool               // false when nothing in the names is worth searching for
	versions []*entry
	extras   []*entry
}

func (u *unit) entries() []*entry { return append(slices.Clone(u.versions), u.extras...) }

func (u *unit) files() []file {
	var out []file
	for _, e := range u.entries() {
		out = append(out, e.parts...)
	}
	return out
}

func (u *unit) fileIDs() []id.ID {
	files := u.files()
	ids := make([]id.ID, len(files))
	for k, f := range files {
		ids[k] = f.ID
	}
	return ids
}

func (u *unit) has(file id.ID) bool {
	for _, f := range u.files() {
		if f.ID == file {
			return true
		}
	}
	return false
}
