package query_test

import (
	"context"
	"errors"
	"testing"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/query"
)

// TestMissingFiles checks that the Missing files list pages newest missing
// first. Ties within a batch marked gone at once break by ID. Each file
// names its title, and the list keeps to the libraries the Viewer has.
func TestMissingFiles(t *testing.T) {
	h := newHousehold(t)
	h.b.Write(func(ctx context.Context, tx *store.Tx) error {
		if err := h.b.MarkGone(ctx, tx, h.secret.File, h.expanse1.File, h.dune.File); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE media_file SET gone_at = 2000 WHERE status = 'gone' AND id <> ?`, h.lost.File)
		if err == nil {
			_, err = tx.ExecContext(ctx, `UPDATE media_file SET gone_at = 1000 WHERE id = ?`, h.lost.File)
		}
		return err
	})

	var got []query.File
	var after query.Cursor
	for range 3 {
		p, err := h.q.MissingFiles(ctx, h.admin, id.ID{}, after, 3)
		if err != nil {
			t.Fatal(err)
		}
		if after == "" && p.Total != 4 {
			t.Errorf("total %d, want 4", p.Total)
		}
		got = append(got, p.Items...)
		if after = p.Next; after == "" {
			break
		}
	}
	if len(got) != 4 || got[3].ID != h.lost.File || got[3].Title != "Lost Film" || got[3].Gone.UnixMilli() != 1000 {
		t.Fatalf("missing files %+v", got)
	}
	for i := range 2 {
		if got[i].ID.String() <= got[i+1].ID.String() {
			t.Errorf("a tie isn't ordered by ID, newest first: %s then %s", got[i].ID, got[i+1].ID)
		}
	}
	titles := map[id.ID]string{}
	for _, f := range got {
		titles[f.ID] = f.Title
	}
	if titles[h.dune.File] != "Dune" || titles[h.expanse1.File] != "Episode 1" || titles[h.secret.File] != "Secret" {
		t.Errorf("titles %v", titles)
	}

	friend, err := h.q.MissingFiles(ctx, h.friend, id.ID{}, "", 0)
	if err != nil || friend.Total != 3 || len(friend.Items) != 3 {
		t.Errorf("a Viewer without Private: %+v, %v", friend, err)
	}
	shows, err := h.q.MissingFiles(ctx, h.admin, h.shows, "", 0)
	if err != nil || shows.Total != 1 || len(shows.Items) != 1 || shows.Items[0].ID != h.expanse1.File {
		t.Errorf("Shows alone: %+v, %v", shows, err)
	}
}

// TestWorkFiles checks that Show files lists every part of a work's
// versions, with their tracks in catalog's order. It lists nothing for a
// work the Viewer may not see.
func TestWorkFiles(t *testing.T) {
	h := newHousehold(t)
	files, err := h.q.WorkFiles(ctx, h.admin, h.dune.Work)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].ID != h.dune.File || files[0].Version != h.dune.Version || files[0].Part != 0 ||
		files[0].Title != "Dune" || !files[0].Gone.IsZero() || files[0].Duration == 0 {
		t.Fatalf("files %+v", files)
	}
	var types []string
	for _, tr := range files[0].Tracks {
		types = append(types, tr.Type)
	}
	if len(types) != 3 || types[0] != "video" || types[1] != "audio" || types[2] != "subtitle" || !files[0].Tracks[2].Forced {
		t.Errorf("tracks %+v", files[0].Tracks)
	}

	if files, err := h.q.WorkFiles(ctx, h.admin, h.expanse); err != nil || len(files) != 0 {
		t.Errorf("a series: %+v, %v", files, err)
	}
	for name, w := range map[string]id.ID{"a work above the kid's ceiling": h.dune.Work, "no such work": id.New()} {
		if _, err := h.q.WorkFiles(ctx, h.kid, w); !errors.Is(err, query.ErrNotFound) {
			t.Errorf("%s: %v", name, err)
		}
	}
}
