package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"
)

//go:embed migrations/instance/*.sql migrations/project/*.sql
var migrationsFS embed.FS

// ErrNewerSchema is returned when a database was written by a newer Ratatoskr.
var ErrNewerSchema = errors.New("database schema is newer than this version of Ratatoskr supports")

// NewerSchemaError carries the versions involved; it matches ErrNewerSchema.
type NewerSchemaError struct{ Have, Supported int }

func (e *NewerSchemaError) Error() string {
	return fmt.Sprintf("database schema version %d is newer than supported version %d; upgrade Ratatoskr", e.Have, e.Supported)
}

// Is reports a match with ErrNewerSchema.
func (e *NewerSchemaError) Is(target error) bool { return target == ErrNewerSchema }

type migration struct {
	version int
	name    string
	sql     string
}

func loadMigrations(kind string) ([]migration, error) {
	dir := path.Join("migrations", kind)
	entries, err := fs.ReadDir(migrationsFS, dir)
	if err != nil {
		return nil, err
	}
	var out []migration
	for _, e := range entries {
		num, _, ok := strings.Cut(e.Name(), "_")
		v, err := strconv.Atoi(num)
		if !ok || err != nil || v <= 0 {
			return nil, fmt.Errorf("bad migration name %q", e.Name())
		}
		b, err := fs.ReadFile(migrationsFS, path.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		out = append(out, migration{version: v, name: e.Name(), sql: string(b)})
	}
	slices.SortFunc(out, func(a, b migration) int { return a.version - b.version })
	for i, m := range out {
		if m.version != i+1 {
			return nil, fmt.Errorf("migration versions must be contiguous from 1, got %s at position %d", m.name, i+1)
		}
	}
	return out, nil
}

// migrate applies pending migrations and returns the resulting version.
func migrate(ctx context.Context, db *sql.DB, kind string) (int, error) {
	migs, err := loadMigrations(kind)
	if err != nil {
		return 0, err
	}
	latest := len(migs)

	conn, err := db.Conn(ctx)
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_version (
		version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at INTEGER NOT NULL) STRICT`); err != nil {
		return 0, err
	}
	var cur int
	if err := conn.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&cur); err != nil {
		return 0, err
	}
	if cur > latest {
		return 0, &NewerSchemaError{Have: cur, Supported: latest}
	}
	for _, m := range migs[cur:] {
		if err := applyMigration(ctx, conn, m); err != nil {
			return 0, fmt.Errorf("%s: %w", m.name, err)
		}
	}
	return latest, nil
}

func applyMigration(ctx context.Context, conn *sql.Conn, m migration) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, m.sql); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_version (version, name, applied_at) VALUES (?, ?, ?)`,
		m.version, m.name, time.Now().Unix()); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
