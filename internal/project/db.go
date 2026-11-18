package project

import (
	"context"
	"database/sql"
	"encoding/json/v2"
	"errors"
)

type row struct {
	id         int64
	name       string
	created    int64
	lastOpened sql.NullInt64
	def        bool
}

type scanner interface{ Scan(dest ...any) error }

func scanRow(sc scanner) (row, error) {
	var r row

	err := sc.Scan(&r.id, &r.name, &r.created, &r.lastOpened, &r.def)

	return r, err
}

func getRow(ctx context.Context, tx *sql.Tx, id int64) (row, error) {
	r, err := scanRow(tx.QueryRowContext(ctx,
		`SELECT id, name, created_at, last_opened_at, is_default FROM projects WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return row{}, errNotFound(id)
	}

	return r, err
}

func loadTemp(ctx context.Context, tx *sql.Tx) (map[int64]bool, error) {
	var raw string

	err := tx.QueryRowContext(ctx, `SELECT value_json FROM settings WHERE key = ?`, tempKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return map[int64]bool{}, nil
	}

	if err != nil {
		return nil, err
	}

	var ids []int64
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return nil, err
	}

	set := make(map[int64]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}

	return set, nil
}

func saveTemp(ctx context.Context, tx *sql.Tx, set map[int64]bool) error {
	ids := make([]int64, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}

	raw, err := json.Marshal(ids)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `INSERT INTO settings (key, value_json) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json`, tempKey, string(raw))

	return err
}
