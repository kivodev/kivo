package store

import (
	"crypto/sha256"
	"database/sql"
	"net/url"

	"github.com/ncruces/go-sqlite3"
	_ "github.com/ncruces/go-sqlite3/driver" // registers "sqlite3"
	"github.com/ncruces/go-sqlite3/ext/fts5"
)

// driverName is the name ncruces/go-sqlite3 registers. This is the only
// file that names the driver.
const driverName = "sqlite3"

func init() {
	sqlite3.AutoExtension(func(c *sqlite3.Conn) error {
		err := c.CreateFunction("sha256", -1, sqlite3.DETERMINISTIC|sqlite3.INNOCUOUS, sha256Func)
		if err != nil {
			return err
		}
		return fts5.RegisterCustom(c, func(api *fts5.API) error {
			return api.CreateFunction("match_rank", matchRankFunc)
		})
	})
}

// sha256Func is the SQL function sha256(a, ...). It returns, as a blob,
// the SHA-256 of its arguments' bytes concatenated in order. A text
// argument contributes its UTF-8 bytes.
func sha256Func(ctx sqlite3.Context, arg ...sqlite3.Value) {
	h := sha256.New()
	for _, a := range arg {
		h.Write(a.RawBlob())
	}
	ctx.ResultBlob(h.Sum(nil))
}

// matchRankFunc is the FTS5 auxiliary function match_rank(table, text,
// key). It returns the row's MatchRank, computed from the columns that
// hold every phrase of the query.
func matchRankFunc(fts fts5.Context, ctx sqlite3.Context, arg ...sqlite3.Value) {
	all := ^uint64(0)
	for p := range fts.PhraseCount() {
		if fts.ColumnCount() == 1 {
			break // every phrase is in the one column
		}
		cols, err := fts.PhraseIterColumn(p)
		if err != nil {
			ctx.ResultError(err)
			return
		}
		var in uint64
		for c := range cols {
			in |= 1 << c
		}
		all &= in
	}
	ctx.ResultInt(MatchRank(arg[0].Text(), arg[1].Text(), all))
}

// openWriter opens the writer pool. Its transactions take BEGIN IMMEDIATE,
// so a write transaction never has to upgrade its lock.
func openWriter(path string) (*sql.DB, error) {
	q := pragmas()
	q.Set("_txlock", "immediate")
	return sql.Open(driverName, fileURI(path, q))
}

// openReader opens the query_only reader pool.
func openReader(path string) (*sql.DB, error) {
	q := pragmas()
	q.Add("_pragma", "query_only(1)")
	return sql.Open(driverName, fileURI(path, q))
}

// pragmas are set in the DSN so that every pooled connection gets them.
func pragmas() url.Values {
	q := url.Values{}
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(10000)")
	q.Add("_pragma", "synchronous(NORMAL)")
	return q
}

// openReadOnly opens an existing database file read-only, outside the pools.
func openReadOnly(path string) (*sql.DB, error) {
	q := url.Values{}
	q.Set("mode", "ro")
	return sql.Open(driverName, fileURI(path, q))
}

// openExclusive opens an existing database file outside the pools to take
// an exclusive lock. The lock fails at once, rather than waiting, while
// another connection has the file open.
func openExclusive(path string) (*sql.DB, error) {
	q := url.Values{}
	q.Set("mode", "rw")
	q.Add("_pragma", "busy_timeout(0)")
	q.Add("_pragma", "locking_mode(exclusive)")
	return sql.Open(driverName, fileURI(path, q))
}

func fileURI(path string, q url.Values) string {
	return "file:" + (&url.URL{Path: path}).EscapedPath() + "?" + q.Encode()
}
