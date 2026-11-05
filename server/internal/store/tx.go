package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"testing"
	"time"
)

// slowWrite is how long a write transaction may hold the writer before
// it's logged. Every other write waits behind it.
const slowWrite = time.Second

// writeKey marks the context that Write passes to fn. Its value is the DB
// whose writer the Write holds.
type writeKey struct{}

// Tx is a write transaction, which Write hands to fn. It runs statements
// and satisfies sqlc's DBTX, so dbq.New(tx) runs generated queries in it.
// Only Write commits it or rolls it back.
type Tx struct {
	tx          *sql.Tx
	afterCommit []func()
}

// AfterCommit registers f to run once the transaction commits. f is
// dropped if the transaction rolls back.
func (tx *Tx) AfterCommit(f func()) {
	tx.afterCommit = append(tx.afterCommit, f)
}

// ExecContext runs a statement that returns no rows.
func (tx *Tx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return tx.tx.ExecContext(ctx, query, args...)
}

// QueryContext runs a query that returns rows.
func (tx *Tx) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return tx.tx.QueryContext(ctx, query, args...)
}

// QueryRowContext runs a query that returns at most one row.
func (tx *Tx) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return tx.tx.QueryRowContext(ctx, query, args...)
}

// PrepareContext prepares a statement that runs in the transaction.
func (tx *Tx) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return tx.tx.PrepareContext(ctx, query)
}

// Write runs fn in a write transaction (BEGIN IMMEDIATE) on the single
// writer connection and commits if fn returns nil. Writes are serialized,
// so keep them short and never do I/O inside fn. Once the transaction
// commits, Write runs the functions registered with tx.AfterCommit, in
// order.
//
// fn gets a context that marks the write. Pass it on, so that a nested
// Write returns ErrNestedWrite instead of deadlocking. A nested Write would
// otherwise wait forever for the connection the outer one holds. Under
// `go test` a nested Write also panics.
func (db *DB) Write(ctx context.Context, fn func(ctx context.Context, tx *Tx) error) (err error) {
	if in, ok := ctx.Value(writeKey{}).(*DB); ok && in == db {
		// Under go test a nested Write panics, so the test that makes one
		// fails even where the caller swallows the error.
		if testing.Testing() {
			panic(ErrNestedWrite)
		}
		return ErrNestedWrite
	}

	db.writeMu.Lock()
	held := time.Now()
	defer func() {
		db.writeMu.Unlock()
		if d := time.Since(held); d > slowWrite {
			_, file, line, _ := runtime.Caller(2)
			slog.WarnContext(ctx, "store: slow write", "held", d.Round(time.Millisecond), "caller", fmt.Sprintf("%s:%d", file, line))
		}
	}()

	sqlTx, err := db.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin write: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = sqlTx.Rollback()
			panic(p)
		}
		if err != nil {
			_ = sqlTx.Rollback()
		}
	}()

	tx := &Tx{tx: sqlTx}
	if err = fn(context.WithValue(ctx, writeKey{}, db), tx); err != nil {
		return err
	}
	if err = commit(ctx, sqlTx); err != nil {
		return err
	}
	for _, f := range tx.afterCommit {
		f()
	}
	return nil
}

// commit commits a write's transaction.
func commit(ctx context.Context, tx *sql.Tx) error {
	err := tx.Commit()
	// When ctx ends, database/sql rolls the transaction back itself. If
	// that happens first, Commit finds the transaction already done, and
	// ctx's error says what happened.
	if cerr := ctx.Err(); err != nil && cerr != nil && errors.Is(err, sql.ErrTxDone) {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("commit write: %w", err)
	}
	return nil
}

// Read runs fn in a read-only transaction, for several queries that must see
// one consistent snapshot. Single statements can use Reader directly.
func (db *DB) Read(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := db.reader.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return fmt.Errorf("begin read: %w", err)
	}
	err = fn(tx)
	// When ctx ends, database/sql rolls the transaction back itself, so
	// Rollback then finds it already done. fn's error says what happened.
	if rerr := tx.Rollback(); !errors.Is(rerr, sql.ErrTxDone) {
		err = errors.Join(err, rerr)
	}
	return err
}
