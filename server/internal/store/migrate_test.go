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
