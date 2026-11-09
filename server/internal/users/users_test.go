package users

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/store/storetest"
)

type fixture struct {
	t  *testing.T
	db *store.DB
	s  *Service
}

func newFixture(t *testing.T) *fixture {
	db := storetest.New(t)
	return &fixture{t: t, db: db, s: New(db, events.NewBus())}
}

// libraries creates n libraries for grants to name.
func (f *fixture) libraries(n int) []id.ID {
	f.t.Helper()
	out := make([]id.ID, n)
	err := f.db.Write(context.Background(), func(ctx context.Context, tx *store.Tx) error {
		for i := range out {
			out[i] = id.New()
			_, err := tx.ExecContext(ctx, `INSERT INTO library (id, name, kind, created_at) VALUES (?, ?, 'movies', 0)`, out[i], fmt.Sprint("Library ", i))
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		f.t.Fatal(err)
	}
	return out
}

func (f *fixture) create(u NewUser) (User, error) {
	var out User
	err := f.db.Write(context.Background(), func(ctx context.Context, tx *store.Tx) error {
		var err error
		out, err = f.s.CreateUser(ctx, tx, u)
		return err
	})
	return out, err
}

// user creates a user. With no grants given, the user sees every library.
func (f *fixture) user(u NewUser) User {
	f.t.Helper()
	if len(u.Grants.Libraries) == 0 {
		u.Grants.All = true
	}
	out, err := f.create(u)
	if err != nil {
		f.t.Fatal(err)
	}
	return out
}

func (f *fixture) viewer(user id.ID) Viewer {
	f.t.Helper()
	v, err := f.s.Viewer(context.Background(), user)
	if err != nil {
		f.t.Fatal(err)
	}
	return v
}

func TestZeroViewerSeesNothing(t *testing.T) {
	var v Viewer
	if v.Admin() || v.AllLibraries() || len(v.Libraries()) != 0 || v.CanSee(id.New()) || v.ShowsUnrated() {
		t.Fatalf("zero Viewer sees something: %+v", v)
	}
	if !v.User().IsZero() {
		t.Fatal("zero Viewer has an owner")
	}
}

func TestNamesAreUniqueIgnoringCase(t *testing.T) {
	f := newFixture(t)
	f.user(NewUser{Username: "Anna"})
	if _, err := f.create(NewUser{Username: "anna", Grants: Grants{All: true}}); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("second anna: err = %v, want ErrNameTaken", err)
	}
}

func TestInvalidNames(t *testing.T) {
	f := newFixture(t)
	for _, name := range []string{"", " anna", "anna ", "an\nna", strings.Repeat("x", maxNameLen+1), "\xff"} {
		if _, err := f.create(NewUser{Username: name, Grants: Grants{All: true}}); !errors.Is(err, ErrInvalidName) {
			t.Errorf("username %q: err = %v, want ErrInvalidName", name, err)
		}
	}
}

func TestLibrariesMustBeChosen(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	if _, err := f.create(NewUser{Username: "anna"}); !errors.Is(err, ErrNoLibraries) {
		t.Fatalf("no libraries: err = %v, want ErrNoLibraries", err)
	}
	if _, err := f.create(NewUser{Username: "anna", Grants: Grants{Libraries: []id.ID{id.New()}}}); !errors.Is(err, ErrUnknownLibrary) {
		t.Fatalf("unknown library: err = %v, want ErrUnknownLibrary", err)
	}
	u := f.user(NewUser{Username: "ben"})
	if err := f.s.Update(ctx, u.ID, Change{Grants: &Grants{}}); !errors.Is(err, ErrNoLibraries) {
		t.Fatalf("update to no libraries: err = %v, want ErrNoLibraries", err)
	}
}

func TestViewerGrants(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	l := f.libraries(3)
	a, b, c := l[0], l[1], l[2]
	u := f.user(NewUser{Username: "anna", Grants: Grants{Libraries: []id.ID{b, a}}})

	v := f.viewer(u.ID)
	want := []id.ID{a, b} // IDs sort by creation
	if got := v.Libraries(); !slices.Equal(got, want) {
		t.Fatalf("Libraries = %v, want %v", got, want)
	}
	if !v.CanSee(a) || !v.CanSee(b) || v.CanSee(c) || v.AllLibraries() {
		t.Fatal("CanSee disagrees with the grants")
	}
	v.Libraries()[0] = c // Libraries returns a copy, so the Viewer is unchanged
	if v.CanSee(c) {
		t.Fatal("Libraries exposes the Viewer's slice")
	}

	// Revoking a grant applies to the next Viewer.
	if err := f.s.Update(ctx, u.ID, Change{Grants: &Grants{Libraries: []id.ID{b}}}); err != nil {
		t.Fatal(err)
	}
	if f.viewer(u.ID).CanSee(a) {
		t.Fatal("revoked grant still visible")
	}

	// Grant every library, then a list again.
	if err := f.s.Update(ctx, u.ID, Change{Grants: &Grants{All: true}}); err != nil {
		t.Fatal(err)
	}
	if v := f.viewer(u.ID); !v.AllLibraries() || !v.CanSee(c) || v.CanSee(id.ID{}) {
		t.Fatalf("all libraries: %+v", v)
	}
	if got, _ := f.s.User(ctx, u.ID); !got.Grants.All || len(got.Grants.Libraries) != 0 {
		t.Fatalf("Grants = %+v, want all", got.Grants)
	}
	if err := f.s.Update(ctx, u.ID, Change{Grants: &Grants{Libraries: []id.ID{c}}}); err != nil {
		t.Fatal(err)
	}
	if v := f.viewer(u.ID); v.AllLibraries() || !slices.Equal(v.Libraries(), []id.ID{c}) {
		t.Fatalf("back to a list: %+v", v)
	}

	if err := f.s.Update(ctx, id.New(), Change{Grants: &Grants{All: true}}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Update on a missing user: err = %v", err)
	}
}

func TestViewerParental(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	u := f.user(NewUser{Username: "kid", Parental: Parental{Ceiling: ptr(7), BlockUnrated: true}})
	v := f.viewer(u.ID)
	if c, ok := v.RatingCeiling(); !ok || c != 7 || v.ShowsUnrated() || v.Admin() {
		t.Fatalf("kid Viewer: %+v", v)
	}
	if err := f.s.Update(ctx, u.ID, Change{Parental: &Parental{}}); err != nil {
		t.Fatal(err)
	}
	v = f.viewer(u.ID)
	if _, ok := v.RatingCeiling(); ok || !v.ShowsUnrated() {
		t.Fatalf("no parental controls: %+v", v)
	}
}

func TestViewerOfDisabledUser(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	f.user(NewUser{Username: "admin", Admin: true})
	u := f.user(NewUser{Username: "ben"})

	if _, err := f.s.Viewer(ctx, id.New()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing user: err = %v, want ErrNotFound", err)
	}
	if err := f.s.Update(ctx, u.ID, Change{Disabled: ptr(true)}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.Viewer(ctx, u.ID); !errors.Is(err, ErrDisabled) {
		t.Fatalf("disabled user: err = %v, want ErrDisabled", err)
	}
	if err := f.s.Update(ctx, u.ID, Change{Disabled: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	f.viewer(u.ID)
}

func TestLastAdmin(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	a := f.user(NewUser{Username: "a", Admin: true})
	b := f.user(NewUser{Username: "b", Admin: true})
	c := f.user(NewUser{Username: "c"})

	if err := f.s.Update(ctx, a.ID, Change{Disabled: ptr(true)}); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]Change{
		"disable":  {Disabled: ptr(true)},
		"un-admin": {Admin: ptr(false)},
	} {
		if err := f.s.Update(ctx, b.ID, change); !errors.Is(err, ErrLastAdmin) {
			t.Errorf("%s the last admin: err = %v, want ErrLastAdmin", name, err)
		}
	}
	if err := f.s.Delete(ctx, b.ID); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("deleting the last admin: err = %v, want ErrLastAdmin", err)
	}
	// A refused change changes nothing.
	if got, _ := f.s.User(ctx, b.ID); !got.Admin || got.Disabled {
		t.Fatalf("after refusals: %+v", got)
	}

	// Another admin makes it possible.
	if err := f.s.Update(ctx, c.ID, Change{Admin: ptr(true)}); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Delete(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.User(ctx, b.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted user: err = %v", err)
	}
	if err := f.s.Delete(ctx, b.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleting again: err = %v, want ErrNotFound", err)
	}
}

func TestUsers(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	l := f.libraries(2)
	f.user(NewUser{Username: "zoe", Grants: Grants{Libraries: []id.ID{l[1]}}})
	f.user(NewUser{Username: "Anna", Admin: true})
	us, err := f.s.Users(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(us) != 2 || us[0].Username != "Anna" || !us[0].Admin || !us[0].Grants.All ||
		us[1].Username != "zoe" || !slices.Equal(us[1].Grants.Libraries, []id.ID{l[1]}) {
		t.Fatalf("Users = %+v", us)
	}
}

func ptr[T any](v T) *T { return &v }

// Only api, playback (for a play's owner) and cmd/kivo build Viewers.
// Every other package takes one.
func TestOnlyAllowedPackagesCallViewer(t *testing.T) {
	allowed := []string{"internal/api", "internal/playback", "internal/users", "cmd/kivo"}
	folder := filepath.Join("..", "..")
	fset := token.NewFileSet()
	err := filepath.WalkDir(folder, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		rel, _ := filepath.Rel(folder, path)
		rel = filepath.ToSlash(rel)
		if slices.ContainsFunc(allowed, func(p string) bool { return strings.HasPrefix(rel, p+"/") }) {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		if !slices.ContainsFunc(f.Imports, func(s *ast.ImportSpec) bool {
			p, _ := strconv.Unquote(s.Path.Value)
			return p == "github.com/mmcdole/kivo/internal/users"
		}) {
			return nil
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if c, ok := n.(*ast.CallExpr); ok {
				if sel, ok := c.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Viewer" && len(c.Args) == 2 {
					t.Errorf("%s: calls Viewer; only %v may", fset.Position(c.Pos()), allowed)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
