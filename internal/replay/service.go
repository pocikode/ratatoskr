// Package replay persists HTTP Replay collections, sessions and entries per project.
package replay

import (
	"context"
	"database/sql"
	"encoding/json/v2"
	"errors"
	"time"

	"github.com/pocikode/ratatoskr/internal/apperr"
	"github.com/pocikode/ratatoskr/internal/store"
)

// Collection holds sessions in display order.
type Collection struct {
	ID        int64
	Name      string
	IsDefault bool
}

// Connection is the destination independent of the draft's Host header.
type Connection struct {
	Host string
	Port int
	TLS  bool
	SNI  *string
}

// Options are the persisted send options.
type Options struct {
	FollowRedirects     bool
	UpdateContentLength bool
	TimeoutMs           int
	ApplyMatchReplace   bool
}

// State is stored in replay_sessions.state_json; Draft is a separate BLOB.
type State struct {
	Connection Connection
	Options    Options
	Color      string
}

// Session is an HTTP draft with its destination and options.
type Session struct {
	ID           int64
	CollectionID int64
	Name         string
	State
	Draft     []byte
	CreatedAt time.Time
}

// Service uses the selected project, captured once for each operation.
type Service struct{ currentDB func() *store.DB }

// New returns a service using project.Service.DB.
func New(db func() *store.DB) *Service { return &Service{currentDB: db} }

func (s *Service) db() (*store.DB, error) {
	if s.currentDB != nil {
		if db := s.currentDB(); db != nil {
			return db, nil
		}
	}
	return nil, apperr.New(apperr.Conflict, "no project is open")
}

func ensureDefault(ctx context.Context, tx *sql.Tx) (Collection, error) {
	_, err := tx.ExecContext(ctx, `INSERT INTO replay_collections(name, sort_order, is_default)
 SELECT 'Default', coalesce(max(sort_order), -1) + 1, 1 FROM replay_collections
 HAVING NOT EXISTS (SELECT 1 FROM replay_collections WHERE is_default = 1)`)
	if err != nil {
		return Collection{}, err
	}
	var c Collection
	err = tx.QueryRowContext(ctx, `SELECT id, name, is_default FROM replay_collections WHERE is_default = 1`).Scan(&c.ID, &c.Name, &c.IsDefault)
	return c, err
}

func collection(ctx context.Context, tx *sql.Tx, id int64) (Collection, error) {
	var c Collection
	err := tx.QueryRowContext(ctx, `SELECT id, name, is_default FROM replay_collections WHERE id = ?`, id).Scan(&c.ID, &c.Name, &c.IsDefault)
	if errors.Is(err, sql.ErrNoRows) {
		err = apperr.New(apperr.NotFound, "replay collection %d not found", id)
	}
	return c, err
}

func collections(ctx context.Context, tx *sql.Tx) ([]Collection, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, name, is_default FROM replay_collections ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Collection{}
	for rows.Next() {
		var c Collection
		if err := rows.Scan(&c.ID, &c.Name, &c.IsDefault); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Collections lists collections, lazily creating Default in a serialized transaction.
func (s *Service) Collections(ctx context.Context) (out []Collection, err error) {
	db, err := s.db()
	if err != nil {
		return nil, err
	}
	err = db.Write(ctx, func(tx *sql.Tx) error {
		if _, err := ensureDefault(ctx, tx); err != nil {
			return err
		}
		out, err = collections(ctx, tx)
		return err
	})
	return out, err
}

// Collection returns a collection or NOT_FOUND.
func (s *Service) Collection(ctx context.Context, id int64) (out Collection, err error) {
	db, err := s.db()
	if err != nil {
		return out, err
	}
	err = db.View(ctx, func(tx *sql.Tx) error { out, err = collection(ctx, tx, id); return err })
	return out, err
}

// CreateCollection appends a collection after ensuring Default exists.
func (s *Service) CreateCollection(ctx context.Context, name string) (out Collection, err error) {
	name, err = cleanName(name)
	if err != nil {
		return out, err
	}
	db, err := s.db()
	if err != nil {
		return out, err
	}
	err = db.Write(ctx, func(tx *sql.Tx) error {
		if _, err := ensureDefault(ctx, tx); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO replay_collections(name, sort_order) SELECT ?, coalesce(max(sort_order), -1) + 1 FROM replay_collections`, name)
		if err != nil {
			return err
		}
		out.Name = name
		out.ID, err = res.LastInsertId()
		return err
	})
	return out, err
}

// RenameCollection changes a collection's name, including Default's.
func (s *Service) RenameCollection(ctx context.Context, id int64, name string) (out Collection, err error) {
	name, err = cleanName(name)
	if err != nil {
		return out, err
	}
	db, err := s.db()
	if err != nil {
		return out, err
	}
	err = db.Write(ctx, func(tx *sql.Tx) error {
		out, err = collection(ctx, tx, id)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE replay_collections SET name = ? WHERE id = ?`, name, id)
		out.Name = name
		return err
	})
	return out, err
}

// DeleteCollection deletes its sessions and entries, retaining history records.
func (s *Service) DeleteCollection(ctx context.Context, id int64) error {
	db, err := s.db()
	if err != nil {
		return err
	}
	return db.Write(ctx, func(tx *sql.Tx) error {
		c, err := collection(ctx, tx, id)
		if err != nil {
			return err
		}
		if c.IsDefault {
			return apperr.New(apperr.Conflict, "cannot delete the Default collection")
		}
		// The original schema uses SET NULL on this FK; delete children explicitly.
		if _, err := tx.ExecContext(ctx, `DELETE FROM replay_sessions WHERE collection_id = ?`, id); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM replay_collections WHERE id = ?`, id)
		return err
	})
}

// ReorderCollections atomically requires and applies the exact current set of IDs.
func (s *Service) ReorderCollections(ctx context.Context, ids []int64) (out []Collection, err error) {
	db, err := s.db()
	if err != nil {
		return nil, err
	}
	err = db.Write(ctx, func(tx *sql.Tx) error {
		if _, err := ensureDefault(ctx, tx); err != nil {
			return err
		}
		all, err := collections(ctx, tx)
		if err != nil {
			return err
		}
		seen := map[int64]bool{}
		for _, id := range ids {
			seen[id] = true
		}
		if len(ids) != len(all) || len(seen) != len(all) {
			return apperr.New(apperr.BadInput, "ids must list every collection exactly once")
		}
		for _, c := range all {
			if !seen[c.ID] {
				return apperr.New(apperr.BadInput, "ids must list every collection exactly once")
			}
		}
		for i, id := range ids {
			if _, err := tx.ExecContext(ctx, `UPDATE replay_collections SET sort_order = ? WHERE id = ?`, i, id); err != nil {
				return err
			}
		}
		out, err = collections(ctx, tx)
		return err
	})
	return out, err
}

const sessionColumns = `id, collection_id, name, state_json, draft, created_at`

func scanSession(row interface{ Scan(...any) error }) (Session, error) {
	var out Session
	var state string
	var created int64
	err := row.Scan(&out.ID, &out.CollectionID, &out.Name, &state, &out.Draft, &created)
	if err != nil {
		return out, err
	}
	if out.Draft == nil {
		out.Draft = []byte{}
	}
	out.Options = Options{UpdateContentLength: true, TimeoutMs: 60000}
	out.CreatedAt = time.UnixMilli(created).UTC()
	err = json.Unmarshal([]byte(state), &out.State)
	return out, err
}

func session(ctx context.Context, tx *sql.Tx, id int64) (Session, error) {
	out, err := scanSession(tx.QueryRowContext(ctx, `SELECT `+sessionColumns+` FROM replay_sessions WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		err = apperr.New(apperr.NotFound, "replay session %d not found", id)
	}
	return out, err
}

// Session returns one session or NOT_FOUND.
func (s *Service) Session(ctx context.Context, id int64) (out Session, err error) {
	db, err := s.db()
	if err != nil {
		return out, err
	}
	err = db.View(ctx, func(tx *sql.Tx) error { out, err = session(ctx, tx, id); return err })
	return out, err
}

// Sessions lists a collection's sessions in position order.
func (s *Service) Sessions(ctx context.Context, collectionID int64) (out []Session, err error) {
	db, err := s.db()
	if err != nil {
		return nil, err
	}
	err = db.View(ctx, func(tx *sql.Tx) error {
		if _, err := collection(ctx, tx, collectionID); err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, `SELECT `+sessionColumns+` FROM replay_sessions WHERE collection_id = ? ORDER BY sort_order, id`, collectionID)
		if err != nil {
			return err
		}
		defer rows.Close()
		out = []Session{}
		for rows.Next() {
			item, err := scanSession(rows)
			if err != nil {
				return err
			}
			out = append(out, item)
		}
		return rows.Err()
	})
	return out, err
}
