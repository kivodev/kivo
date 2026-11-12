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

// semver is a parsed semantic version.
type semver struct {
	num [3]int // major, minor and patch
	pre bool   // a prerelease
}

// Parse reads a semantic version, such as 0.4.0, 0.3.1-dev+56c0018d or
// 0.3.0+78, with or without a leading v. Its build metadata, after the +,
// plays no part in comparing.
func Parse(s string) (semver, bool) {
	s = strings.TrimPrefix(s, "v")
	s, build, hasBuild := strings.Cut(s, "+")
	if hasBuild && identifiers(build) == nil {
		return semver{}, false
	}
	core, pre, hasPre := strings.Cut(s, "-")
	if hasPre && identifiers(pre) == nil {
		return semver{}, false
	}
	v := semver{pre: hasPre}
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return semver{}, false
	}
	for i, p := range parts {
		n, ok := number(p)
		if !ok {
			return semver{}, false
		}
		v.num[i] = n
	}
	return v, true
}

// compare returns -1, 0 or +1 as v sorts before, with or after w: by
// major, minor and patch, then a prerelease before its release.
func (v semver) compare(w semver) int {
	for i := range v.num {
		if c := cmp.Compare(v.num[i], w.num[i]); c != 0 {
			return c
		}
	}
	switch {
	case v.pre == w.pre:
		return 0
	case v.pre:
		return -1
	}
	return 1
}

// Newer reports whether version a is newer than b. A version Parse can't
// read is never newer, nor older.
func Newer(a, b string) bool {
	x, ok1 := Parse(a)
	y, ok2 := Parse(b)
	return ok1 && ok2 && x.compare(y) > 0
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
