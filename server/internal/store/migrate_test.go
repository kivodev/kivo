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

// 0073 gives every list the custom sort and every work in one a place,
// 2^32 apart, in the order the list was shown in: most recently added
// first, then by title. The works keep when they were added, and still
// leave with their list or their work.
func TestMigrationListOrder(t *testing.T) {
	ctx := context.Background()
	db, path := testDB(t, through(t, "_drop_work_sort_title.sql"))
	err := db.Write(ctx, func(ctx context.Context, tx *Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO user (id, username, admin, all_libraries, created_at) VALUES (x'a1', 'anna', 0, 1, 1);
			INSERT INTO list (id, user_id, name, built_in, created_at, query) VALUES
				(x'c1', x'a1', 'Watchlist', 1, 1, NULL), (x'c2', x'a1', 'Later', 0, 1, NULL),
				(x'c3', x'a1', 'Smart', 0, 1, '{"version":1,"kinds":["movie"]}');
			INSERT INTO work (id, kind, parent_id, root_id, title, sort_title, added_at) VALUES
				(x'01', 'movie', NULL, x'01', 'Alien', 'Alien', 1), (x'02', 'movie', NULL, x'02', 'Brazil', 'Brazil', 1),
				(x'03', 'movie', NULL, x'03', 'Casablanca', 'Casablanca', 1), (x'04', 'movie', NULL, x'04', 'Dune', 'Dune', 1);
			INSERT INTO list_work (list_id, work_id, added_at) VALUES
				(x'c1', x'01', 10), (x'c1', x'03', 30), (x'c1', x'04', 20), (x'c1', x'02', 20),
				(x'c2', x'04', 5), (x'c2', x'01', 7);`)
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
	read := func(query string) (out string) {
		t.Helper()
		if err := up.Reader().QueryRowContext(ctx, query).Scan(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	places := `SELECT group_concat(hex(work_id) || ':' || (position / 4294967296) || ':' || added_at, ' ')
		FROM (SELECT * FROM list_work WHERE list_id = x'%s' ORDER BY position)`
	if got, want := read(fmt.Sprintf(places, "c1")), "03:1:30 02:2:20 04:3:20 01:4:10"; got != want {
		t.Errorf("Watchlist's places: %s, want %s", got, want)
	}
	if got, want := read(fmt.Sprintf(places, "c2")), "01:1:7 04:2:5"; got != want {
		t.Errorf("the other list's places: %s, want %s", got, want)
	}
	if got := read(`SELECT group_concat(sort, ',') FROM list`); got != "custom,custom,custom" {
		t.Errorf("the lists' sorts: %s", got)
	}
	if got, want := read(`SELECT group_concat(name, ',') FROM (SELECT name FROM pragma_index_list('list_work') WHERE origin = 'c' ORDER BY name)`),
		"list_work_position,list_work_work"; got != want {
		t.Errorf("list_work's indexes: %s, want %s", got, want)
	}
	err = up.Write(ctx, func(ctx context.Context, tx *Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE list SET sort = 'shuffled' WHERE id = x'c1'`); err == nil {
			t.Error("an unknown sort was accepted")
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM list WHERE id = x'c2'; DELETE FROM work WHERE id = x'03';`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := read(`SELECT group_concat(hex(list_id) || hex(work_id), ' ') FROM (SELECT * FROM list_work ORDER BY position)`), "C102 C104 C101"; got != want {
		t.Errorf("after deleting a list and a work: %s, want %s", got, want)
	}
}

// 0074 turns each movie's trailer link into its first trailer and drops the
// link. Such a movie is made due a refresh, and no other is. Every image is
// kept as it was, a trailer can own one, and a trailer and its image leave
// with their movie.
func TestMigrationTrailers(t *testing.T) {
	ctx := context.Background()
	db, path := testDB(t, through(t, "_list_order.sql"))
	err := db.Write(ctx, func(ctx context.Context, tx *Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO work (id, kind, parent_id, root_id, title, sort_title, added_at, refreshed_at, refresh_error, trailer_url) VALUES
				(x'01', 'movie', NULL, x'01', 'Dune', 'Dune', 1, 100, NULL, 'https://www.youtube.com/watch?v=n9xhJrPXop4'),
				(x'02', 'movie', NULL, x'02', 'Heat', 'Heat', 1, 200, 'failed', NULL),
				(x'03', 'movie', NULL, x'03', 'Alien', 'Alien', 1, 300, NULL, 'https://www.youtube.com/watch?v=LjLamj-b0I8'),
				(x'04', 'movie', NULL, x'04', 'Odd', 'Odd', 1, 400, NULL, 'https://example.org/trailer');
			INSERT INTO person (id, name, sort_name) VALUES (x'b1', 'Ann', 'Ann');
			INSERT INTO collection (id, key, name, sort_name, created_at) VALUES (x'c1', 'tmdb:1', 'Dune Collection', 'Dune Collection', 1);
			INSERT INTO image (id, work_id, person_id, collection_id, kind, source, origin, origin_mtime, width, height, blurhash, chosen, fetched_at) VALUES
				(x'e1', x'01', NULL, NULL, 'poster', 'local', '/m/Dune/poster.jpg', 7, 1000, 1500, 'hash1', 1, 11),
				(x'e2', NULL, x'b1', NULL, 'photo', 'provider', 'https://image.tmdb.org/a.jpg', NULL, 400, 600, 'hash2', 0, 12),
				(x'e3', NULL, NULL, x'c1', 'backdrop', 'provider', 'https://image.tmdb.org/b.jpg', NULL, 1920, 1080, 'hash3', 0, 13);`)
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
	read := func(query string) (out string) {
		t.Helper()
		if err := up.Reader().QueryRowContext(ctx, query).Scan(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	if got, want := read(`SELECT group_concat(hex(work_id) || ' ' || ord || ' ' || key || ' ' || title || ' ' || length(id) || ' ' || substr(hex(id), 13, 1), ', ')
		FROM (SELECT * FROM trailer ORDER BY work_id)`),
		"01 0 n9xhJrPXop4 Trailer 16 7, 03 0 LjLamj-b0I8 Trailer 16 7"; got != want {
		t.Errorf("trailers: %s, want %s", got, want)
	}
	if got := read(`SELECT count(*) FROM pragma_table_info('work') WHERE name = 'trailer_url'`); got != "0" {
		t.Error("work still has trailer_url")
	}
	if got, want := read(`SELECT group_concat(hex(id) || ':' || coalesce(refreshed_at, '-') || ':' || coalesce(refresh_error, '-'), ' ')
		FROM (SELECT * FROM work ORDER BY id)`), "01:-:- 02:200:failed 03:-:- 04:400:-"; got != want {
		t.Errorf("refreshes due: %s, want %s", got, want)
	}
	if got, want := read(`SELECT group_concat(hex(id) || '|' || coalesce(hex(work_id), '') || '|' || coalesce(hex(person_id), '') || '|' ||
		coalesce(hex(collection_id), '') || '|' || coalesce(hex(trailer_id), '') || '|' || kind || '|' || source || '|' || origin || '|' ||
		coalesce(origin_mtime, '-') || '|' || width || 'x' || height || '|' || blurhash || '|' || chosen || '|' || fetched_at, ' ')
		FROM (SELECT * FROM image ORDER BY id)`),
		"E1|01||||poster|local|/m/Dune/poster.jpg|7|1000x1500|hash1|1|11 "+
			"E2||B1|||photo|provider|https://image.tmdb.org/a.jpg|-|400x600|hash2|0|12 "+
			"E3|||C1||backdrop|provider|https://image.tmdb.org/b.jpg|-|1920x1080|hash3|0|13"; got != want {
		t.Errorf("images: %s, want %s", got, want)
	}
	if got := read(`SELECT group_concat(name, ',') FROM (SELECT name FROM sqlite_master WHERE type = 'index' AND tbl_name = 'image' AND name LIKE 'image_%' ORDER BY name)`); got != "image_collection,image_person,image_trailer,image_work" {
		t.Errorf("image's indexes: %s", got)
	}

	thumb := `INSERT INTO image (id, %s, kind, source, origin, width, height, blurhash, fetched_at)
		SELECT x'%s', %s, 'thumb', 'provider', 'https://i.ytimg.com/vi/n9xhJrPXop4/maxresdefault.jpg', 1280, 720, 'hash4', 14 FROM trailer WHERE work_id = x'01'`
	exec := func(query string) error {
		return up.Write(ctx, func(ctx context.Context, tx *Tx) error {
			_, err := tx.ExecContext(ctx, query)
			return err
		})
	}
	if err := exec(fmt.Sprintf(thumb, "trailer_id", "e4", "id")); err != nil {
		t.Fatalf("a trailer's image: %v", err)
	}
	if err := exec(fmt.Sprintf(thumb, "trailer_id", "e5", "id")); err == nil {
		t.Error("a trailer took a second thumbnail")
	}
	if err := exec(fmt.Sprintf(thumb, "trailer_id, work_id", "e6", "id, x'01'")); err == nil {
		t.Error("an image took two owners")
	}
	if err := exec(`DELETE FROM work WHERE id = x'01'`); err != nil {
		t.Fatal(err)
	}
	if got := read(`SELECT (SELECT count(*) FROM trailer WHERE work_id = x'01') || ' ' || (SELECT group_concat(hex(id), ',') FROM (SELECT id FROM image ORDER BY id))`); got != "0 E2,E3" {
		t.Errorf("after deleting the movie: %s, want no trailer and only the other owners' images", got)
	}
}

// 0076 drops the trailer columns only ranking used. Every trailer keeps its
// ID, order, video, title and thumbnail, a movie still lists a video once,
// and lists keep their order.
func TestMigrationTrailerColumns(t *testing.T) {
	ctx := context.Background()
	db, path := testDB(t, through(t, "_drop_probe_id.sql"))
	err := db.Write(ctx, func(ctx context.Context, tx *Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO user (id, username, admin, all_libraries, created_at) VALUES (x'a1', 'anna', 0, 1, 1);
			INSERT INTO list (id, user_id, name, built_in, created_at) VALUES (x'c1', x'a1', 'Watchlist', 1, 1);
			INSERT INTO work (id, kind, parent_id, root_id, title, sort_title, added_at) VALUES
				(x'01', 'movie', NULL, x'01', 'Dune', 'Dune', 1), (x'02', 'movie', NULL, x'02', 'Heat', 'Heat', 1),
				(x'03', 'movie', NULL, x'03', 'Alien', 'Alien', 1);
			INSERT INTO list_work (list_id, work_id, added_at, position) VALUES
				(x'c1', x'01', 1, 4294967296), (x'c1', x'02', 2, -4294967296), (x'c1', x'03', 3, 6442450944);
			INSERT INTO trailer (id, work_id, ord, site, key, title, language, official, published_at) VALUES
				(x'd1', x'01', 1, 'youtube', 'n9xhJrPXop4', 'Main Trailer', 'en', 1, '2021-07-22T15:00:00.000Z'),
				(x'd2', x'01', 0, 'youtube', '8g18jFHCLXk', 'Trailer', NULL, 0, NULL);
			INSERT INTO image (id, trailer_id, kind, source, origin, width, height, blurhash, fetched_at) VALUES
				(x'e1', x'd1', 'thumb', 'provider', 'https://i.ytimg.com/vi/n9xhJrPXop4/maxresdefault.jpg', 1280, 720, 'hash', 1);`)
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
	read := func(query string) (out string) {
		t.Helper()
		if err := up.Reader().QueryRowContext(ctx, query).Scan(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	if got, want := read(`SELECT group_concat(hex(t.id) || ' ' || t.ord || ' ' || t.key || ' ' || t.title || ' ' || iif(i.id IS NULL, '-', hex(i.id)), ', ')
		FROM (SELECT * FROM trailer ORDER BY ord) t LEFT JOIN image i ON i.trailer_id = t.id`),
		"D2 0 8g18jFHCLXk Trailer -, D1 1 n9xhJrPXop4 Main Trailer E1"; got != want {
		t.Errorf("trailers: %s, want %s", got, want)
	}
	if got := read(`SELECT group_concat(name, ',') FROM pragma_table_info('trailer')`); got != "id,work_id,ord,key,title" {
		t.Errorf("trailer's columns: %s", got)
	}
	if got := read(`SELECT group_concat(hex(work_id), ',') FROM (SELECT work_id FROM list_work WHERE list_id = x'c1' ORDER BY position)`); got != "02,01,03" {
		t.Errorf("the list's order: %s, want 02,01,03", got)
	}
	err = up.Write(ctx, func(ctx context.Context, tx *Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO trailer (id, work_id, ord, key, title) VALUES (x'd3', x'01', 2, 'n9xhJrPXop4', 'Again')`)
		return err
	})
	if err == nil {
		t.Error("a movie listed a video twice")
	}
}
