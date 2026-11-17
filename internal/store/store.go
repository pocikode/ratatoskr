package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"

	_ "modernc.org/sqlite" // pure-Go SQLite driver ("sqlite")
)

const (
	instanceFile = "instance.db"
	projectFile  = "project.db"
	maxReaders   = 8
)

// DB is an open SQLite database: one serialized writer plus a read pool.
type DB struct {
	path    string
	writer  *sql.DB
	readers *sql.DB
	version int
}

// OpenInstance opens (creating if needed) instance.db inside root and migrates it.
func OpenInstance(ctx context.Context, root *os.Root) (*DB, error) {
	return open(ctx, root, instanceFile, "instance")
}

// OpenProject opens (creating if needed) project.db inside root and migrates it.
func OpenProject(ctx context.Context, root *os.Root) (*DB, error) {
	return open(ctx, root, projectFile, "project")
}

func open(ctx context.Context, root *os.Root, file, kind string) (*DB, error) {
	if root == nil {
		return nil, errors.New("store: nil root")
	}
	if err := registerFuncs(); err != nil {
		return nil, err
	}
	// Create the file through the root so path traversal is impossible; the
	// driver then reopens it by path.
	f, err := root.OpenFile(file, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("store: create %s: %w", file, err)
	}
	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("store: close %s: %w", file, err)
	}
	path := filepath.Join(root.Name(), file)

	writer, err := sql.Open("sqlite", dsn(path, true))
	if err != nil {
		return nil, fmt.Errorf("store: open writer: %w", err)
	}
	writer.SetMaxOpenConns(1)
	writer.SetMaxIdleConns(1)
	writer.SetConnMaxLifetime(0)

	version, err := migrate(ctx, writer, kind)
	if err != nil {
		_ = writer.Close()
		return nil, fmt.Errorf("store: migrate %s: %w", file, err)
	}

	readers, err := sql.Open("sqlite", dsn(path, false))
	if err != nil {
		_ = writer.Close()
		return nil, fmt.Errorf("store: open readers: %w", err)
	}
	n := min(max(runtime.GOMAXPROCS(0), 2), maxReaders)
	readers.SetMaxOpenConns(n)
	readers.SetMaxIdleConns(n)
	if err := readers.PingContext(ctx); err != nil {
		_ = writer.Close()
		_ = readers.Close()
		return nil, fmt.Errorf("store: ping readers: %w", err)
	}
	return &DB{path: path, writer: writer, readers: readers, version: version}, nil
}

func dsn(path string, write bool) string {
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "foreign_keys(1)")
	if write {
		q.Add("_pragma", "journal_mode(WAL)")
		q.Add("_pragma", "synchronous(NORMAL)")
		q.Set("_txlock", "immediate")
	} else {
		q.Add("_pragma", "query_only(1)")
	}
	u := url.URL{Scheme: "file", Path: path, RawQuery: q.Encode()}
	return u.String()
}

// Path returns the database file path.
func (d *DB) Path() string { return d.path }

// SchemaVersion returns the migrated schema version.
func (d *DB) SchemaVersion() int { return d.version }

// Reader returns the read-only connection pool.
func (d *DB) Reader() *sql.DB { return d.readers }

// Close closes both handles.
func (d *DB) Close() error {
	return errors.Join(d.readers.Close(), d.writer.Close())
}

// Write runs fn in an immediate write transaction on the single writer
// connection. Writes are therefore serialized and never hit SQLITE_BUSY from
// other in-process writers. fn's error (or a panic) rolls back.
func (d *DB) Write(ctx context.Context, fn func(tx *sql.Tx) error) (err error) {
	return runTx(ctx, d.writer, false, fn)
}

// View runs fn in a read transaction (consistent snapshot) from the read pool.
func (d *DB) View(ctx context.Context, fn func(tx *sql.Tx) error) error {
	return runTx(ctx, d.readers, true, fn)
}

func runTx(ctx context.Context, db *sql.DB, readOnly bool, fn func(tx *sql.Tx) error) (err error) {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: readOnly})
	if err != nil {
		return fmt.Errorf("store: begin: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if err = fn(tx); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("store: commit: %w", err)
	}
	return nil
}
