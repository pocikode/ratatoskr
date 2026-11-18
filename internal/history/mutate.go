package history

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/pocikode/ratatoskr/internal/apperr"
)

// SetColor sets, or with an empty color clears, the colour of each request.
// An unknown ID is [apperr.NotFound] and nothing changes. It returns the
// updated requests in ids order and announces each as Updated.
func (s *Service) SetColor(ctx context.Context, ids []int64, color string) ([]Request, error) {
	if color != "" && !slices.Contains(Colors, color) {
		return nil, apperr.New(apperr.BadInput, "unknown colour %q", color)
	}

	var v any
	if color != "" {
		v = color
	}

	return s.update(ctx, ids, "color", v)
}

// SetNote sets the trimmed note of a request; an empty note clears it.
func (s *Service) SetNote(ctx context.Context, id int64, note string) (Request, error) {
	var v any
	if note = strings.TrimSpace(note); note != "" {
		v = note
	}

	rs, err := s.update(ctx, []int64{id}, "note", v)
	if err != nil {
		return Request{}, err
	}

	return rs[0], nil
}

// update sets one column (a constant name, never client input) on every id in
// one transaction.
func (s *Service) update(ctx context.Context, ids []int64, col string, v any) ([]Request, error) {
	db, err := s.db()
	if err != nil {
		return nil, err
	}

	err = db.Write(ctx, func(tx *sql.Tx) error {
		for _, id := range ids {
			res, err := tx.ExecContext(ctx, `UPDATE requests SET `+col+` = ? WHERE id = ?`, v, id) //nolint:gosec // T-016: col is a constant of this package
			if err != nil {
				return fmt.Errorf("history: set %s: %w", col, err)
			}

			if n, _ := res.RowsAffected(); n == 0 {
				return apperr.New(apperr.NotFound, "request %d not found", id)
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	out := make([]Request, 0, len(ids))

	for _, id := range ids {
		r, err := get(ctx, db.Reader(), id)
		if err != nil {
			return nil, err
		}

		out = append(out, r)
		s.publish(Event{Kind: Updated, RequestID: id})
	}

	return out, nil
}

// DeleteArgs select what [Service.Delete] removes: the requests in IDs, or
// everything matching Query (an empty query matches all) within ScopeID.
type DeleteArgs struct {
	IDs     []int64 // set exactly one of IDs and Query
	Query   *string
	ScopeID *string
}

// Delete removes requests; responses and headers follow through the foreign
// keys. Records edited from a deleted request keep existing. It returns the
// number deleted and announces them in Deleted events of at most MaxPage IDs.
func (s *Service) Delete(ctx context.Context, a DeleteArgs) (int64, error) {
	if (a.IDs != nil) == (a.Query != nil) {
		return 0, apperr.New(apperr.BadInput, "give exactly one of ids and query")
	}

	var (
		f   filter
		err error
	)

	if a.IDs == nil {
		if f, err = s.filter(ctx, a.Query, a.ScopeID); err != nil {
			return 0, err
		}
	}

	db, err := s.db()
	if err != nil {
		return 0, err
	}

	var deleted []int64

	err = db.Write(ctx, func(tx *sql.Tx) error {
		deleted = deleted[:0]

		if a.IDs == nil {
			rows, err := tx.QueryContext(ctx, `SELECT r.id`+fromJoin+` WHERE `+f.where(), f.sql.Args...) //nolint:gosec // T-016: compiled by httpql with bind parameters
			if err != nil {
				return fmt.Errorf("history: select deletions: %w", err)
			}

			defer rows.Close()

			for rows.Next() {
				var id int64
				if err := rows.Scan(&id); err != nil {
					return fmt.Errorf("history: select deletions: %w", err)
				}

				deleted = append(deleted, id)
			}

			if err := rows.Err(); err != nil {
				return fmt.Errorf("history: select deletions: %w", err)
			}
		} else {
			// Unknown IDs are ignored: keep the ones that exist.
			for chunk := range slices.Chunk(a.IDs, 500) {
				rows, err := tx.QueryContext(ctx, `SELECT id FROM requests WHERE id IN (`+placeholders(len(chunk))+`)`, anys(chunk)...) //nolint:gosec // T-016: placeholders only
				if err != nil {
					return fmt.Errorf("history: select deletions: %w", err)
				}

				for rows.Next() {
					var id int64
					if err := rows.Scan(&id); err != nil {
						return errors.Join(fmt.Errorf("history: select deletions: %w", err), rows.Close())
					}

					deleted = append(deleted, id)
				}

				if err := rows.Close(); err != nil {
					return fmt.Errorf("history: select deletions: %w", err)
				}
			}
		}

		for chunk := range slices.Chunk(deleted, 500) {
			if _, err := tx.ExecContext(ctx, `DELETE FROM requests WHERE id IN (`+placeholders(len(chunk))+`)`, anys(chunk)...); err != nil { //nolint:gosec // T-016: placeholders only
				return fmt.Errorf("history: delete: %w", err)
			}
		}

		return nil
	})
	if err != nil {
		return 0, err
	}

	for chunk := range slices.Chunk(deleted, MaxPage) {
		s.publish(Event{Kind: Deleted, IDs: slices.Clone(chunk)})
	}

	return int64(len(deleted)), nil
}

func anys(ids []int64) []any {
	out := make([]any, len(ids))
	for i, id := range ids {
		out[i] = id
	}

	return out
}
