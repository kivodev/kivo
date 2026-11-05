package identify

import (
	"cmp"
	"context"
	"path"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/metadata"
)

// classifyBooks makes a book's folder one book. Every audio file in it,
// including those in CD and disc folders, is a part of the book's one
// version. A file directly in the library folder is a book of its own.
// Books are identified from their folders and tags alone.
func classifyBooks(folder string, files []file) []*unit {
	if len(files) == 0 {
		return nil
	}
	parts := bookOrder(files)
	subject := folder
	if folder == "" {
		subject = "|" + parts[0].Rel
	}
	return []*unit{{
		subject: subject, kind: catalog.KindBook, title: metadata.BookTitle(parts[0].Rel, parts[0].tags),
		versions: []*entry{{parts: parts}},
	}}
}

// bookOrder puts a book's files in playing order. It sorts by disc (a CD or
// disc folder, else the disc tag), then by track, then by name, comparing
// numbers in names as numbers. Track tags count only when every file has one
// and no disc and track pair repeats. Otherwise names decide.
func bookOrder(files []file) []file {
	type key struct{ disc, track int }
	keys := make(map[string]key, len(files))
	seen := map[key]bool{}
	tracks := true
	for _, f := range files {
		k := key{disc: cmp.Or(metadata.Disc(f.Rel), leadingInt(f.tags["disc"])), track: leadingInt(f.tags["track"])}
		keys[f.Rel] = k
		tracks = tracks && k.track > 0 && !seen[k]
		seen[k] = true
	}
	out := slices.Clone(files)
	slices.SortStableFunc(out, func(a, b file) int {
		ka, kb := keys[a.Rel], keys[b.Rel]
		if c := cmp.Compare(ka.disc, kb.disc); c != 0 {
			return c
		}
		if tracks {
			if c := cmp.Compare(ka.track, kb.track); c != 0 {
				return c
			}
		}
		return natural(path.Base(a.Rel), path.Base(b.Rel))
	})
	return out
}

// leadingInt returns the number a tag starts with, such as 2 for a track of
// "2/35".
func leadingInt(s string) int {
	end := strings.IndexFunc(s, func(r rune) bool { return !unicode.IsDigit(r) })
	if end < 0 {
		end = len(s)
	}
	n, _ := strconv.Atoi(s[:end])
	return n
}

// natural compares names with their runs of digits as numbers, so
// "Part 2" comes before "Part 10".
func natural(a, b string) int {
	for a != "" && b != "" {
		da, db := digits(a), digits(b)
		if da > 0 && db > 0 {
			na, nb := strings.TrimLeft(a[:da], "0"), strings.TrimLeft(b[:db], "0")
			if c := cmp.Or(cmp.Compare(len(na), len(nb)), strings.Compare(na, nb)); c != 0 {
				return c
			}
			a, b = a[da:], b[db:]
			continue
		}
		ra, wa := utf8.DecodeRuneInString(a)
		rb, wb := utf8.DecodeRuneInString(b)
		if c := cmp.Compare(unicode.ToLower(ra), unicode.ToLower(rb)); c != 0 {
			return c
		}
		a, b = a[wa:], b[wb:]
	}
	return cmp.Compare(len(a), len(b))
}

func digits(s string) int {
	n := 0
	for n < len(s) && s[n] >= '0' && s[n] <= '9' {
		n++
	}
	return n
}

// describeBook queues the refresh that describes a book from its folders,
// tags and cover. It runs whenever the book's folder is identified, because
// its files, their tags or the images beside them may have changed.
func (i *Identifier) describeBook(ctx context.Context, book id.ID) error {
	return i.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		return i.deps.Metadata.Refresh(ctx, tx, book)
	})
}
