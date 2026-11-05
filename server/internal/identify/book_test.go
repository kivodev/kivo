package identify

import (
	"reflect"
	"testing"
)

func TestBookOrder(t *testing.T) {
	names := func(fs []file) []string {
		var out []string
		for _, f := range fs {
			out = append(out, f.Rel)
		}
		return out
	}
	tagged := func(rel, disc, track string) file {
		return file{Rel: rel, tags: map[string]string{"disc": disc, "track": track}}
	}
	for _, tc := range []struct {
		name  string
		files []file
		want  []string
	}{
		{"tracks decide when every file has a distinct one",
			[]file{tagged("b/a.mp3", "", "2/3"), tagged("b/c.mp3", "", "1/3"), tagged("b/b.mp3", "", "3/3")},
			[]string{"b/c.mp3", "b/a.mp3", "b/b.mp3"}},
		{"disc tags, then tracks",
			[]file{tagged("b/x.mp3", "2/2", "1"), tagged("b/y.mp3", "1/2", "2"), tagged("b/z.mp3", "1/2", "1")},
			[]string{"b/z.mp3", "b/y.mp3", "b/x.mp3"}},
		{"a repeated track: names decide, numbers as numbers",
			[]file{tagged("b/Part 10.mp3", "", "1"), tagged("b/Part 2.mp3", "", "1"), tagged("b/part 1.mp3", "", "")},
			[]string{"b/part 1.mp3", "b/Part 2.mp3", "b/Part 10.mp3"}},
		{"disc folders first",
			[]file{{Rel: "b/CD2/01.mp3"}, {Rel: "b/CD10/01.mp3"}, {Rel: "b/CD1/02.mp3"}, {Rel: "b/CD1/01.mp3"}},
			[]string{"b/CD1/01.mp3", "b/CD1/02.mp3", "b/CD2/01.mp3", "b/CD10/01.mp3"}},
		{"a gap in the numbers",
			[]file{{Rel: "b/Heir of Novron 03 of 32.mp3"}, {Rel: "b/Heir of Novron 01 of 32.mp3"}},
			[]string{"b/Heir of Novron 01 of 32.mp3", "b/Heir of Novron 03 of 32.mp3"}},
	} {
		if got := names(bookOrder(tc.files)); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
}
