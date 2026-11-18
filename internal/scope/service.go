package scope

import (
	"context"
	"database/sql"
	"encoding/json/v2"
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/pocikode/ratatoskr/internal/apperr"
	"github.com/pocikode/ratatoskr/internal/store"
)

const maxEntries = 1000

// Preset is a named scope.
type Preset struct {
	ID    int64
	Name  string
	Allow []string
	Deny  []string
}

// Matcher compiles the preset's lists.
func (p Preset) Matcher() (*Matcher, error) { return NewMatcher(p.Allow, p.Deny) }

// Service stores scope presets in the selected project's database.
type Service struct{ db func() *store.DB }

// New returns a Service; db yields the current project database (project.Service.DB).
func New(db func() *store.DB) *Service { return &Service{db: db} }

func clean(p Preset) (Preset, error) {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" || utf8.RuneCountInString(p.Name) > 200 {
		return p, apperr.New(apperr.BadInput, "scope name must be 1-200 characters")
	}

	if len(p.Allow) > maxEntries || len(p.Deny) > maxEntries {
		return p, apperr.New(apperr.BadInput, "at most %d entries per list", maxEntries)
	}

	if _, err := NewMatcher(p.Allow, p.Deny); err != nil {
		return p, apperr.New(apperr.BadInput, "%v", err)
	}

	p.Allow, p.Deny = nonNil(p.Allow), nonNil(p.Deny)

	return p, nil
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}

	return s
}

func scan(sc interface{ Scan(...any) error }) (Preset, error) {
	var (
		p    Preset
		a, d string
	)

	if err := sc.Scan(&p.ID, &p.Name, &a, &d); err != nil {
		return p, err
	}

	if err := json.Unmarshal([]byte(a), &p.Allow); err != nil {
		return p, err
	}

	return p, json.Unmarshal([]byte(d), &p.Deny)
}

const cols = `id, name, allow_json, deny_json`

// List returns all presets in creation order.
func (s *Service) List(ctx context.Context) ([]Preset, error) {
	var out []Preset

	err := s.db().View(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT `+cols+` FROM scope_presets ORDER BY id`)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			p, err := scan(rows)
			if err != nil {
				return err
			}

			out = append(out, p)
		}

		return rows.Err()
	})

	return out, err
}

// Get returns one preset or a NOT_FOUND error.
func (s *Service) Get(ctx context.Context, id int64) (p Preset, err error) {
	err = s.db().View(ctx, func(tx *sql.Tx) error {
		p, err = get(ctx, tx, id)
		return err
	})

	return p, err
}

func get(ctx context.Context, tx *sql.Tx, id int64) (Preset, error) {
	p, err := scan(tx.QueryRowContext(ctx, `SELECT `+cols+` FROM scope_presets WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return p, apperr.New(apperr.NotFound, "scope %d not found", id)
	}

	return p, err
}

func nameTaken(ctx context.Context, tx *sql.Tx, name string, except int64) error {
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM scope_presets WHERE name = ? AND id <> ?`, name, except).Scan(&n); err != nil {
		return err
	}

	if n > 0 {
		return apperr.New(apperr.Conflict, "a scope named %q already exists", name)
	}

	return nil
}

func encode(p Preset) (a, d []byte, err error) {
	if a, err = json.Marshal(p.Allow); err != nil {
		return nil, nil, err
	}

	d, err = json.Marshal(p.Deny)

	return a, d, err
}

// Create adds a preset; ID in p is ignored.
func (s *Service) Create(ctx context.Context, p Preset) (Preset, error) {
	p, err := clean(p)
	if err != nil {
		return p, err
	}

	a, d, err := encode(p)
	if err != nil {
		return p, err
	}

	err = s.db().Write(ctx, func(tx *sql.Tx) error {
		if err := nameTaken(ctx, tx, p.Name, 0); err != nil {
			return err
		}

		res, err := tx.ExecContext(ctx, `INSERT INTO scope_presets(name, allow_json, deny_json) VALUES (?, ?, ?)`, p.Name, string(a), string(d))
		if err != nil {
			return err
		}

		p.ID, err = res.LastInsertId()

		return err
	})

	return p, err
}

// Update replaces the preset with p.ID.
func (s *Service) Update(ctx context.Context, p Preset) (Preset, error) {
	p, err := clean(p)
	if err != nil {
		return p, err
	}

	a, d, err := encode(p)
	if err != nil {
		return p, err
	}

	err = s.db().Write(ctx, func(tx *sql.Tx) error {
		if _, err := get(ctx, tx, p.ID); err != nil {
			return err
		}

		if err := nameTaken(ctx, tx, p.Name, p.ID); err != nil {
			return err
		}

		_, err := tx.ExecContext(ctx, `UPDATE scope_presets SET name = ?, allow_json = ?, deny_json = ? WHERE id = ?`, p.Name, string(a), string(d), p.ID)

		return err
	})

	return p, err
}

// Duplicate copies a preset under "<name> (copy)", "<name> (copy 2)", ...
func (s *Service) Duplicate(ctx context.Context, id int64) (out Preset, err error) {
	err = s.db().Write(ctx, func(tx *sql.Tx) error {
		src, err := get(ctx, tx, id)
		if err != nil {
			return err
		}

		out = Preset{Name: src.Name + " (copy)", Allow: src.Allow, Deny: src.Deny}
		for i := 2; nameTaken(ctx, tx, out.Name, 0) != nil; i++ {
			out.Name = src.Name + " (copy " + strconv.Itoa(i) + ")"
		}

		a, d, err := encode(out)
		if err != nil {
			return err
		}

		res, err := tx.ExecContext(ctx, `INSERT INTO scope_presets(name, allow_json, deny_json) VALUES (?, ?, ?)`, out.Name, string(a), string(d))
		if err != nil {
			return err
		}

		out.ID, err = res.LastInsertId()

		return err
	})

	return out, err
}

// Delete removes a preset.
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.db().Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM scope_presets WHERE id = ?`, id)
		if err != nil {
			return err
		}

		if n, _ := res.RowsAffected(); n == 0 {
			return apperr.New(apperr.NotFound, "scope %d not found", id)
		}

		return nil
	})
}
