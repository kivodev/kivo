package identify

import (
	"slices"
	"testing"

	"github.com/mmcdole/kivo/internal/parse"
)

func TestParseNFO(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       []parse.ExternalID
	}{
		{"uniqueid", `<movie><uniqueid type="tmdb" default="true">438631</uniqueid><uniqueid type="imdb">tt1160419</uniqueid></movie>`,
			[]parse.ExternalID{{Provider: "tmdb", ID: "438631"}, {Provider: "imdb", ID: "tt1160419"}}},
		{"old tags", `<tvshow><tvdbid>78874</tvdbid><id>tt0303461</id></tvshow>`,
			[]parse.ExternalID{{Provider: "tvdb", ID: "78874"}, {Provider: "imdb", ID: "tt0303461"}}},
		{"url only", "https://www.themoviedb.org/movie/949-heat\n", []parse.ExternalID{{Provider: "tmdb", ID: "949"}}},
		{"bad values", `<movie><uniqueid type="tmdb">abc</uniqueid><uniqueid type="imdb">123</uniqueid><uniqueid type="other">1</uniqueid></movie>`, nil},
		{"entity", `<!DOCTYPE m [<!ENTITY x "1">]><movie><tmdbid>&x;</tmdbid></movie>`, nil},
	} {
		if got := parseNFO([]byte(tc.body)); !slices.Equal(got, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}
