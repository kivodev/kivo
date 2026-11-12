package version

import "testing"

// TestMinApp checks that the oldest app the server supports is a release,
// and one that exists: no newer than the latest.
func TestMinApp(t *testing.T) {
	v, ok := Parse(MinApp)
	if !ok || v.pre {
		t.Fatalf("MinApp %q isn't a release's major.minor.patch", MinApp)
	}
	if Newer(MinApp, latest) {
		t.Errorf("MinApp %s is newer than the latest release %s", MinApp, latest)
	}
}
