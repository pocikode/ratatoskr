// Package filterpreset stores HTTPQL filter presets, referenced from queries
// as preset:"Name" or preset:alias (docs/01-feature-spec.md F-FILTER-1).
package filterpreset

import (
	"context"
	"database/sql"
	"encoding/json/v2"
	"errors"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/pocikode/ratatoskr/internal/apperr"
	"github.com/pocikode/ratatoskr/internal/query"
	"github.com/pocikode/ratatoskr/internal/query/core"
	"github.com/pocikode/ratatoskr/internal/query/httpql"
	"github.com/pocikode/ratatoskr/internal/store"
)

const maxPresets = 1000

// Preset is a named, optionally aliased HTTPQL query.
type Preset struct {
	ID    int64  `json:"-"`
	Name  string `json:"name"`
	Alias string `json:"alias,omitzero"`
	Query string `json:"query"`
}

// Service stores filter presets in the selected project's database.
type Service struct{ db func() *store.DB }

// New returns a Service; db yields the current project database (project.Service.DB).
func New(db func() *store.DB) *Service { return &Service{db: db} }

const cols = `id, name, coalesce(alias, ''), clause`

func scan(sc interface{ Scan(...any) error }) (Preset, error) {
	var p Preset
	err := sc.Scan(&p.ID, &p.Name, &p.Alias, &p.Query)

	return p, err
}

func list(ctx context.Context, tx *sql.Tx) ([]Preset, error) {
	rows, err := tx.QueryContext(ctx, `SELECT `+cols+` FROM filter_presets ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Preset

	for rows.Next() {
		p, err := scan(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, p)
	}

	return out, rows.Err()
}

// List returns all presets in display order.
func (s *Service) List(ctx context.Context) (out []Preset, err error) {
	err = s.db().View(ctx, func(tx *sql.Tx) (err error) {
		out, err = list(ctx, tx)
		return err
	})

	return out, err
}

// Get returns one preset or a NOT_FOUND error.
func (s *Service) Get(ctx context.Context, id int64) (p Preset, err error) {
	err = s.db().View(ctx, func(tx *sql.Tx) (err error) {
		p, err = get(ctx, tx, id)
		return err
	})

	return p, err
}

func get(ctx context.Context, tx *sql.Tx, id int64) (Preset, error) {
	p, err := scan(tx.QueryRowContext(ctx, `SELECT `+cols+` FROM filter_presets WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return p, apperr.New(apperr.NotFound, "filter preset %d not found", id)
	}

	return p, err
}

// validAlias reports whether s is lowercase-kebab: [a-z0-9][a-z0-9_-]*.
func validAlias(s string) bool {
	for i := range len(s) {
		c := s[i]
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || i > 0 && (c == '-' || c == '_') {
			continue
		}

		return false
	}

	return s != ""
}

func clean(p Preset) (Preset, error) {
	p.Name, p.Alias, p.Query = strings.TrimSpace(p.Name), strings.TrimSpace(p.Alias), strings.TrimSpace(p.Query)
	if p.Name == "" || utf8.RuneCountInString(p.Name) > 200 {
		return p, apperr.New(apperr.BadInput, "preset name must be 1-200 characters")
	}

	if p.Alias != "" && !validAlias(p.Alias) {
		return p, apperr.New(apperr.BadInput, "alias %q must be lowercase letters, digits, - or _", p.Alias)
	}

	if p.Query == "" {
		return p, apperr.New(apperr.BadInput, "preset query must not be empty")
	}

	return p, nil
}

// lookup resolves a preset: reference by exact name, then by alias. The key
// is the id, so name and alias of one preset count as one in cycle checks.
func lookup(all []Preset) query.PresetLookup {
	return func(ref string) (string, string, bool) {
		for _, byName := range []bool{true, false} {
			for _, p := range all {
				if byName && p.Name == ref || !byName && p.Alias != "" && p.Alias == ref {
					return strconv.FormatInt(p.ID, 10), p.Query, true
				}
			}
		}

		return "", "", false
	}
}

// expandErr converts a query error into BAD_QUERY with its span.
func expandErr(err error) error {
	if ce, ok := errors.AsType[*core.Error](err); ok {
		return apperr.Query(apperr.Span{Start: ce.Span.Start, End: ce.Span.End}, "%s", ce.Msg)
	}

	return err
}

// ExpandText parses src, expands its preset: references and returns the AST
// (nil for an empty query), ready for httpql.Compile / Eval.
func (s *Service) ExpandText(ctx context.Context, src string) (core.Node, error) {
	var all []Preset

	if strings.Contains(src, "preset") { // skip the DB read for the common case
		var err error
		if all, err = s.List(ctx); err != nil {
			return nil, err
		}
	}

	return Expand(src, all)
}

// Expand is ExpandText over an explicit preset list.
func Expand(src string, all []Preset) (core.Node, error) {
	n, err := httpql.Language.Parse(src)
	if err != nil {
		return nil, expandErr(err)
	}

	n, err = query.ExpandPresets(httpql.Language, n, lookup(all))

	return n, expandErr(err)
}

// check validates p against the other presets: unique name and alias, and no
// preset cycle through p's query.
func check(p Preset, others []Preset) error {
	for _, o := range others {
		if o.ID == p.ID {
			continue
		}

		if o.Name == p.Name {
			return apperr.New(apperr.Conflict, "a preset named %q already exists", p.Name)
		}

		if p.Alias != "" && o.Alias == p.Alias {
			return apperr.New(apperr.Conflict, "alias %q is already used by %q", p.Alias, o.Name)
		}
	}

	all := append(slices.DeleteFunc(slices.Clone(others), func(o Preset) bool { return o.ID == p.ID }), p)

	// A cycle through p always shows when expanding p itself.
	_, err := Expand(p.Query, all)

	return err
}

// Create adds p at the end of the list; p.ID is ignored.
func (s *Service) Create(ctx context.Context, p Preset) (Preset, error) {
	p, err := clean(p)
	if err != nil {
		return p, err
	}

	err = s.db().Write(ctx, func(tx *sql.Tx) error {
		all, err := list(ctx, tx)
		if err != nil {
			return err
		}

		if len(all) >= maxPresets {
			return apperr.New(apperr.BadInput, "at most %d presets", maxPresets)
		}

		p.ID = 0
		if err := check(p, all); err != nil {
			return err
		}

		res, err := tx.ExecContext(ctx, `INSERT INTO filter_presets(name, alias, clause, sort_order) VALUES (?, nullif(?, ''), ?, ?)`, p.Name, p.Alias, p.Query, len(all))
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

	err = s.db().Write(ctx, func(tx *sql.Tx) error {
		if _, err := get(ctx, tx, p.ID); err != nil {
			return err
		}

		all, err := list(ctx, tx)
		if err != nil {
			return err
		}

		if err := check(p, all); err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, `UPDATE filter_presets SET name = ?, alias = nullif(?, ''), clause = ? WHERE id = ?`, p.Name, p.Alias, p.Query, p.ID)

		return err
	})

	return p, err
}

// Delete removes a preset. Presets that referenced it start failing to
// expand until fixed, which the UI surfaces as a query error.
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.db().Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM filter_presets WHERE id = ?`, id)
		if err != nil {
			return err
		}

		if n, _ := res.RowsAffected(); n == 0 {
			return apperr.New(apperr.NotFound, "filter preset %d not found", id)
		}

		return nil
	})
}

// Reorder sets the display order to ids, which must list every preset once.
func (s *Service) Reorder(ctx context.Context, ids []int64) error {
	return s.db().Write(ctx, func(tx *sql.Tx) error {
		all, err := list(ctx, tx)
		if err != nil {
			return err
		}

		seen := map[int64]bool{}
		for _, id := range ids {
			seen[id] = true
		}

		if len(ids) != len(all) || len(seen) != len(all) {
			return apperr.New(apperr.BadInput, "ids must list every preset exactly once")
		}

		for _, p := range all {
			if !seen[p.ID] {
				return apperr.New(apperr.BadInput, "unknown preset %d in order", p.ID)
			}
		}

		for i, id := range ids {
			if _, err := tx.ExecContext(ctx, `UPDATE filter_presets SET sort_order = ? WHERE id = ?`, i, id); err != nil {
				return err
			}
		}

		return nil
	})
}

type exportFile struct {
	Version int      `json:"version"`
	Presets []Preset `json:"presets"`
}

// Export returns all presets as JSON.
func (s *Service) Export(ctx context.Context) ([]byte, error) {
	all, err := s.List(ctx)
	if err != nil {
		return nil, err
	}

	return json.Marshal(exportFile{Version: 1, Presets: all}, json.Deterministic(true))
}

// Import adds the presets of an Export file, all or nothing. A preset whose
// name already exists is skipped; the returned count is the number added.
func (s *Service) Import(ctx context.Context, data []byte) (added int, err error) {
	var f exportFile
	if err := json.Unmarshal(data, &f); err != nil {
		return 0, apperr.New(apperr.BadInput, "invalid preset file: %v", err)
	}

	if f.Version != 1 {
		return 0, apperr.New(apperr.BadInput, "unsupported preset file version %d", f.Version)
	}

	if len(f.Presets) > maxPresets {
		return 0, apperr.New(apperr.BadInput, "at most %d presets per file", maxPresets)
	}

	err = s.db().Write(ctx, func(tx *sql.Tx) error {
		added = 0

		all, err := list(ctx, tx)
		if err != nil {
			return err
		}

		n := len(all)

		// Insert first, then validate the whole set: imported presets may
		// reference each other in any order.
		for _, p := range f.Presets {
			if p, err = clean(p); err != nil {
				return err
			}

			if slices.ContainsFunc(all, func(o Preset) bool { return o.Name == p.Name }) {
				continue
			}

			if len(all) >= maxPresets {
				return apperr.New(apperr.BadInput, "at most %d presets per project", maxPresets)
			}

			if p.Alias != "" && slices.ContainsFunc(all, func(o Preset) bool { return o.Alias == p.Alias }) {
				return apperr.New(apperr.Conflict, "alias %q is already used", p.Alias)
			}

			res, err := tx.ExecContext(ctx, `INSERT INTO filter_presets(name, alias, clause, sort_order) VALUES (?, nullif(?, ''), ?, ?)`, p.Name, p.Alias, p.Query, len(all))
			if err != nil {
				return err
			}

			if p.ID, err = res.LastInsertId(); err != nil {
				return err
			}

			all = append(all, p)
			added++
		}

		// Only new rows: an old preset left dangling by Delete must not block imports.
		for _, p := range all[n:] {
			if _, err := Expand(p.Query, all); err != nil {
				return err
			}
		}

		return nil
	})

	return added, err
}
