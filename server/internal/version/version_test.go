package version

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestOf(t *testing.T) {
	v, _ := Parse(latest)
	next := fmt.Sprintf("%d.%d.%d", v.num[0], v.num[1], v.num[2]+1)
	dev := next + "-dev"
	for _, tc := range []struct{ set, commit, want string }{
		{"0.4.0", "56c0018d", "0.4.0"},
		{"", "56c0018d", dev + "+56c0018d"},
		{"", "", dev},
		{"", "not/a commit", dev},
	} {
		if got := Of(tc.set, tc.commit); got != tc.want {
			t.Errorf("Of(%q, %q) = %q, want %q", tc.set, tc.commit, got, tc.want)
		}
	}
	got := Of("", "56c0018d")
	if !Newer(got, latest) || !Newer(next, got) {
		t.Errorf("%s doesn't sort between %s and %s", got, latest, next)
	}
}

// TestLatestIsTheManifests holds latest to the repository's version, which
// release-please raises in both.
func TestLatestIsTheManifests(t *testing.T) {
	b, err := os.ReadFile("../../../.github/.release-please-manifest.json")
	if err != nil {
		t.Skip("no manifest:", err)
	}
	m := regexp.MustCompile(`"\.":\s*"([^"]+)"`).FindSubmatch(b)
	if m == nil || string(m[1]) != latest {
		t.Errorf("the manifest says %s, latest is %s", m, latest)
	}
}

func TestNewer(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"v0.5.0", "0.4.0", true},
		{"0.4.1", "0.4.0", true},
		{"1.0.0", "0.10.9", true},
		{"0.10.0", "0.9.0", true},
		{"v0.4.0", "0.4.0", false},
		{"0.4.0", "0.5.0", false},
		// A dev build is newer than the release before it, older than the next.
		{"0.3.1-dev+56c0018d", "0.3.0", true},
		{"0.3.0", "0.3.1-dev+56c0018d", false},
		{"0.4.0", "0.3.1-dev+56c0018d", true},
		{"0.3.1", "0.3.1-dev", true},
		// Build metadata, such as an app's build number, doesn't count.
		{"0.3.0+78", "0.3.0+12", false},
		{"0.3.0+12", "0.3.0+78", false},
		{"0.4.0", "0.3.0+78", true},
		// Prereleases, by their identifiers in turn.
		{"1.0.0-alpha.1", "1.0.0-alpha", true},
		{"1.0.0-alpha.beta", "1.0.0-alpha.1", true},
		{"1.0.0-beta.11", "1.0.0-beta.2", true},
		{"1.0.0-rc.1", "1.0.0-beta.11", true},
		{"1.0.0", "1.0.0-rc.1", true},
		// A version it can't read is never newer, nor older.
		{"v0.5.0", "dev", false},
		{"dev", "v0.5.0", false},
		{"v0.5.0", "56c0018d", false},
		{"latest", "0.4.0", false},
		{"0.5", "0.4.0", false},
		{"0.5.0", "0.3.0 (78)", false},
		{"0.5.0", "0.4.0-", false},
		{"0.5.0", "0.4.0+", false},
		{"0.5.0", "0.4.0-dev..1", false},
		{"0.5.0", "0.04.0", false},
		{"0.5.0", "-1.4.0", false},
		{"0.5.0", "", false},
	} {
		if got := Newer(tc.a, tc.b); got != tc.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestParse(t *testing.T) {
	for _, s := range []string{"0.4.0", "v0.4.0", "0.3.1-dev+56c0018d", "0.3.0+78", "1.0.0-rc.1"} {
		if _, ok := Parse(s); !ok {
			t.Errorf("Parse(%q) failed", s)
		}
	}
	for _, s := range []string{"", "dev", "0.4", "0.4.0.1", "0.4.x", "0.4.0-é", strings.Repeat("9", 30) + ".0.0"} {
		if _, ok := Parse(s); ok {
			t.Errorf("Parse(%q) read it", s)
		}
	}
}
