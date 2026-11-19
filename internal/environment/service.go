// Package environment stores Global and project variables and resolves template values.
package environment

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/pocikode/ratatoskr/internal/apperr"
	"github.com/pocikode/ratatoskr/internal/secret"
	"github.com/pocikode/ratatoskr/internal/store"
)

// GlobalID identifies the instance environment in every project.
const GlobalID int64 = 0

// Variable is one plain or sealed value. Service results contain decrypted values.
type Variable struct {
	Name, Value string
	Secret      bool
}

// Environment is a named variable list; ID zero denotes Global.
type Environment struct {
	ID                 int64
	Name               string
	IsGlobal, Selected bool
	Variables          []Variable
}

// Deps supplies instance storage, the current project and the shared secret box.
type Deps struct {
	Instance *store.DB
	Project  func() *store.DB
	Secrets  *secret.Box
}

// Service stores environments. Each operation captures the current project once.
type Service struct{ deps Deps }

// New returns a service using the supplied databases and secret box.
func New(deps Deps) *Service { return &Service{deps: deps} }

func global() Environment {
	return Environment{ID: GlobalID, Name: "Global", IsGlobal: true, Variables: []Variable{}}
}

func missing(id int64) error { return apperr.New(apperr.NotFound, "environment %d not found", id) }

func cleanName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if !utf8.ValidString(name) || name == "" || utf8.RuneCountInString(name) > 200 || strings.ContainsFunc(name, unicode.IsControl) {
		return "", apperr.New(apperr.BadInput, "environment name must be 1-200 characters without controls")
	}
	if name == "Global" {
		return "", apperr.New(apperr.Conflict, "Global is reserved")
	}
	return name, nil
}

func nameAvailable(ctx context.Context, tx *sql.Tx, name string, except int64) error {
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM environments WHERE name = ? AND id <> ?`, name, except).Scan(&n); err != nil {
		return err
	}
	if n != 0 {
		return apperr.New(apperr.Conflict, "environment %q already exists", name)
	}
	return nil
}

func aad(id int64, name string) []byte {
	return []byte("ratatoskr/environment/v1/" + strconv.FormatInt(id, 10) + "/" + name)
}

func (s *Service) variables(ctx context.Context, tx *sql.Tx, id int64) ([]Variable, error) {
	query := `SELECT name, value, secret FROM env_vars WHERE env_id = ? ORDER BY rowid`
	args := []any{id}
	if id == GlobalID {
		query, args = `SELECT name, value, secret FROM env_global ORDER BY rowid`, nil
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Variable{}
	for rows.Next() {
		var v Variable
		if err := rows.Scan(&v.Name, &v.Value, &v.Secret); err != nil {
			return nil, err
		}
		if v.Secret {
			blob, err := base64.StdEncoding.DecodeString(v.Value)
			if err != nil {
				return nil, fmt.Errorf("environment: decode secret %q: %w", v.Name, err)
			}
			plain, err := s.deps.Secrets.Open(blob, aad(id, v.Name))
			if err != nil {
				return nil, fmt.Errorf("environment: open secret %q: %w", v.Name, err)
			}
			v.Value = string(plain)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Service) get(ctx context.Context, tx *sql.Tx, id int64) (Environment, error) {
	e := global()
	if id != GlobalID {
		e = Environment{ID: id}
		err := tx.QueryRowContext(ctx, `SELECT name, selected FROM environments WHERE id = ?`, id).Scan(&e.Name, &e.Selected)
		if errors.Is(err, sql.ErrNoRows) {
			return e, missing(id)
		}
		if err != nil {
			return e, err
		}
	}
	var err error
	e.Variables, err = s.variables(ctx, tx, id)
	return e, err
}

func (s *Service) database(id int64) *store.DB {
	if id == GlobalID {
		return s.deps.Instance
	}
	return s.deps.Project()
}

// Get returns an environment or NOT_FOUND.
func (s *Service) Get(ctx context.Context, id int64) (e Environment, err error) {
	err = s.database(id).View(ctx, func(tx *sql.Tx) error { e, err = s.get(ctx, tx, id); return err })
	return e, err
}

// List returns Global first, followed by project environments in name order.
func (s *Service) List(ctx context.Context) ([]Environment, error) {
	db := s.deps.Project()
	g, err := s.Get(ctx, GlobalID)
	if err != nil {
		return nil, err
	}
	out := []Environment{g}
	err = db.View(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT id, name, selected FROM environments ORDER BY name, id`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var e Environment
			if err := rows.Scan(&e.ID, &e.Name, &e.Selected); err != nil {
				return err
			}
			out = append(out, e)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
		for i := 1; i < len(out); i++ {
			out[i].Variables, err = s.variables(ctx, tx, out[i].ID)
			if err != nil {
				return err
			}
		}
		return nil
	})
	return out, err
}

// Create adds an empty unselected environment to the current project.
func (s *Service) Create(ctx context.Context, name string) (Environment, error) {
	name, err := cleanName(name)
	if err != nil {
		return Environment{}, err
	}
	e := Environment{Name: name, Variables: []Variable{}}
	err = s.deps.Project().Write(ctx, func(tx *sql.Tx) error {
		if err := nameAvailable(ctx, tx, name, 0); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO environments(name) VALUES (?)`, name)
		if err != nil {
			return err
		}
		e.ID, err = res.LastInsertId()
		return err
	})
	return e, err
}

// Rename changes a project environment's name; Global cannot be renamed.
func (s *Service) Rename(ctx context.Context, id int64, name string) (e Environment, err error) {
	if id == GlobalID {
		return e, apperr.New(apperr.Conflict, "cannot rename Global")
	}
	name, err = cleanName(name)
	if err != nil {
		return e, err
	}
	err = s.deps.Project().Write(ctx, func(tx *sql.Tx) error {
		e, err = s.get(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := nameAvailable(ctx, tx, name, id); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE environments SET name = ? WHERE id = ?`, name, id)
		e.Name = name
		return err
	})
	return e, err
}

// Delete removes an environment and its variables; Global cannot be deleted.
func (s *Service) Delete(ctx context.Context, id int64) error {
	if id == GlobalID {
		return apperr.New(apperr.Conflict, "cannot delete Global")
	}
	return s.deps.Project().Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM environments WHERE id = ?`, id)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return missing(id)
		}
		return nil
	})
}

// SetVariables atomically replaces the complete list, preserving input order.
func (s *Service) SetVariables(ctx context.Context, id int64, vars []Variable) (e Environment, err error) {
	seen := make(map[string]bool, len(vars))
	for _, v := range vars {
		if len(v.Name) > 100 || !validVariableName(v.Name) || seen[v.Name] || len(v.Value) > 1<<20 || !utf8.ValidString(v.Value) {
			return e, apperr.New(apperr.BadInput, "invalid or duplicate variable %q (name <= 100 characters, value <= 1 MiB)", v.Name)
		}
		seen[v.Name] = true
	}
	err = s.database(id).Write(ctx, func(tx *sql.Tx) error {
		e, err = s.get(ctx, tx, id)
		if err != nil {
			return err
		}
		query, args := `DELETE FROM env_vars WHERE env_id = ?`, []any{id}
		if id == GlobalID {
			query, args = `DELETE FROM env_global`, nil
		}
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return err
		}
		for _, v := range vars {
			value := v.Value
			if v.Secret {
				value = base64.StdEncoding.EncodeToString(s.deps.Secrets.Seal([]byte(value), aad(id, v.Name)))
			}
			query, args = `INSERT INTO env_vars(env_id,name,value,secret) VALUES (?,?,?,?)`, []any{id, v.Name, value, v.Secret}
			if id == GlobalID {
				query, args = `INSERT INTO env_global(name,value,secret) VALUES (?,?,?)`, []any{v.Name, value, v.Secret}
			}
			if _, err := tx.ExecContext(ctx, query, args...); err != nil {
				return err
			}
		}
		e.Variables = append([]Variable{}, vars...)
		return nil
	})
	return e, err
}

// Select selects a project environment; nil deselects. Global is always used.
func (s *Service) Select(ctx context.Context, id *int64) (out *Environment, err error) {
	if id != nil && *id == GlobalID {
		return nil, apperr.New(apperr.BadInput, "Global is always used; select null instead")
	}
	err = s.deps.Project().Write(ctx, func(tx *sql.Tx) error {
		if id != nil {
			e, getErr := s.get(ctx, tx, *id)
			if getErr != nil {
				return getErr
			}
			e.Selected = true
			out = &e
		}
		if _, err := tx.ExecContext(ctx, `UPDATE environments SET selected = 0 WHERE selected <> 0`); err != nil {
			return err
		}
		if id != nil {
			_, err := tx.ExecContext(ctx, `UPDATE environments SET selected = 1 WHERE id = ?`, *id)
			return err
		}
		return nil
	})
	return out, err
}

// Vars returns Global values overlaid by the current project's selection.
func (s *Service) Vars(ctx context.Context) (map[string]string, error) {
	db := s.deps.Project()
	g, err := s.Get(ctx, GlobalID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(g.Variables))
	for _, v := range g.Variables {
		out[v.Name] = v.Value
	}
	err = db.View(ctx, func(tx *sql.Tx) error {
		var id int64
		err := tx.QueryRowContext(ctx, `SELECT id FROM environments WHERE selected <> 0`).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		vars, err := s.variables(ctx, tx, id)
		if err != nil {
			return err
		}
		for _, v := range vars {
			out[v.Name] = v.Value
		}
		return nil
	})
	return out, err
}

func validVariableName(name string) bool {
	if name == "" {
		return false
	}
	for i, c := range []byte(name) {
		if c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c == '_' {
			continue
		}
		if i > 0 && (c >= '0' && c <= '9' || c == '.' || c == '-') {
			continue
		}
		return false
	}
	return true
}

var tokenRE = regexp.MustCompile(`\{\{[ \t\r\n]*([A-Za-z_][A-Za-z0-9_.-]*)[ \t\r\n]*\}\}`)

// Interpolate substitutes known {{name}} tokens once, preserving every other byte.
// Names may have leading and trailing whitespace matching [ \t\r\n]* inside the braces.
func Interpolate(raw []byte, vars map[string]string) []byte {
	return tokenRE.ReplaceAllFunc(raw, func(match []byte) []byte {
		name := strings.TrimSpace(string(match[2 : len(match)-2]))
		if value, ok := vars[name]; ok {
			return []byte(value)
		}
		return match
	})
}
