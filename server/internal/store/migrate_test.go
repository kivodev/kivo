package store

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

// through returns the embedded migrations up to and including the one
// whose name ends with suffix.
func through(t *testing.T, suffix string) fstest.MapFS {
	t.Helper()
	all, err := loadMigrations(embeddedMigrations(t))
	if err != nil {
		t.Fatal(err)
	}
	n := slices.IndexFunc(all, func(m migration) bool { return strings.HasSuffix(m.name, suffix) })
	if n < 0 {
		t.Fatalf("no migration %s", suffix)
	}
	out := fstest.MapFS{}
	for _, m := range all[:n+1] {
		out[m.name] = &fstest.MapFile{Data: []byte(m.sql)}
	}
	return out
}

// 0056 stores a subtitle file next to the video as 'external'. Every track
// keeps its rowid and its other columns, a new external track is accepted
// after it, and the index, the view and the cascade from media_file still
// work.
func TestMigrationExternalSubtitles(t *testing.T) {
	ctx := context.Background()
	db, path := testDB(t, through(t, "_session_device.sql"))
	err := db.Write(ctx, func(ctx context.Context, tx *Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO library (id, name, kind, created_at) VALUES (x'01', 'Movies', 'movies', 0);
			INSERT INTO library_folder (id, library_id, path) VALUES (x'02', x'01', '/movies');
			INSERT INTO media_file (id, folder_id, rel_path, size, mtime, status) VALUES (x'03', x'02', 'Dune/Dune.mkv', 1, 0, 'present');
			INSERT INTO media_track (rowid, id, file_id, source, stream_index, rel_path, type, codec, language, info) VALUES
				(7, x'a1', x'03', 'embedded', 0, NULL, 'video', 'hevc', NULL, '{"width":3840}'),
				(8, x'a2', x'03', 'embedded', 2, NULL, 'subtitle', 'ass', 'en', NULL),
				(9, x'a3', x'03', 'sidecar', NULL, 'Dune/Dune.fr.srt', 'subtitle', 'subrip', 'fr', NULL),
				(10, x'a4', x'03', 'downloaded', NULL, '03/de.srt', 'subtitle', 'subrip', 'de', NULL);`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	up, err := open(ctx, path, embeddedMigrations(t))
	if err != nil {
		t.Fatal(err)
	}
	defer up.Close()
	rows, err := up.Reader().QueryContext(ctx, `SELECT rowid, hex(id), source, coalesce(rel_path, ''), codec, coalesce(language, '') FROM media_track ORDER BY rowid`)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for rows.Next() {
		var rowid int
		var id, source, rel, codec, lang string
		if err := rows.Scan(&rowid, &id, &source, &rel, &codec, &lang); err != nil {
			t.Fatal(err)
		}
		got = append(got, fmt.Sprintf("%d %s %s %s %s %s", rowid, id, source, rel, codec, lang))
	}
	rows.Close()
	want := []string{
		"7 A1 embedded  hevc ",
		"8 A2 embedded  ass en",
		"9 A3 external Dune/Dune.fr.srt subrip fr",
		"10 A4 downloaded 03/de.srt subrip de",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("media_track after 0056:\n%q\nwant\n%q", got, want)
	}

	var n int
	if err := up.Reader().QueryRowContext(ctx, `SELECT count(*) FROM sqlite_schema WHERE name IN ('media_track_extracted', 'work_browse_keys')`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("the index and view after 0056: %d of 2 (%v)", n, err)
	}
	if _, err := up.Reader().ExecContext(ctx, `SELECT * FROM work_browse_keys`); err != nil {
		t.Fatalf("work_browse_keys after 0056: %v", err)
	}

	err = up.Write(ctx, func(ctx context.Context, tx *Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO media_track (id, file_id, source, rel_path, type, codec) VALUES
			(x'a5', x'03', 'external', 'Dune/Dune.es.srt', 'subtitle', 'subrip')`)
		return err
	})
	if err != nil {
		t.Fatalf("a new external subtitle after 0056: %v", err)
	}

	// The media file's cascade still reaches the rebuilt table.
	err = up.Write(ctx, func(ctx context.Context, tx *Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM media_file WHERE id = x'03'`)
		return err
	})
	if err != nil || up.Reader().QueryRowContext(ctx, `SELECT count(*) FROM media_track`).Scan(&n) != nil || n != 0 {
		t.Fatalf("after deleting the file: %d tracks left (%v)", n, err)
	}
}

// 0059 hides the Recently Added rows of unpinned libraries, and Continue
// Listening for a user with audiobooks libraries, every one unpinned. A
// library with no user_library row counts as pinned, and a user's other
// rows stay out of the new table.
func TestMigrationHomeRows(t *testing.T) {
	ctx := context.Background()
	db, path := testDB(t, through(t, "_continue_removed.sql"))
	err := db.Write(ctx, func(ctx context.Context, tx *Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO user (id, username, admin, all_libraries, created_at) VALUES
				(x'a1', 'unpinned', 0, 1, 1),
				(x'a2', 'someleft', 0, 1, 1),
				(x'a3', 'granted', 0, 0, 1),
				(x'a4', 'nobooks', 0, 0, 1),
				(x'a5', 'allpinned', 0, 1, 1);
			INSERT INTO library (id, name, kind, created_at) VALUES
				(x'b1000000000000000000000000000000', 'Movies', 'movies', 1),
				(x'b2000000000000000000000000000000', 'Books', 'audiobooks', 2),
				(x'b3000000000000000000000000000000', 'More Books', 'audiobooks', 3);
			INSERT INTO library_grant (user_id, library_id) VALUES (x'a3', x'b2000000000000000000000000000000'), (x'a3', x'b1000000000000000000000000000000');
			INSERT INTO user_library (user_id, library_id, position, pinned) VALUES
				(x'a1', x'b1000000000000000000000000000000', 0, 0), (x'a1', x'b2000000000000000000000000000000', 1, 0), (x'a1', x'b3000000000000000000000000000000', 2, 0),
				(x'a2', x'b2000000000000000000000000000000', 0, 0),
				(x'a3', x'b1000000000000000000000000000000', 0, 1), (x'a3', x'b2000000000000000000000000000000', 1, 0);`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	up, err := open(ctx, path, embeddedMigrations(t))
	if err != nil {
		t.Fatal(err)
	}
	defer up.Close()
	rows, err := up.Reader().QueryContext(ctx, `
		SELECT hex(user_id), row_id, position IS NULL, shown FROM user_home_row ORDER BY user_id, row_id`)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for rows.Next() {
		var u, r string
		var null, shown int
		if err := rows.Scan(&u, &r, &null, &shown); err != nil {
			t.Fatal(err)
		}
		got = append(got, fmt.Sprintf("%s %s %d %d", u, r, null, shown))
	}
	rows.Close()
	want := []string{
		"A1 continueListening 1 0",
		"A1 recentlyAdded:b1000000-0000-0000-0000-000000000000 1 0",
		"A1 recentlyAdded:b2000000-0000-0000-0000-000000000000 1 0",
		"A1 recentlyAdded:b3000000-0000-0000-0000-000000000000 1 0",
		"A2 recentlyAdded:b2000000-0000-0000-0000-000000000000 1 0",
		"A3 continueListening 1 0",
		"A3 recentlyAdded:b2000000-0000-0000-0000-000000000000 1 0",
	}
	if !slices.Equal(got, want) {
		t.Errorf("user_home_row after 0059:\n%q\nwant\n%q", got, want)
	}
}

// 0060 creates My List for every user and leaves user_state without the
// old columns.
func TestMigrationLists(t *testing.T) {
	ctx := context.Background()
	db, path := testDB(t, through(t, "_home_rows.sql"))
	err := db.Write(ctx, func(ctx context.Context, tx *Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO user (id, username, admin, all_libraries, created_at) VALUES
				(x'a1', 'anna', 0, 1, 1), (x'a2', 'bob', 0, 1, 1);`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	up, err := open(ctx, path, embeddedMigrations(t))
	if err != nil {
		t.Fatal(err)
	}
	defer up.Close()
	var lists int
	err = up.Reader().QueryRowContext(ctx, `SELECT count(*) FROM list WHERE built_in AND name = 'Watchlist'`).Scan(&lists)
	if err != nil || lists != 2 {
		t.Fatalf("after 0060: %d My Lists, %v", lists, err)
	}
	if err := up.Reader().QueryRowContext(ctx, `SELECT favorite FROM user_state LIMIT 1`).Scan(new(int)); err == nil {
		t.Fatal("user_state still has favorite")
	}
}

// 0063 calls the built-in list Watchlist, and a list a user had already
// named that takes the built-in list's old name.
func TestMigrationWatchlist(t *testing.T) {
	ctx := context.Background()
	db, path := testDB(t, through(t, "_smart_lists.sql"))
	err := db.Write(ctx, func(ctx context.Context, tx *Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO user (id, username, admin, all_libraries, created_at) VALUES
				(x'a1', 'anna', 0, 1, 1), (x'a2', 'bob', 0, 1, 1);
			INSERT INTO list (id, user_id, name, built_in, created_at) VALUES
				(x'c1', x'a1', 'My List', 1, 1), (x'c2', x'a1', 'watchlist', 0, 1),
				(x'c3', x'a2', 'My List', 1, 1), (x'c4', x'a2', 'Road Trip', 0, 1);`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	up, err := open(ctx, path, embeddedMigrations(t))
	if err != nil {
		t.Fatal(err)
	}
	defer up.Close()
	for id, want := range map[string]string{"c1": "Watchlist", "c2": "My List", "c3": "Watchlist", "c4": "Road Trip"} {
		var name string
		if err := up.Reader().QueryRowContext(ctx, `SELECT name FROM list WHERE hex(id) = upper(?)`, id).Scan(&name); err != nil || name != want {
			t.Errorf("list %s: %q, %v; want %q", id, name, err, want)
		}
	}
}

// 0061 stores each work's release date as its earliest day, for the grid's
// release sort.
func TestMigrationReleaseKey(t *testing.T) {
	ctx := context.Background()
	db, path := testDB(t, through(t, "_lists.sql"))
	err := db.Write(ctx, func(ctx context.Context, tx *Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO library (id, name, kind, created_at) VALUES (x'b1', 'Movies', 'movies', 1);
			INSERT INTO work (id, kind, root_id, title, sort_title, year, release_date, added_at) VALUES
				(x'01', 'movie', x'01', 'Full', 'Full', 2010, '2010-05-09', 1),
				(x'02', 'movie', x'02', 'Partial', 'Partial', 2011, '2011-03', 1),
				(x'03', 'movie', x'03', 'Year', 'Year', 2012, NULL, 1),
				(x'04', 'movie', x'04', 'None', 'None', NULL, NULL, 1);
			INSERT INTO work_library (library_id, work_id, is_root, sort_title, added_at) VALUES
				(x'b1', x'01', 1, 'Full', 1), (x'b1', x'02', 1, 'Partial', 1),
				(x'b1', x'03', 1, 'Year', 1), (x'b1', x'04', 1, 'None', 1);`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	up, err := open(ctx, path, embeddedMigrations(t))
	if err != nil {
		t.Fatal(err)
	}
	defer up.Close()
	rows, err := up.Reader().QueryContext(ctx, `SELECT release_key FROM work_library ORDER BY work_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			t.Fatal(err)
		}
		got = append(got, k)
	}
	if want := []string{"2010-05-09", "2011-03-01", "2012-01-01", ""}; !slices.Equal(got, want) {
		t.Errorf("release keys %q, want %q", got, want)
	}
}

// 0057 makes the problem codes problemRaised events keep camelCase, in
// their codes and their problems' IDs, and leaves other events and codes
// as they were.
func TestMigrationProblemCodes(t *testing.T) {
	ctx := context.Background()
	db, path := testDB(t, through(t, "_external_subtitles.sql"))
	err := db.Write(ctx, func(ctx context.Context, tx *Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO activity (id, at, kind, data) VALUES
				(x'01', 1, 'problemRaised', '{"problem":"scan.folder_unreachable","code":"folder_unreachable","message":"m","severity":"warning"}'),
				(x'02', 2, 'problemRaised', '{"problem":"subtitles.opensubtitles_key_rejected","code":"opensubtitles_key_rejected"}'),
				(x'03', 3, 'problemRaised', '{"problem":"later.new_code","code":"new_code"}'),
				(x'05', 5, 'problemRaised', '{"problem":"metadata.tmdb_key_rejected","code":"tmdb_key_rejected"}'),
				(x'06', 6, 'problemRaised', '{"problem":"metadata.tvdb_unavailable","code":"tvdb_unavailable"}'),
				(x'04', 4, 'signedIn', '{"code":"cache_full"}');`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	up, err := open(ctx, path, embeddedMigrations(t))
	if err != nil {
		t.Fatal(err)
	}
	defer up.Close()
	rows, err := up.Reader().QueryContext(ctx, `SELECT data FROM activity ORDER BY at`)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			t.Fatal(err)
		}
		got = append(got, data)
	}
	rows.Close()
	want := []string{
		`{"problem":"scan.folderUnreachable","code":"folderUnreachable","message":"m","severity":"warning"}`,
		`{"problem":"subtitles.openSubtitlesKeyRejected","code":"openSubtitlesKeyRejected"}`,
		`{"problem":"later.new_code","code":"new_code"}`,
		`{"code":"cache_full"}`,
		`{"problem":"metadata.tmdbKeyRejected","code":"tmdbKeyRejected"}`,
		`{"problem":"metadata.tvdbUnavailable","code":"tvdbUnavailable"}`,
	}
	if !slices.Equal(got, want) {
		t.Fatalf("activity after 0057:\n%q\nwant\n%q", got, want)
	}
}

// The download migration keeps what a download delivers in place of its
// plan and converted files. The downloads stored before it are dropped,
// and the table has its indexes again.
func TestMigrationDownloadDelivered(t *testing.T) {
	ctx := context.Background()
	db, path := testDB(t, through(t, "_drop_audit.sql"))
	err := db.Write(ctx, func(ctx context.Context, tx *Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO user (id, username, admin, all_libraries, created_at) VALUES (x'a1', 'anna', 0, 1, 1);
			INSERT INTO session (id, token_hash, user_id, kind, created_at, last_used_at) VALUES (x'b1', x'00', x'a1', 'app', 1, 1);
			INSERT INTO download (id, user_id, session_id, work_id, version_id, request, plan, state, created_at, queued_at) VALUES
				(x'd1', x'a1', x'b1', x'01', x'02', '{}', '{}', 'queued', 1, 1);`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	up, err := open(ctx, path, through(t, "_download_delivered.sql"))
	if err != nil {
		t.Fatal(err)
	}
	defer up.Close()
	var n int
	if err := up.Reader().QueryRowContext(ctx, `SELECT count(*) FROM download`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("downloads kept: %d (%v)", n, err)
	}
	if err := up.Reader().QueryRowContext(ctx, `SELECT count(*) FROM sqlite_schema
		WHERE name IN ('download_user', 'download_session', 'download_queue', 'download_settled')`).Scan(&n); err != nil || n != 4 {
		t.Fatalf("the table's indexes: %d of 4 (%v)", n, err)
	}
	err = up.Write(ctx, func(ctx context.Context, tx *Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO download (id, user_id, session_id, work_id, version_id, request, delivered, state, created_at, queued_at)
			VALUES (x'd2', x'a1', x'b1', x'01', x'02', '{}', '{}', 'queued', 1, 1)`)
		return err
	})
	if err != nil {
		t.Fatalf("a download after it: %v", err)
	}
}

// The typed-token migration clears every session, API key and cast token,
// since none made before it can authenticate. Users stay.
func TestMigrationTypedTokens(t *testing.T) {
	ctx := context.Background()
	db, path := testDB(t, through(t, "_download_delivered.sql"))
	err := db.Write(ctx, func(ctx context.Context, tx *Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO user (id, username, admin, all_libraries, created_at) VALUES (x'a1', 'anna', 0, 1, 1);
			INSERT INTO session (id, token_hash, user_id, kind, created_at, last_used_at) VALUES (x'b1', x'00', x'a1', 'app', 1, 1);
			INSERT INTO api_key (id, token_hash, name, created_by, created_at) VALUES (x'c1', x'01', 'k', x'a1', 1);
			INSERT INTO cast_token (id, token_hash, session_id, name, receiver, created_at, last_used_at) VALUES
				(x'e1', x'02', x'b1', 'TV', 'web', 1, 1);`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	up, err := open(ctx, path, through(t, "_typed_tokens.sql"))
	if err != nil {
		t.Fatal(err)
	}
	defer up.Close()
	var credentials, users int
	err = up.Reader().QueryRowContext(ctx, `SELECT (SELECT count(*) FROM session) + (SELECT count(*) FROM api_key) +
		(SELECT count(*) FROM cast_token), (SELECT count(*) FROM user)`).Scan(&credentials, &users)
	if err != nil || credentials != 0 || users != 1 {
		t.Fatalf("after it: %d credentials, %d users (%v)", credentials, users, err)
	}
}

// 0068 lets activity record passwordChanged. Every event keeps its rowid
// and columns, the table has both its indexes again, and a kind it
// doesn't know is still refused.
func TestMigrationPasswordChanged(t *testing.T) {
	ctx := context.Background()
	db, path := testDB(t, through(t, "_typed_tokens.sql"))
	err := db.Write(ctx, func(ctx context.Context, tx *Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO activity (rowid, id, at, kind, data) VALUES
				(4, x'a1', 10, 'signedIn', '{"username":"ben"}'),
				(9, x'a2', 20, 'userJoined', '{"username":"dana","by":"admin"}');`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	up, err := open(ctx, path, through(t, "_password_changed.sql"))
	if err != nil {
		t.Fatal(err)
	}
	defer up.Close()
	rows, err := up.Reader().QueryContext(ctx, `SELECT rowid, hex(id), at, kind, data FROM activity ORDER BY rowid`)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for rows.Next() {
		var rowid, at int
		var id, kind, data string
		if err := rows.Scan(&rowid, &id, &at, &kind, &data); err != nil {
			t.Fatal(err)
		}
		got = append(got, fmt.Sprintf("%d %s %d %s %s", rowid, id, at, kind, data))
	}
	rows.Close()
	want := []string{
		`4 A1 10 signedIn {"username":"ben"}`,
		`9 A2 20 userJoined {"username":"dana","by":"admin"}`,
	}
	if !slices.Equal(got, want) {
		t.Fatalf("activity after 0068:\n%q\nwant\n%q", got, want)
	}
	var n int
	if err := up.Reader().QueryRowContext(ctx, `SELECT count(*) FROM sqlite_schema
		WHERE name IN ('activity_at', 'activity_kind')`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("the table's indexes: %d of 2 (%v)", n, err)
	}
	insert := func(kind string) error {
		return up.Write(ctx, func(ctx context.Context, tx *Tx) error {
			_, err := tx.ExecContext(ctx, `INSERT INTO activity (id, at, kind, data) VALUES (randomblob(16), 30, ?, '{}')`, kind)
			return err
		})
	}
	if err := insert("passwordChanged"); err != nil {
		t.Fatalf("a passwordChanged event after it: %v", err)
	}
	if err := insert("nope"); err == nil {
		t.Fatal("an unknown kind was accepted")
	}
}

// 0070 takes collapse out of each smart list's query and leaves every other
// query, a manual list and a query that doesn't parse as they were.
func TestMigrationQueryCollapse(t *testing.T) {
	ctx := context.Background()
	db, path := testDB(t, through(t, "_artwork_preview.sql"))
	err := db.Write(ctx, func(ctx context.Context, tx *Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO user (id, username, admin, all_libraries, created_at) VALUES (x'a1', 'anna', 0, 1, 1);
			INSERT INTO list (id, user_id, name, built_in, created_at, query) VALUES
				(x'c1', x'a1', 'Watchlist', 1, 1, NULL),
				(x'c2', x'a1', 'Shows', 0, 1, '{"version":1,"kinds":["series","episode"],"sort":[{"field":"title"}],"collapse":"series"}'),
				(x'c3', x'a1', 'Movies', 0, 1, '{"version":1,"kinds":["movie"],"maxWorks":10}'),
				(x'c4', x'a1', 'Broken', 0, 1, '{"collapse":');`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	up, err := open(ctx, path, through(t, "_query_collapse.sql"))
	if err != nil {
		t.Fatal(err)
	}
	defer up.Close()
	for id, want := range map[string]string{
		"c1": "",
		"c2": `{"version":1,"kinds":["series","episode"],"sort":[{"field":"title"}]}`,
		"c3": `{"version":1,"kinds":["movie"],"maxWorks":10}`,
		"c4": `{"collapse":`,
	} {
		var query string
		if err := up.Reader().QueryRowContext(ctx, `SELECT coalesce(query, '') FROM list WHERE hex(id) = upper(?)`, id).Scan(&query); err != nil || query != want {
			t.Errorf("list %s: %q, %v; want %q", id, query, err, want)
		}
	}
}

// 0071 rebuilds the sorts' indexes with is_root in them. Each still holds
// a library's top-level works alone, in its order, and a read of them in
// that order takes its rows from the index without the table.
func TestMigrationSortIndexesCover(t *testing.T) {
	ctx := context.Background()
	db, path := testDB(t, through(t, "_query_collapse.sql"))
	err := db.Write(ctx, func(ctx context.Context, tx *Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO library (id, name, kind, created_at) VALUES (x'b1', 'Shows', 'shows', 1);
			INSERT INTO work (id, kind, parent_id, root_id, title, sort_title, added_at) VALUES
				(x'01', 'series', NULL, x'01', 'B', 'B', 3),
				(x'02', 'series', NULL, x'02', 'A', 'A', 2),
				(x'03', 'season', x'01', x'01', 'Season 1', 'Season 1', 9);
			INSERT INTO work_library (library_id, work_id, is_root, sort_title, added_at) VALUES
				(x'b1', x'01', 1, 'B', 3), (x'b1', x'02', 1, 'A', 2), (x'b1', x'03', 0, 'Season 1', 9);`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	up, err := open(ctx, path, embeddedMigrations(t))
	if err != nil {
		t.Fatal(err)
	}
	defer up.Close()
	for index, column := range map[string]string{
		"work_library_title": "sort_title", "work_library_release": "release_key", "work_library_runtime": "runtime_sort",
		"work_library_rating": "rating_sort", "work_library_creator": "creator_sort", "work_library_last_added": "last_added_at",
		"work_library_added": "added_at",
	} {
		var columns string
		var partial bool
		err := up.Reader().QueryRowContext(ctx, `
			SELECT (SELECT group_concat(name, ',') FROM pragma_index_info(l.name)), l.partial
			FROM pragma_index_list('work_library') l WHERE l.name = ?`, index).Scan(&columns, &partial)
		want := "library_id," + column + ",sort_title,work_id,is_root"
		if column == "sort_title" {
			want = "library_id,sort_title,work_id,is_root"
		}
		if err != nil || columns != want || !partial {
			t.Errorf("%s holds %s, partial %v, %v; want %s of the top-level works", index, columns, partial, err, want)
		}
		read := `SELECT hex(wl.work_id) FROM work_library wl INDEXED BY ` + index + `
			WHERE wl.library_id = x'b1' AND wl.is_root ORDER BY wl.` + column
		var plan []string
		rows, err := up.Reader().QueryContext(ctx, "EXPLAIN QUERY PLAN "+read)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
				t.Fatal(err)
			}
			plan = append(plan, detail)
		}
		rows.Close()
		if want := []string{"SEARCH wl USING COVERING INDEX " + index + " (library_id=?)"}; !slices.Equal(plan, want) {
			t.Errorf("%s: the read's plan is %q, want %q", index, plan, want)
		}
	}
	var titled, added string
	err = up.Reader().QueryRowContext(ctx, `SELECT
		(SELECT group_concat(hex(work_id), ',') FROM (SELECT work_id FROM work_library INDEXED BY work_library_title
			WHERE library_id = x'b1' AND is_root ORDER BY sort_title)),
		(SELECT group_concat(hex(work_id), ',') FROM (SELECT work_id FROM work_library INDEXED BY work_library_added
			WHERE library_id = x'b1' AND is_root ORDER BY added_at DESC))`).Scan(&titled, &added)
	if err != nil || titled != "02,01" || added != "01,02" {
		t.Errorf("by title %s and by added %s, %v; want 02,01 and 01,02", titled, added, err)
	}
}

// 0072 drops the index of works by sort title, which no read uses, and
// leaves the works as they were.
func TestMigrationDropsWorkSortTitle(t *testing.T) {
	ctx := context.Background()
	db, path := testDB(t, through(t, "_sort_indexes_cover.sql"))
	var before int
	if err := db.Reader().QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name = 'work_sort_title'`).Scan(&before); err != nil || before != 1 {
		t.Fatalf("before: %d indexes named work_sort_title, %v; want 1", before, err)
	}
	err := db.Write(ctx, func(ctx context.Context, tx *Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO work (id, kind, parent_id, root_id, title, sort_title, added_at) VALUES
				(x'01', 'movie', NULL, x'01', 'b', 'b', 1), (x'02', 'movie', NULL, x'02', 'A', 'A', 2);`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	up, err := open(ctx, path, embeddedMigrations(t))
	if err != nil {
		t.Fatal(err)
	}
	defer up.Close()
	var after int
	var titles string
	err = up.Reader().QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM sqlite_master WHERE name = 'work_sort_title'),
		(SELECT group_concat(sort_title, ',') FROM (SELECT sort_title FROM work ORDER BY sort_title))`).Scan(&after, &titles)
	if err != nil || after != 0 || titles != "A,b" {
		t.Errorf("after: %d indexes named work_sort_title, works %q, %v; want none and A,b", after, titles, err)
	}
}
