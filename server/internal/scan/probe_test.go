package scan

import "testing"

func TestTitleFlags(t *testing.T) {
	for _, c := range []struct {
		title       string
		forced, sdh bool
	}{
		{"English (Forced)", true, false},
		{"Foreign Parts", true, false},
		{"English [SDH]", false, true},
		{"English CC", false, true},
		{"Hearing Impaired", false, true},
		{"English Lyrics/Signs", false, false},
		{"Enforcedness", false, false},
		{"", false, false},
	} {
		got := titleFlags(c.title)
		if got.forced != c.forced || got.sdh != c.sdh {
			t.Errorf("%q: forced %v sdh %v, want %v %v", c.title, got.forced, got.sdh, c.forced, c.sdh)
		}
	}
}
