package parse

import (
	"path"
	"regexp"
	"strconv"
	"strings"
)

// discFolder matches a CD or disc folder. Its files belong to the book in
// the folder above it.
var discFolder = regexp.MustCompile(`(?i)^(?:cd|dis[ck])\s*\d{1,3}$`)

// BookFolder returns the folder a book's file belongs to. That is the folder
// the file is in, or the one above it when the file is in a CD or disc
// folder. It returns "" for a file directly in the library folder, which is
// a book of its own.
func BookFolder(rel string) string {
	dir := path.Dir(rel)
	if discFolder.MatchString(path.Base(dir)) {
		dir = path.Dir(dir)
	}
	if dir == "." {
		return ""
	}
	return dir
}

// Disc returns the number of the disc folder a book's file is in, or zero.
func Disc(rel string) int {
	d := path.Base(path.Dir(rel))
	if !discFolder.MatchString(d) {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimLeft(d, "cdiskCDISK \t"))
	return n
}
