package version

import (
	"cmp"
	"fmt"
	"strconv"
	"strings"
)

// latest is the latest release, which release-please raises as it tags one.
const latest = "0.3.0" // x-release-please-version

// Of returns the version a build reports. set is the version a release
// build is given, reported as it is. Without one it's the patch after the
// latest release as a dev prerelease, with commit, the build's short commit
// hash, as build metadata when there is one: 0.3.1-dev+56c0018d.
func Of(set, commit string) string {
	if set != "" {
		return set
	}
	v, _ := Parse(latest)
	out := fmt.Sprintf("%d.%d.%d-dev", v.num[0], v.num[1], v.num[2]+1)
	if commit != "" && identifiers(commit) != nil {
		out += "+" + commit
	}
	return out
}

// Version is a parsed semantic version.
type Version struct {
	num [3]int   // major, minor and patch
	pre []string // the prerelease's identifiers, none for a release
}

// Parse reads a semantic version, such as 0.4.0, 0.3.1-dev+56c0018d or
// 0.3.0+78, with or without a leading v. Its build metadata, after the +,
// plays no part in comparing.
func Parse(s string) (Version, bool) {
	s = strings.TrimPrefix(s, "v")
	s, build, hasBuild := strings.Cut(s, "+")
	if hasBuild && identifiers(build) == nil {
		return Version{}, false
	}
	core, pre, hasPre := strings.Cut(s, "-")
	var v Version
	if hasPre {
		if v.pre = identifiers(pre); v.pre == nil {
			return Version{}, false
		}
	}
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return Version{}, false
	}
	for i, p := range parts {
		n, ok := number(p)
		if !ok {
			return Version{}, false
		}
		v.num[i] = n
	}
	return v, true
}

// Compare returns -1, 0 or +1 as v sorts before, with or after w: by
// major, minor and patch, then a prerelease before its release, and two
// prereleases by their identifiers in turn, numbers below words.
func (v Version) Compare(w Version) int {
	for i := range v.num {
		if c := cmp.Compare(v.num[i], w.num[i]); c != 0 {
			return c
		}
	}
	switch {
	case len(v.pre) == 0 && len(w.pre) == 0:
		return 0
	case len(v.pre) == 0:
		return 1
	case len(w.pre) == 0:
		return -1
	}
	for i := range min(len(v.pre), len(w.pre)) {
		a, aNum := number(v.pre[i])
		b, bNum := number(w.pre[i])
		var c int
		switch {
		case aNum && bNum:
			c = cmp.Compare(a, b)
		case aNum:
			c = -1
		case bNum:
			c = 1
		default:
			c = strings.Compare(v.pre[i], w.pre[i])
		}
		if c != 0 {
			return c
		}
	}
	return cmp.Compare(len(v.pre), len(w.pre))
}

// Newer reports whether version a is newer than b. A version Parse can't
// read is never newer, nor older.
func Newer(a, b string) bool {
	x, ok1 := Parse(a)
	y, ok2 := Parse(b)
	return ok1 && ok2 && x.Compare(y) > 0
}

// identifiers splits s into its dot-separated identifiers, each one or
// more ASCII letters, digits and hyphens; nil if one isn't.
func identifiers(s string) []string {
	ids := strings.Split(s, ".")
	for _, id := range ids {
		if id == "" || strings.Trim(id, "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz-") != "" {
			return nil
		}
	}
	return ids
}

// number reads s as a whole number of digits only, without a leading zero.
func number(s string) (int, bool) {
	if s == "" || strings.Trim(s, "0123456789") != "" || (len(s) > 1 && s[0] == '0') {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	return n, err == nil
}
