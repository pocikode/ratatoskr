package project

import (
	"context"
	"database/sql"
	"errors"
)

// rowCols is the column list scanRow expects.
const rowCols = `id, name, created_at, last_opened_at, is_default, is_temporary`

type row struct {
	id         int64
	name       string
	created    int64
	lastOpened sql.NullInt64
	def        bool
	temp       bool
}

type scanner interface{ Scan(dest ...any) error }

func scanRow(sc scanner) (row, error) {
	var r row

	err := sc.Scan(&r.id, &r.name, &r.created, &r.lastOpened, &r.def, &r.temp)

	return r, err
}

func getRow(ctx context.Context, tx *sql.Tx, id int64) (row, error) {
	r, err := scanRow(tx.QueryRowContext(ctx, `SELECT `+rowCols+` FROM projects WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return row{}, errNotFound(id)
	}

	return r, err
}
