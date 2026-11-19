package store

import (
	"context"
	"database/sql"
	"fmt"
)

// Request is a row of the requests table used by the batch insert helper.
type Request struct {
	ID               int64 // zero allocates an ID in SQLite
	Source           string
	Host             string
	Port             int
	TLS              bool
	Method           string
	Path             string
	Query            string
	Ext              string
	Raw              []byte
	BodyOffset       int
	CreatedAt        int64 // unix milliseconds
	PluginID         *string
	AutomateAttackID *int64
	Truncated        bool // request bytes were cut; response truncation is recorded later
	Dropped          bool
	EditedFrom       *int64
}

const insertRequestSQL = `INSERT INTO requests
	(id, source, host, port, tls, method, path, query, ext, raw, body_offset, len,
	 created_at, truncated, request_truncated, is_dropped, edited_from, plugin_id, automate_attack_id)
	VALUES (NULLIF(?, 0), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

// InsertRequestTx inserts one row inside tx and returns its id.
func InsertRequestTx(ctx context.Context, tx *sql.Tx, r Request) (int64, error) {
	if r.EditedFrom != nil {
		var exists bool
		if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM requests WHERE edited_from = ?)", *r.EditedFrom).Scan(&exists); err != nil {
			return 0, fmt.Errorf("store: check edited request: %w", err)
		}
		if exists {
			return 0, fmt.Errorf("store: request %d already has an edited record", *r.EditedFrom)
		}
	}

	res, err := tx.ExecContext(ctx, insertRequestSQL, r.ID, r.Source, r.Host, r.Port, r.TLS, r.Method, r.Path,
		r.Query, r.Ext, r.Raw, r.BodyOffset, len(r.Raw), r.CreatedAt, r.Truncated, r.Truncated,
		r.Dropped, r.EditedFrom, r.PluginID, r.AutomateAttackID)
	if err != nil {
		return 0, fmt.Errorf("store: insert request: %w", err)
	}

	if r.ID != 0 {
		return r.ID, nil
	}

	return res.LastInsertId()
}

// InsertRequests inserts rows in a single transaction and returns their new
// ids in order.
func (d *DB) InsertRequests(ctx context.Context, rows []Request) ([]int64, error) {
	ids := make([]int64, 0, len(rows))
	err := d.Write(ctx, func(tx *sql.Tx) error {
		for i := range rows {
			id, err := InsertRequestTx(ctx, tx, rows[i])
			if err != nil {
				return fmt.Errorf("request %d: %w", i, err)
			}
			ids = append(ids, id)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ids, nil
}
