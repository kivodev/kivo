package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func testDB(t *testing.T, migs fstest.MapFS) (*DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kivo.db")
	db, err := open(context.Background(), path, migs)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db, path
}

var basic = fstest.MapFS{
	"0001_items.sql": {Data: []byte(`CREATE TABLE item (id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE) STRICT;`)},
}

// baselineVersion is the embedded baseline's version, the oldest a database
// can be for this release to open it.
const baselineVersion = 53

// A new database gets the embedded baseline and ends at the latest
// migration's version.
func TestOpenEmbeddedMigrations(t *testing.T) {
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "kivo.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	all, err := loadMigrations(embeddedMigrations(t))
	if err != nil {
		t.Fatal(err)
	}
	if all[0].version != baselineVersion {
		t.Fatalf("the baseline is %s, want version %d", all[0].name, baselineVersion)
	}
	if v, _ := db.Version(context.Background()); v != all[len(all)-1].version {
		t.Fatalf("version = %d, want %d", v, all[len(all)-1].version)
	}
}

// A database at the latest version opens as it is: nothing runs again, its
// rows stay, and no snapshot is taken.
func TestCurrentDatabaseUntouched(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "kivo.db")
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Write(ctx, func(ctx context.Context, tx *Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO setting (key, value) VALUES ('kept', '1')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	before := schema(t, db)
	v, _ := db.Version(ctx)
	db.Close()

	again, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if got, _ := again.Version(ctx); got != v {
		t.Fatalf("version = %d, want %d", got, v)
	}
	if after := schema(t, again); after != before {
		t.Fatalf("the schema changed on opening:\n%s\nwant\n%s", after, before)
	}
	var value string
	if err := again.Reader().QueryRow(`SELECT value FROM setting WHERE key = 'kept'`).Scan(&value); err != nil || value != "1" {
		t.Fatalf("setting = %q, %v", value, err)
	}
	if snaps, _ := filepath.Glob(path + ".v*.snapshot"); len(snaps) != 0 {
		t.Fatalf("snapshots %v, want none", snaps)
	}
}

// A database older than the embedded baseline is refused and left as it
// was.
func TestDatabaseBeforeBaselineRefused(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "kivo.db")
	db, err := open(ctx, path, basic)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.writer.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, baselineVersion-1)); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := Open(ctx, path); !errors.Is(err, ErrOlderDatabase) {
		t.Fatalf("err = %v, want ErrOlderDatabase", err)
	}
	old, err := open(ctx, path, fstest.MapFS{fmt.Sprintf("%04d_items.sql", baselineVersion-1): basic["0001_items.sql"]})
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	var n int
	if err := old.Reader().QueryRow(`SELECT count(*) FROM sqlite_schema WHERE name = 'setting'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("the refused database gained the baseline's tables (n=%d, err=%v)", n, err)
	}
}

// The first migration is the baseline, whatever its number. A new database
// gets it and every migration after it; one at the baseline or later gets
// those after its version; one below the baseline is refused.
func TestBaseline(t *testing.T) {
	ctx := context.Background()
	migs := fstest.MapFS{
		"0005_baseline.sql": {Data: []byte(`CREATE TABLE item (id INTEGER PRIMARY KEY, name TEXT NOT NULL) STRICT;`)},
		"0006_more.sql":     {Data: []byte(`ALTER TABLE item ADD COLUMN extra TEXT;`)},
	}
	db, _ := testDB(t, migs)
	if v, _ := db.Version(ctx); v != 6 {
		t.Fatalf("a new database is at version %d, want 6", v)
	}

	// A database at the baseline gets only what follows it. The baseline
	// can't run again on it: its table exists.
	at, path := testDB(t, fstest.MapFS{"0005_baseline.sql": migs["0005_baseline.sql"]})
	if v, _ := at.Version(ctx); v != 5 {
		t.Fatalf("the baseline alone leaves version %d, want 5", v)
	}
	at.Close()
	up, err := open(ctx, path, migs)
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := up.Version(ctx); v != 6 {
		t.Fatalf("upgraded from the baseline to version %d, want 6", v)
	}
	if _, err := up.Reader().Exec(`SELECT extra FROM item`); err != nil {
		t.Fatal(err)
	}
	up.Close()

	below, path := testDB(t, basic)
	below.Close()
	if _, err := open(ctx, path, migs); !errors.Is(err, ErrOlderDatabase) {
		t.Fatalf("a database at version 1 opened with a baseline at 5: %v, want ErrOlderDatabase", err)
	}
}

// schema lists the database's tables, indexes, views and triggers with
// their SQL.
func schema(t *testing.T, db *DB) string {
	t.Helper()
	rows, err := db.Reader().Query(`SELECT type, name, coalesce(sql, '') FROM sqlite_schema ORDER BY type, name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var b strings.Builder
	for rows.Next() {
		var typ, name, sql string
		if err := rows.Scan(&typ, &name, &sql); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&b, "%s %s: %s\n", typ, name, sql)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestNewerDatabaseRefused(t *testing.T) {
	db, path := testDB(t, basic)
	if _, err := db.writer.Exec(`PRAGMA user_version = 99`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	_, err := open(context.Background(), path, basic)
	if !errors.Is(err, ErrNewerDatabase) {
		t.Fatalf("err = %v, want ErrNewerDatabase", err)
	}
}

func TestFailedMigrationRollsBack(t *testing.T) {
	_, path := testDB(t, basic)
	bad := fstest.MapFS{
		"0001_items.sql": basic["0001_items.sql"],
		"0002_bad.sql":   {Data: []byte(`CREATE TABLE other (id INTEGER); INSERT INTO nope VALUES (1);`)},
	}
	if _, err := open(context.Background(), path, bad); err == nil {
		t.Fatal("open succeeded with a failing migration")
	}
	db, err := open(context.Background(), path, basic)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if v, _ := db.Version(context.Background()); v != 1 {
		t.Fatalf("version = %d, want 1", v)
	}
	var n int
	if err := db.reader.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE name = 'other'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("table from the failed migration exists (n=%d, err=%v)", n, err)
	}
}

func TestMigrationNumbering(t *testing.T) {
	gap := fstest.MapFS{"0001_a.sql": {Data: []byte(`SELECT 1;`)}, "0003_c.sql": {Data: []byte(`SELECT 1;`)}}
	if _, err := loadMigrations(gap); err == nil {
		t.Fatal("a gap in numbering was accepted")
	}
}

func TestForeignKeysOffMigration(t *testing.T) {
	migs := fstest.MapFS{
		"0001_a.sql": {Data: []byte(`CREATE TABLE parent (id INTEGER PRIMARY KEY);
CREATE TABLE child (id INTEGER PRIMARY KEY, parent_id INTEGER NOT NULL REFERENCES parent(id));
INSERT INTO parent VALUES (1); INSERT INTO child VALUES (1, 1);`)},
		"0002_rebuild.sql": {Data: []byte(fkOffDirective + `
CREATE TABLE parent_new (id INTEGER PRIMARY KEY, name TEXT);
INSERT INTO parent_new (id) SELECT id FROM parent;
DROP TABLE parent;
ALTER TABLE parent_new RENAME TO parent;`)},
	}
	db, _ := testDB(t, migs)
	var on int
	if err := db.writer.QueryRow(`PRAGMA foreign_keys`).Scan(&on); err != nil || on != 1 {
		t.Fatalf("foreign_keys = %d after migration, want 1 (err=%v)", on, err)
	}

	broken := fstest.MapFS{
		"0001_a.sql":       migs["0001_a.sql"],
		"0002_orphans.sql": {Data: []byte(fkOffDirective + "\nDELETE FROM parent;")},
	}
	path := filepath.Join(t.TempDir(), "b.db")
	if _, err := open(context.Background(), path, broken); err == nil {
		t.Fatal("a migration leaving orphan rows was accepted")
	}
}

func TestWriteCommitsAndRollsBack(t *testing.T) {
	db, _ := testDB(t, basic)
	ctx := context.Background()
	ran := 0
	err := db.Write(ctx, func(ctx context.Context, tx *Tx) error {
		tx.AfterCommit(func() { ran++ })
		_, err := tx.ExecContext(ctx, `INSERT INTO item (name) VALUES ('a')`)
		return err
	})
	if err != nil || ran != 1 {
		t.Fatalf("Write = %v, after-commit ran %d times; want nil, 1", err, ran)
	}

	boom := errors.New("boom")
	err = db.Write(ctx, func(ctx context.Context, tx *Tx) error {
		tx.AfterCommit(func() { ran++ })
		if _, err := tx.ExecContext(ctx, `INSERT INTO item (name) VALUES ('b')`); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) || ran != 1 {
		t.Fatalf("Write = %v, after-commit ran %d times; want boom, 1", err, ran)
	}
	var n int
	_ = db.reader.QueryRow(`SELECT count(*) FROM item`).Scan(&n)
	if n != 1 {
		t.Fatalf("rows = %d, want 1 (rollback kept the second insert)", n)
	}
}

func TestWritePanicRollsBack(t *testing.T) {
	db, _ := testDB(t, basic)
	func() {
		defer func() { _ = recover() }()
		_ = db.Write(context.Background(), func(ctx context.Context, tx *Tx) error {
			_, _ = tx.ExecContext(ctx, `INSERT INTO item (name) VALUES ('a')`)
			panic("boom")
		})
	}()
	// The writer must be usable again, with nothing committed.
	err := db.Write(context.Background(), func(ctx context.Context, tx *Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO item (name) VALUES ('a')`)
		return err
	})
	if err != nil {
		t.Fatalf("Write after a panic: %v", err)
	}
}

func TestNestedWritePanicsInTests(t *testing.T) {
	db, _ := testDB(t, basic)
	defer func() {
		if p := recover(); p != ErrNestedWrite {
			t.Fatalf("recovered %v, want ErrNestedWrite", p)
		}
	}()
	_ = db.Write(context.Background(), func(ctx context.Context, tx *Tx) error {
		return db.Write(ctx, func(context.Context, *Tx) error { return nil })
	})
	t.Fatal("nested Write did not panic")
}

func TestReadersProceedDuringWrite(t *testing.T) {
	db, _ := testDB(t, basic)
	ctx := context.Background()
	inWrite, release := make(chan struct{}), make(chan struct{})
	done := make(chan error)
	go func() {
		done <- db.Write(ctx, func(ctx context.Context, tx *Tx) error {
			if _, err := tx.ExecContext(ctx, `INSERT INTO item (name) VALUES ('uncommitted')`); err != nil {
				return err
			}
			close(inWrite)
			<-release
			return nil
		})
	}()
	<-inWrite

	readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	err := db.Read(readCtx, func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRow(`SELECT count(*) FROM item`).Scan(&n); err != nil {
			return err
		}
		if n != 0 {
			t.Errorf("reader saw %d uncommitted rows", n)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read during a long write: %v", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestReaderIsReadOnly(t *testing.T) {
	db, _ := testDB(t, basic)
	if _, err := db.Reader().Exec(`INSERT INTO item (name) VALUES ('x')`); err == nil {
		t.Fatal("reader pool accepted a write")
	}
}

// A read whose context ends mid-query, as a request's context does when
// its client goes away, fails with SQLite's interrupt. It doesn't also
// fail on the transaction that database/sql rolled back on its own.
func TestReadCancelled(t *testing.T) {
	db, _ := testDB(t, basic)
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	err := db.Read(ctx, func(tx *sql.Tx) error {
		var n int
		return tx.QueryRowContext(ctx, `WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n) SELECT count(*) FROM n`).Scan(&n)
	})
	if err == nil || errors.Is(err, sql.ErrTxDone) {
		t.Fatalf("a cancelled read: %v", err)
	}
}

func TestIsUnique(t *testing.T) {
	db, _ := testDB(t, basic)
	insert := func(ctx context.Context, tx *Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO item (name) VALUES ('dup')`)
		return err
	}
	if err := db.Write(context.Background(), insert); err != nil {
		t.Fatal(err)
	}
	err := db.Write(context.Background(), insert)
	if !IsUnique(err) {
		t.Fatalf("IsUnique(%v) = false", err)
	}
	if IsUnique(errors.New("other")) {
		t.Fatal("IsUnique(plain error) = true")
	}
}

// TestWordSearch checks the tokenizer that the search indexes use. Words
// are compared without case or diacritics, punctuation splits them, and a
// prefix query works. It also checks match_rank, which is registered on
// every connection.
func TestWordSearch(t *testing.T) {
	db, _ := testDB(t, basic)
	err := db.Write(context.Background(), func(ctx context.Context, tx *Tx) error {
		_, err := tx.ExecContext(ctx, `CREATE VIRTUAL TABLE f USING fts5(title, tokenize = 'unicode61 remove_diacritics 2', prefix = '1 2 3');
INSERT INTO f (title) VALUES ('Amélie'), ('Spider-Man'), ('The Tournament');`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for q, want := range map[string]string{`"amelie"`: "Amélie", `"spider" "man"`: "Spider-Man", `"tour"*`: "The Tournament", `"nam"*`: ""} {
		var got string
		err := db.reader.QueryRow(`SELECT coalesce(group_concat(title), '') FROM f WHERE f MATCH ?`, q).Scan(&got)
		if err != nil || got != want {
			t.Errorf("MATCH %s = %q, %v; want %q", q, got, err, want)
		}
	}
	var rank int
	err = db.reader.QueryRow(`SELECT match_rank(f, title, 'spider man') FROM f WHERE f MATCH '"spider" "man"*'`).Scan(&rank)
	if err != nil || rank != 1 {
		t.Errorf("match_rank = %d, %v; want 1", rank, err)
	}
}

func TestSearchKeyAndMatchRank(t *testing.T) {
	for in, want := range map[string]string{
		"The Name of the Rose":    "name of the rose",
		"Amélie":                  "amelie",
		"Spider-Man: No Way Home": "spider man no way home",
		"A":                       "a",
		"An  American   Tail":     "american tail",
		"  ":                      "",
		"東京物語":                    "東京物語",
	} {
		if got := SearchKey(in); got != want {
			t.Errorf("SearchKey(%q) = %q, want %q", in, got, want)
		}
	}
	for _, tt := range []struct {
		text, q string
		want    int
	}{
		{"The Office", "the office", 1},
		{"Amélie", "amelie", 1},
		{"The Name of the Rose", "Name of the", 2},
		{"The Name of the Rose", "nam", 2},
		{"In the Name of the Father", "name of the", 3},
		{"The Tournament", "nam", 4},
		{"Rose Name", "name rose", 4},
	} {
		if got := MatchRank(tt.text, SearchKey(tt.q), 1); got != tt.want {
			t.Errorf("MatchRank(%q, %q) = %d, want %d", tt.text, tt.q, got, tt.want)
		}
	}
	if got := MatchRank("Garden", "rose", 2|4); got != 5 {
		t.Errorf("an alternative title's match ranks %d, want 5", got)
	}
	if got := MatchRank("Garden", "rose", 4); got != 6 {
		t.Errorf("a person's match ranks %d, want 6", got)
	}
}

func TestSnapshotAndBackup(t *testing.T) {
	db, _ := testDB(t, basic)
	ctx := context.Background()
	if err := db.Write(ctx, func(ctx context.Context, tx *Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO item (name) VALUES ('kept')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	var last string
	for range 3 {
		p, err := db.Backup(ctx, dir, 2)
		if err != nil {
			t.Fatal(err)
		}
		last = p
		time.Sleep(2 * time.Millisecond) // backup names have millisecond resolution
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Fatalf("backups kept = %d, want 2", len(entries))
	}

	b, err := open(ctx, last, basic)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	var check, name string
	if err := b.reader.QueryRow(`PRAGMA quick_check`).Scan(&check); err != nil || check != "ok" {
		t.Fatalf("quick_check = %q, %v", check, err)
	}
	if err := b.reader.QueryRow(`SELECT name FROM item`).Scan(&name); err != nil || name != "kept" {
		t.Fatalf("backup row = %q, %v", name, err)
	}
}

// Restore refuses while the database is open, as it is while the server
// runs. It also refuses a snapshot that isn't a sound database. With the
// server stopped, it puts the snapshot in place of the database and its
// WAL.
func TestRestore(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows can't replace the database while Restore holds its lock (#9)")
	}
	ctx := context.Background()
	db, path := testDB(t, basic)
	insert := func(name string) {
		t.Helper()
		if err := db.Write(ctx, func(ctx context.Context, tx *Tx) error {
			_, err := tx.ExecContext(ctx, `INSERT INTO item (name) VALUES (?)`, name)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	insert("kept")
	snap := filepath.Join(t.TempDir(), "snap.db")
	if err := db.Snapshot(ctx, snap); err != nil {
		t.Fatal(err)
	}
	insert("after the snapshot")

	if err := Restore(ctx, path, snap); !errors.Is(err, ErrInUse) {
		t.Fatalf("while open: %v, want ErrInUse", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	garbage := filepath.Join(t.TempDir(), "garbage.db")
	if err := os.WriteFile(garbage, []byte(strings.Repeat("not a database ", 512)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Restore(ctx, path, garbage); err == nil {
		t.Fatal("restored a file that isn't a database")
	}

	if err := Restore(ctx, path, snap); err != nil {
		t.Fatal(err)
	}
	restored, err := open(ctx, path, basic)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	var names []string
	rows, err := restored.reader.Query(`SELECT name FROM item`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		names = append(names, n)
	}
	if !slices.Equal(names, []string{"kept"}) {
		t.Fatalf("restored rows %v", names)
	}
}

func TestSnapshotRefusesLowSpace(t *testing.T) {
	db, _ := testDB(t, basic)
	orig := freeBytes
	t.Cleanup(func() { freeBytes = orig })
	size, _ := fileSize(db.path)
	freeBytes = func(string) (int64, bool) { return 2*size - 1, true }
	dst := filepath.Join(t.TempDir(), "snap.db")
	if err := db.Snapshot(context.Background(), dst); !errors.Is(err, ErrLowSpace) {
		t.Fatalf("err = %v, want ErrLowSpace", err)
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatal("snapshot file written despite low space")
	}
}

func TestCheckpoint(t *testing.T) {
	db, _ := testDB(t, basic)
	if err := db.Checkpoint(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func embeddedMigrations(t *testing.T) fstest.MapFS {
	t.Helper()
	out := fstest.MapFS{}
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		b, _ := migrations.ReadFile("migrations/" + e.Name())
		out[e.Name()] = &fstest.MapFile{Data: b}
	}
	return out
}

func TestUpgradeSnapshotsFirst(t *testing.T) {
	ctx := context.Background()
	db, path := testDB(t, basic)
	if err := db.Write(ctx, func(ctx context.Context, tx *Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO item (name) VALUES ('before')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	db.Close()

	next := fstest.MapFS{
		"0001_items.sql": basic["0001_items.sql"],
		"0002_more.sql":  {Data: []byte(`ALTER TABLE item ADD COLUMN extra TEXT;`)},
	}
	up, err := open(ctx, path, next)
	if err != nil {
		t.Fatal(err)
	}
	defer up.Close()
	snap, err := open(ctx, path+".v1.snapshot", basic)
	if err != nil {
		t.Fatalf("no snapshot before the upgrade: %v", err)
	}
	defer snap.Close()
	var name string
	if err := snap.Reader().QueryRow(`SELECT name FROM item`).Scan(&name); err != nil || name != "before" {
		t.Fatalf("snapshot holds %q, %v", name, err)
	}
	if v, _ := snap.Version(ctx); v != 1 {
		t.Fatalf("snapshot at version %d, want 1", v)
	}
}
