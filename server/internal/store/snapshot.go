package store

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Snapshot writes a consistent copy of the database to dst. It checks for
// free space of twice the database size and runs quick_check. It then
// runs VACUUM INTO a temp file on its own short-lived connection, and
// renames the file into place.
func (db *DB) Snapshot(ctx context.Context, dst string) error {
	size, err := fileSize(db.path)
	if err != nil {
		return err
	}
	if free, ok := freeBytes(filepath.Dir(dst)); ok && free < 2*size {
		return fmt.Errorf("%w: snapshot needs %d bytes at %s, %d free", ErrLowSpace, 2*size, filepath.Dir(dst), free)
	}

	var check string
	if err := db.reader.QueryRowContext(ctx, `PRAGMA quick_check`).Scan(&check); err != nil {
		return err
	}
	if check != "ok" {
		return fmt.Errorf("%s: quick_check: %s", db.path, check)
	}

	tmp := dst + ".tmp"
	_ = os.Remove(tmp)
	conn, err := openWriter(db.path)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `VACUUM INTO ?`, tmp); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

func fileSize(path string) (int64, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}

const backupPrefix, backupSuffix = "kivo-", ".db"

// Backup snapshots the database into dir as kivo-YYYYMMDD-HHMMSS.mmm.db and
// keeps the newest keep backups there.
func (db *DB) Backup(ctx context.Context, dir string, keep int) (string, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}
	name := backupPrefix + time.Now().UTC().Format("20060102-150405.000") + backupSuffix
	dst := filepath.Join(dir, name)
	if err := db.Snapshot(ctx, dst); err != nil {
		return "", err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return dst, err
	}
	var backups []string
	for _, e := range entries {
		if n := e.Name(); strings.HasPrefix(n, backupPrefix) && strings.HasSuffix(n, backupSuffix) {
			backups = append(backups, n)
		}
	}
	slices.Sort(backups) // the timestamp format sorts chronologically
	var errs []error
	for len(backups) > max(keep, 1) {
		errs = append(errs, os.Remove(filepath.Join(dir, backups[0])))
		backups = backups[1:]
	}
	return dst, errors.Join(errs...)
}

// Restore replaces the database at path with a snapshot, such as the
// rollback snapshot taken before an upgrade or a nightly backup. It holds
// an exclusive lock on the database throughout. It refuses with ErrInUse
// while anything else has the database open, as the running server does.
// It also refuses a snapshot that fails quick_check. The old WAL is
// removed, because replaying it onto the snapshot would corrupt it.
func Restore(ctx context.Context, path, snapshot string) error {
	if _, err := os.Stat(path); err == nil {
		lock, err := openExclusive(path)
		if err != nil {
			return err
		}
		defer lock.Close()
		conn, err := lock.Conn(ctx)
		if err == nil {
			defer conn.Close()
			_, err = conn.ExecContext(ctx, `BEGIN EXCLUSIVE`)
		}
		if busy(err) {
			return ErrInUse
		}
		if err != nil {
			return err
		}
	}
	if err := quickCheck(ctx, snapshot); err != nil {
		return err
	}
	// The copy is synced beside the database before it's renamed over it.
	tmp := path + ".restoring"
	if err := copySynced(snapshot, tmp); err != nil {
		os.Remove(tmp)
		return err
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.Remove(path + suffix); err != nil && !errors.Is(err, fs.ErrNotExist) {
			os.Remove(tmp)
			return err
		}
	}
	return os.Rename(tmp, path)
}

// quickCheck runs quick_check on a database file, read-only.
func quickCheck(ctx context.Context, path string) error {
	if _, err := os.Stat(path); err != nil {
		return err
	}
	db, err := openReadOnly(path)
	if err != nil {
		return err
	}
	defer db.Close()
	var check string
	if err := db.QueryRowContext(ctx, `PRAGMA quick_check`).Scan(&check); err != nil {
		return err
	}
	if check != "ok" {
		return fmt.Errorf("%s: quick_check: %s", path, check)
	}
	return nil
}

// copySynced copies src to a new file dst and syncs it to disk.
func copySynced(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o640)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if err == nil {
		err = out.Sync()
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}
