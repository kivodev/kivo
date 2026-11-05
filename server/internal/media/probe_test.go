package media

import "testing"

func TestSpecLevel(t *testing.T) {
	for _, c := range []struct {
		codec Codec
		level int
		want  float64
	}{
		{H264, 41, 4.1}, {H264, 52, 5.2}, {HEVC, 153, 5.1}, {HEVC, 120, 4}, {HEVC, 93, 3.1},
		{"av1", 8, 4}, {"av1", 13, 5.1}, {"vp9", 40, 4}, {"mpeg2video", 8, 0}, {H264, 0, 0},
	} {
		if got := SpecLevel(c.codec, c.level); got != c.want {
			t.Errorf("%s level %d: %v, want %v", c.codec, c.level, got, c.want)
		}
	}
}

func TestIsCommentary(t *testing.T) {
	for title, want := range map[string]bool{
		"Commentary by director Ron Underwood": true,
		"Director's commentary":                true,
		"Commentaries":                         false,
		"E-AC3 DD+5.1 1024kbs":                 false,
	} {
		if got := (Track{Title: title}).IsCommentary(); got != want {
			t.Errorf("IsCommentary(%q) = %v, want %v", title, got, want)
		}
	}
	if !(Track{Commentary: true}).IsCommentary() {
		t.Error("a track the file marks as commentary isn't one")
	}
}
