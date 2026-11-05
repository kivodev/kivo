package identify

import (
	"testing"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/metadata"
)

func TestSearchText(t *testing.T) {
	for _, c := range []struct {
		in      metadata.Query
		want    metadata.Query
		country string
	}{
		{metadata.Query{Title: "Cash Cab (US)"}, metadata.Query{Title: "Cash Cab"}, "US"},
		{metadata.Query{Title: "Heat (1995)"}, metadata.Query{Title: "Heat", Year: 1995}, ""},
		{metadata.Query{Title: "The Office (UK) (2001)"}, metadata.Query{Title: "The Office", Year: 2001}, "UK"},
		{metadata.Query{Title: "tmdb-438631"}, metadata.Query{Title: "tmdb-438631"}, ""},
		{metadata.Query{Title: "Dune", Year: 2021}, metadata.Query{Title: "Dune", Year: 2021}, ""},
	} {
		got, country := searchText(c.in)
		if got != c.want || country != c.country {
			t.Errorf("searchText(%+v) = %+v, %q; want %+v, %q", c.in, got, country, c.want, c.country)
		}
	}
}

func TestMadeInFirst(t *testing.T) {
	office := func(id, country string) metadata.Match {
		return metadata.Match{ID: catalog.ExternalID{Provider: "tmdb", Kind: "series", ID: id}, Countries: []string{country}}
	}
	cands := []metadata.Match{office("1", "GB"), office("2", "US"), office("3", "GB")}
	madeInFirst(cands, "US")
	if cands[0].ID.ID != "2" || cands[1].ID.ID != "1" || cands[2].ID.ID != "3" {
		t.Errorf("US first, the rest in order: %v", cands)
	}
	madeInFirst(cands, "UK")
	if cands[0].ID.ID != "1" || cands[1].ID.ID != "3" {
		t.Errorf("UK as GB: %v", cands)
	}
}
