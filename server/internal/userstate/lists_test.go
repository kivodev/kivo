package userstate

import (
	"errors"
	"slices"
	"testing"

	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/userstate/dbq"
)

// placesAfter writes the moved works' places alone while there is room
// between their new neighbours, and every work's when there is none.
func TestPlacesAfter(t *testing.T) {
	a, b, c, d, gone := id.ID{1}, id.ID{2}, id.ID{3}, id.ID{4}, id.ID{9}
	row := func(w id.ID, at int64) dbq.ListPlacesRow { return dbq.ListPlacesRow{WorkID: w, Position: at} }
	spaced := []dbq.ListPlacesRow{row(a, listGap), row(b, 2*listGap), row(c, 3*listGap), row(d, 4*listGap)}
	tight := []dbq.ListPlacesRow{row(a, 1), row(b, 2), row(c, 3), row(d, 4)}
	for _, tt := range []struct {
		name  string
		rows  []dbq.ListPlacesRow
		works []id.ID
		after id.ID
		want  []dbq.ListPlacesRow
	}{
		{"first", spaced, []id.ID{c}, id.ID{}, []dbq.ListPlacesRow{row(c, 0)}},
		{"two first, in the order given", spaced, []id.ID{d, b}, id.ID{}, []dbq.ListPlacesRow{row(d, -listGap), row(b, 0)}},
		{"last", spaced, []id.ID{a}, d, []dbq.ListPlacesRow{row(a, 5*listGap)}},
		{"between two", spaced, []id.ID{d}, a, []dbq.ListPlacesRow{row(d, listGap+listGap/2)}},
		{"two between two", spaced, []id.ID{d, c}, a, []dbq.ListPlacesRow{row(d, listGap+listGap/3), row(c, listGap+2*(listGap/3))}},
		{"where it is", spaced, []id.ID{b}, a, []dbq.ListPlacesRow{row(b, 2*listGap)}},
		{"a work not in the list", spaced, []id.ID{gone, c, c}, a, []dbq.ListPlacesRow{row(c, listGap+listGap/2)}},
		{"none in the list", spaced, []id.ID{gone}, a, nil},
		{"the only work", spaced[:1], []id.ID{a}, id.ID{}, []dbq.ListPlacesRow{row(a, listGap)}},
		{"no room between", tight, []id.ID{d}, a, []dbq.ListPlacesRow{row(a, listGap), row(d, 2*listGap), row(b, 3*listGap), row(c, 4*listGap)}},
		{"room at the top", tight, []id.ID{d}, id.ID{}, []dbq.ListPlacesRow{row(d, 1-listGap)}},
	} {
		got, err := placesAfter(slices.Clone(tt.rows), tt.works, tt.after)
		if err != nil || !slices.Equal(got, tt.want) {
			t.Errorf("%s: %v, %v; want %v", tt.name, got, err, tt.want)
		}
	}
	for name, tt := range map[string]struct {
		works []id.ID
		after id.ID
	}{"after a work not in the list": {[]id.ID{a}, gone}, "after one of the works": {[]id.ID{a, b}, b}} {
		if _, err := placesAfter(spaced, tt.works, tt.after); !errors.Is(err, ErrListAnchor) {
			t.Errorf("%s: %v", name, err)
		}
	}
}
