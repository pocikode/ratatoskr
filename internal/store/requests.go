package store

import (
	"context"
	"database/sql"
	"fmt"
)

// Request is a row of the requests table used by the batch insert helper.
type Request struct {
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
	Truncated        bool
}

const insertRequestSQL = `INSERT INTO requests
	(source, host, port, tls, method, path, query, ext, raw, body_offset, len,
	 created_at, truncated, plugin_id, automate_attack_id)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

// InsertRequests inserts rows in a single transaction with one prepared
// statement and returns their new ids in order.
func (d *DB) InsertRequests(ctx context.Context, rows []Request) ([]int64, error) {
	ids := make([]int64, 0, len(rows))
	err := d.Write(ctx, func(tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, insertRequestSQL)
		if err != nil {
			return fmt.Errorf("store: prepare: %w", err)
		}
		defer stmt.Close()
		for i := range rows {
			r := &rows[i]
			res, err := stmt.ExecContext(ctx, r.Source, r.Host, r.Port, r.TLS, r.Method, r.Path,
				r.Query, r.Ext, r.Raw, r.BodyOffset, len(r.Raw), r.CreatedAt, r.Truncated,
				r.PluginID, r.AutomateAttackID)
			if err != nil {
				return fmt.Errorf("store: insert request %d: %w", i, err)
			}
			id, err := res.LastInsertId()
			if err != nil {
				return err
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
