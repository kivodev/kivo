package metadata

import (
	"errors"
	"testing"
)

func TestAniDB(t *testing.T) {
	e := newEnv(t)
	for _, tc := range []struct {
		id   string
		want AniDB
	}{
		{"69", AniDB{TVDB: "81797", Absolute: true}},
		{"3651", AniDB{TMDbSeries: "30983", TVDB: "79604", IMDb: "tt0988824", Season: 1}},
		{"10376", AniDB{TVDB: "267440", Season: 2, Offset: 12}},
		{"5975", AniDB{TMDbMovie: "12429", IMDb: "tt0876563", Season: 1}},
	} {
		got, err := e.s.AniDB(ctx, tc.id)
		if err != nil || got != tc.want {
			t.Errorf("anidb %s: got %+v, %v; want %+v", tc.id, got, err, tc.want)
		}
	}
	for _, id := range []string{"99999", "1"} {
		if _, err := e.s.AniDB(ctx, id); !errors.Is(err, ErrNotFound) {
			t.Errorf("anidb %s: got %v, want ErrNotFound", id, err)
		}
	}
	if n := e.fx.count("/Anime-Lists/anime-lists/master/anime-list-master.xml"); n != 1 {
		t.Errorf("fetched the mapping %d times, want once", n)
	}
}
