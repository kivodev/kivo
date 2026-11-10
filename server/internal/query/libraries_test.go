package query_test

import (
	"context"
	"testing"

	"github.com/mmcdole/kivo/internal/query"
	"github.com/mmcdole/kivo/internal/users"
)

// TestLibraryCounts checks that each library's item and unmatched counts
// match its grid's totals, for every user of the household. It also checks
// that AdminLibraries matches Libraries' order and counts.
func TestLibraryCounts(t *testing.T) {
	h := newHousehold(t)
	ctx := context.Background()
	viewers := map[string]users.Viewer{
		"admin": h.admin, "kid": h.kid, "friend": h.friend, "nobody": h.nobody,
		"no unrated": h.viewer("strict", users.NewUser{Parental: users.Parental{BlockUnrated: true}}),
	}
	for name, v := range viewers {
		libs, err := h.q.Libraries(ctx, v)
		if err != nil {
			t.Fatal(err)
		}
		admin, err := h.q.AdminLibraries(ctx, v)
		if err != nil || len(admin) != len(libs) {
			t.Fatalf("%s: admin libraries %d, %v; want %d", name, len(admin), err, len(libs))
		}
		for i, l := range libs {
			all, err := h.browse(v, l.ID, grid{GridOptions: query.GridOptions{Limit: 1}})
			if err != nil {
				t.Fatal(err)
			}
			unmatched, err := h.browse(v, l.ID, grid{GridOptions: query.GridOptions{Limit: 1}, Filter: filter{Unmatched: true}})
			if err != nil {
				t.Fatal(err)
			}
			if l.Items != all.Total || admin[i].Library != l || admin[i].Unmatched != unmatched.Total {
				t.Errorf("%s %s: %d items and %+v, %d unmatched; grid totals %d and %d unmatched",
					name, l.Name, l.Items, admin[i].Library, admin[i].Unmatched, all.Total, unmatched.Total)
			}
			if len(admin[i].Folders) != 1 || admin[i].Previews == "" {
				t.Errorf("%s %s: folders %+v, previews %q", name, l.Name, admin[i].Folders, admin[i].Previews)
			}
		}
	}
}
