// Package project manages Ratatoskr projects: the registry in instance.db and
// the one project database that is open at any time (docs/01-feature-spec.md
// F-WSP-1, docs/03-architecture.md §5).
//
// Each project lives in <data dir>/projects/<id>/ and owns a project.db. Exactly
// one project is selected; selecting another closes the old handle, opens the
// new one and emits a [Selected] [Change], which other services subscribe to via
// [Service.Subscribe] to reopen their own handles.
package project

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/pocikode/ratatoskr/internal/apperr"
	"github.com/pocikode/ratatoskr/internal/store"
)

const (
	projectsDir = "projects"
	// tempKey is the instance.db settings key holding the IDs of temporary
	// projects. The projects table has no column for it.
	tempKey      = "project.temporary"
	tempName     = "Temporary project"
	copySuffix   = " (copy)"
	maxNameRunes = 200
)

// Project is one registry entry.
type Project struct {
	ID           int64
	Name         string
	Temporary    bool
	Default      bool
	Selected     bool
	CreatedAt    time.Time
	LastOpenedAt *time.Time
	// DBSize is the size of project.db in bytes; zero while it was never opened.
	DBSize int64
}

// Deps are the inputs to [Open].
type Deps struct {
	// Instance is the open instance.db.
	Instance *store.DB
	// Root is the data dir; project directories are created below it.
	Root *os.Root
	// Logger defaults to [slog.Default].
	Logger *slog.Logger
	// Now defaults to [time.Now].
	Now func() time.Time
}

// Service is the project registry plus the selected project's database.
type Service struct {
	inst *store.DB
	root *os.Root
	log  *slog.Logger
	now  func() time.Time

	// mu serializes mutations (create, select, delete, close, ...).
	mu     sync.Mutex
	closed bool

	cur   atomic.Pointer[store.DB]
	curID atomic.Int64
	hub   hub
}

// Open loads the registry, removes leftovers of a crashed run (temporary
// projects, directories without a registry row), then selects the default
// project or, when there is none, a new temporary project.
func Open(ctx context.Context, deps Deps) (*Service, error) {
	if deps.Instance == nil || deps.Root == nil {
		return nil, errors.New("project: Instance and Root are required")
	}

	s := &Service{inst: deps.Instance, root: deps.Root, log: deps.Logger, now: deps.Now}
	if s.log == nil {
		s.log = slog.Default()
	}

	if s.now == nil {
		s.now = time.Now
	}

	if err := s.root.MkdirAll(projectsDir, 0o700); err != nil {
		return nil, fmt.Errorf("project: create %s: %w", projectsDir, err)
	}

	if err := s.purgeLeftovers(ctx); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	id, err := s.defaultID(ctx)
	if err != nil {
		return nil, err
	}

	if id == 0 {
		p, err := s.createLocked(ctx, tempName, true)
		if err != nil {
			return nil, err
		}

		id = p.ID
	}

	if _, err := s.selectLocked(ctx, id); err != nil {
		return nil, err
	}

	return s, nil
}

// Close closes the selected project's database and deletes the temporary
// projects (clean shutdown). It ends all subscriptions.
func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true

	var errs []error

	if db := s.cur.Swap(nil); db != nil {
		errs = append(errs, db.Close())
	}

	s.curID.Store(0)

	// Close must finish even when the caller's context is gone.
	ctx := context.Background()

	ids, err := s.tempIDs(ctx)
	errs = append(errs, err)

	for _, id := range ids {
		errs = append(errs, s.removeProject(ctx, id))
	}

	s.hub.closeAll()

	return errors.Join(errs...)
}

// DB is the selected project's database. Hold it only for the duration of one
// operation and re-fetch after a [Selected] change: switching closes it.
func (s *Service) DB() *store.DB { return s.cur.Load() }

// Current returns the selected project.
func (s *Service) Current(ctx context.Context) (Project, error) {
	return s.Get(ctx, s.curID.Load())
}

// List returns all projects, most recently opened first (never-opened ones by
// creation time).
func (s *Service) List(ctx context.Context) ([]Project, error) {
	var out []Project

	err := s.inst.View(ctx, func(tx *sql.Tx) error {
		temp, err := loadTemp(ctx, tx)
		if err != nil {
			return err
		}

		rows, err := tx.QueryContext(ctx, `SELECT id, name, created_at, last_opened_at, is_default FROM projects
			ORDER BY COALESCE(last_opened_at, created_at) DESC, id DESC`)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			r, err := scanRow(rows)
			if err != nil {
				return err
			}

			out = append(out, s.project(r, temp))
		}

		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("project: list: %w", err)
	}

	return out, nil
}

// Get returns one project, or a NOT_FOUND error.
func (s *Service) Get(ctx context.Context, id int64) (Project, error) {
	var p Project

	err := s.inst.View(ctx, func(tx *sql.Tx) error {
		r, err := getRow(ctx, tx, id)
		if err != nil {
			return err
		}

		temp, err := loadTemp(ctx, tx)
		if err != nil {
			return err
		}

		p = s.project(r, temp)

		return nil
	})
	if err != nil {
		return Project{}, wrap("get", err)
	}

	return p, nil
}

// Create adds a project; with sel it also becomes the selected one.
func (s *Service) Create(ctx context.Context, name string, sel bool) (Project, error) {
	name, err := cleanName(name)
	if err != nil {
		return Project{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.checkOpen(); err != nil {
		return Project{}, err
	}

	p, err := s.createLocked(ctx, name, false)
	if err != nil {
		return Project{}, err
	}

	if sel {
		return s.selectLocked(ctx, p.ID)
	}

	return p, nil
}

// Rename renames a project; renaming a temporary project saves it.
func (s *Service) Rename(ctx context.Context, id int64, name string) (Project, error) {
	name, err := cleanName(name)
	if err != nil {
		return Project{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.checkOpen(); err != nil {
		return Project{}, err
	}

	err = s.inst.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE projects SET name = ? WHERE id = ?`, name, id)
		if err != nil {
			return err
		}

		if n, _ := res.RowsAffected(); n == 0 {
			return errNotFound(id)
		}

		temp, err := loadTemp(ctx, tx)
		if err != nil {
			return err
		}

		if temp[id] {
			delete(temp, id)

			return saveTemp(ctx, tx, temp)
		}

		return nil
	})
	if err != nil {
		return Project{}, wrap("rename", err)
	}

	return s.emitUpdated(ctx, id)
}

// Duplicate copies a project's data into a new, unselected project. An empty
// name means the source name plus " (copy)".
func (s *Service) Duplicate(ctx context.Context, id int64, name string) (Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.checkOpen(); err != nil {
		return Project{}, err
	}

	src, err := s.Get(ctx, id)
	if err != nil {
		return Project{}, err
	}

	if strings.TrimSpace(name) == "" {
		name = src.Name + copySuffix
	}

	name, err = cleanName(name)
	if err != nil {
		return Project{}, err
	}

	dst, err := s.createQuiet(ctx, name, false)
	if err != nil {
		return Project{}, err
	}

	if err := s.copyData(ctx, src.ID, dst.ID); err != nil {
		if rmErr := s.removeProject(context.WithoutCancel(ctx), dst.ID); rmErr != nil {
			s.log.Warn("project: remove failed duplicate", "project_id", dst.ID, "error", rmErr)
		}

		return Project{}, fmt.Errorf("project: duplicate %d: %w", id, err)
	}

	// Re-read so the event carries the copied database size.
	if got, err := s.Get(ctx, dst.ID); err == nil {
		dst = got
	} else {
		s.log.Warn("project: re-read duplicate", "project_id", dst.ID, "error", err)
	}

	s.hub.emit(Change{Kind: Created, ProjectID: dst.ID, Project: &dst})

	return dst, nil
}

// Delete removes a project and its directory. Deleting the selected project
// first selects the default project, or a new temporary one when the deleted
// project was the default or none is.
func (s *Service) Delete(ctx context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.checkOpen(); err != nil {
		return err
	}

	if _, err := s.Get(ctx, id); err != nil {
		return err
	}

	if s.curID.Load() == id {
		next, err := s.defaultID(ctx)
		if err != nil {
			return err
		}

		if next == 0 || next == id {
			p, err := s.createLocked(ctx, tempName, true)
			if err != nil {
				return err
			}

			next = p.ID
		}

		if _, err := s.selectLocked(ctx, next); err != nil {
			return err
		}
	}

	if err := s.removeRow(ctx, id); err != nil {
		return fmt.Errorf("project: delete %d: %w", id, err)
	}

	s.hub.emit(Change{Kind: Deleted, ProjectID: id})

	if rmErr := s.root.RemoveAll(dirOf(id)); rmErr != nil {
		return fmt.Errorf("project: delete %d: %w", id, rmErr)
	}

	return nil
}

// SetDefault makes id the default project; nil clears the default. Temporary
// projects cannot be the default.
func (s *Service) SetDefault(ctx context.Context, id *int64) (*Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.checkOpen(); err != nil {
		return nil, err
	}

	var changed []int64

	err := s.inst.Write(ctx, func(tx *sql.Tx) error {
		if id != nil {
			if _, err := getRow(ctx, tx, *id); err != nil {
				return err
			}

			temp, err := loadTemp(ctx, tx)
			if err != nil {
				return err
			}

			if temp[*id] {
				return apperr.New(apperr.BadInput, "a temporary project cannot be the default; rename it to save it first")
			}
		}

		rows, err := tx.QueryContext(ctx, `SELECT id FROM projects WHERE is_default = 1`)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var prev int64
			if err := rows.Scan(&prev); err != nil {
				return err
			}

			changed = append(changed, prev)
		}

		if err := rows.Err(); err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, `UPDATE projects SET is_default = 0 WHERE is_default = 1`); err != nil {
			return err
		}

		if id == nil {
			return nil
		}

		if !slices.Contains(changed, *id) {
			changed = append(changed, *id)
		}

		_, err = tx.ExecContext(ctx, `UPDATE projects SET is_default = 1 WHERE id = ?`, *id)

		return err
	})
	if err != nil {
		return nil, wrap("set default", err)
	}

	var result *Project

	for _, c := range changed {
		p, err := s.emitUpdated(ctx, c)
		if err != nil {
			return nil, err
		}

		if id != nil && c == *id {
			result = &p
		}
	}

	return result, nil
}

// Select makes id the selected project: it opens the new database, closes the
// old one and emits a [Selected] change. Selecting the selected project does
// nothing.
func (s *Service) Select(ctx context.Context, id int64) (Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.checkOpen(); err != nil {
		return Project{}, err
	}

	return s.selectLocked(ctx, id)
}

func (s *Service) checkOpen() error {
	if s.closed {
		return errors.New("project: service closed")
	}

	return nil
}

func (s *Service) selectLocked(ctx context.Context, id int64) (Project, error) {
	if s.curID.Load() == id && s.cur.Load() != nil {
		return s.Get(ctx, id)
	}

	if _, err := s.Get(ctx, id); err != nil {
		return Project{}, err
	}

	db, err := s.openDB(ctx, id)
	if err != nil {
		return Project{}, fmt.Errorf("project: open %d: %w", id, err)
	}

	err = s.inst.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE projects SET last_opened_at = ? WHERE id = ?`, s.now().UnixMilli(), id)

		return err
	})
	if err != nil {
		_ = db.Close()

		return Project{}, fmt.Errorf("project: record open of %d: %w", id, err)
	}

	old := s.cur.Swap(db)
	s.curID.Store(id)

	if old != nil {
		if err := old.Close(); err != nil {
			s.log.Warn("project: close previous database", "error", err)
		}
	}

	p, err := s.Get(ctx, id)
	if err != nil {
		return Project{}, err
	}

	s.hub.emit(Change{Kind: Selected, ProjectID: id, Project: &p})

	return p, nil
}

// openDB opens (creating) a project's project.db.
func (s *Service) openDB(ctx context.Context, id int64) (*store.DB, error) {
	if err := s.root.MkdirAll(dirOf(id), 0o700); err != nil {
		return nil, err
	}

	sub, err := s.root.OpenRoot(dirOf(id))
	if err != nil {
		return nil, err
	}
	defer sub.Close()

	return store.OpenProject(ctx, sub)
}

func (s *Service) createLocked(ctx context.Context, name string, temp bool) (Project, error) {
	p, err := s.createQuiet(ctx, name, temp)
	if err != nil {
		return Project{}, err
	}

	s.hub.emit(Change{Kind: Created, ProjectID: p.ID, Project: &p})

	return p, nil
}

// createQuiet creates the row and directory without emitting [Created].
func (s *Service) createQuiet(ctx context.Context, name string, temp bool) (Project, error) {
	var id int64

	err := s.inst.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `INSERT INTO projects (name, path, created_at) VALUES (?, '', ?)`,
			name, s.now().UnixMilli())
		if err != nil {
			return err
		}

		if id, err = res.LastInsertId(); err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, `UPDATE projects SET path = ? WHERE id = ?`, dirOf(id), id); err != nil {
			return err
		}

		if !temp {
			return nil
		}

		set, err := loadTemp(ctx, tx)
		if err != nil {
			return err
		}

		set[id] = true

		return saveTemp(ctx, tx, set)
	})
	if err != nil {
		return Project{}, fmt.Errorf("project: create: %w", err)
	}

	if err := s.root.MkdirAll(dirOf(id), 0o700); err != nil {
		if rmErr := s.removeProject(context.WithoutCancel(ctx), id); rmErr != nil {
			s.log.Warn("project: roll back create", "project_id", id, "error", rmErr)
		}

		return Project{}, fmt.Errorf("project: create directory: %w", err)
	}

	return s.Get(ctx, id)
}

func (s *Service) emitUpdated(ctx context.Context, id int64) (Project, error) {
	p, err := s.Get(ctx, id)
	if err != nil {
		return Project{}, err
	}

	s.hub.emit(Change{Kind: Updated, ProjectID: id, Project: &p})

	return p, nil
}

// removeProject deletes the registry row and the directory of id. It does not
// emit events and tolerates missing pieces.
func (s *Service) removeProject(ctx context.Context, id int64) error {
	if err := s.removeRow(ctx, id); err != nil {
		return err
	}

	return s.root.RemoveAll(dirOf(id))
}

// removeRow deletes the registry row (and temporary mark) of id.
func (s *Service) removeRow(ctx context.Context, id int64) error {
	return s.inst.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, id); err != nil {
			return err
		}

		temp, err := loadTemp(ctx, tx)
		if err != nil {
			return err
		}

		if !temp[id] {
			return nil
		}

		delete(temp, id)

		return saveTemp(ctx, tx, temp)
	})
}

// purgeLeftovers deletes temporary projects a crashed run left behind and
// project directories that have no registry row.
func (s *Service) purgeLeftovers(ctx context.Context) error {
	ids, err := s.tempIDs(ctx)
	if err != nil {
		return err
	}

	for _, id := range ids {
		if err := s.removeProject(ctx, id); err != nil {
			return fmt.Errorf("project: remove leftover temporary project %d: %w", id, err)
		}
	}

	known := map[string]bool{}

	err = s.inst.View(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT id FROM projects`)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return err
			}

			known[strconv.FormatInt(id, 10)] = true
		}

		return rows.Err()
	})
	if err != nil {
		return fmt.Errorf("project: list ids: %w", err)
	}

	entries, err := fs.ReadDir(s.root.FS(), projectsDir)
	if err != nil {
		return fmt.Errorf("project: read %s: %w", projectsDir, err)
	}

	for _, e := range entries {
		if known[e.Name()] {
			continue
		}

		s.log.Warn("project: removing orphan project directory", "dir", e.Name())

		if err := s.root.RemoveAll(path.Join(projectsDir, e.Name())); err != nil {
			return fmt.Errorf("project: remove orphan %s: %w", e.Name(), err)
		}
	}

	return nil
}

func (s *Service) tempIDs(ctx context.Context) ([]int64, error) {
	var ids []int64

	err := s.inst.View(ctx, func(tx *sql.Tx) error {
		set, err := loadTemp(ctx, tx)
		if err != nil {
			return err
		}

		for id := range set {
			ids = append(ids, id)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("project: load temporary projects: %w", err)
	}

	slices.Sort(ids)

	return ids, nil
}

// defaultID is the default project's ID, or 0.
func (s *Service) defaultID(ctx context.Context) (int64, error) {
	var id int64

	err := s.inst.View(ctx, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `SELECT id FROM projects WHERE is_default = 1 LIMIT 1`).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}

		return err
	})
	if err != nil {
		return 0, fmt.Errorf("project: find default: %w", err)
	}

	return id, nil
}

// project builds the API view of a row. Size errors read as zero: the file
// does not exist until the project is first opened.
func (s *Service) project(r row, temp map[int64]bool) Project {
	p := Project{
		ID:        r.id,
		Name:      r.name,
		Temporary: temp[r.id],
		Default:   r.def,
		Selected:  s.curID.Load() == r.id,
		CreatedAt: time.UnixMilli(r.created),
	}

	if r.lastOpened.Valid {
		t := time.UnixMilli(r.lastOpened.Int64)
		p.LastOpenedAt = &t
	}

	if info, err := s.root.Stat(path.Join(dirOf(r.id), "project.db")); err == nil {
		p.DBSize = info.Size()
	}

	return p
}

func dirOf(id int64) string { return path.Join(projectsDir, strconv.FormatInt(id, 10)) }

// cleanName trims name and rejects empty, overlong and control-character names.
func cleanName(name string) (string, error) {
	name = strings.TrimSpace(name)

	switch {
	case name == "":
		return "", apperr.New(apperr.BadInput, "project name must not be empty")
	case utf8.RuneCountInString(name) > maxNameRunes:
		return "", apperr.New(apperr.BadInput, "project name must be at most %d characters", maxNameRunes)
	case strings.IndexFunc(name, unicode.IsControl) >= 0:
		return "", apperr.New(apperr.BadInput, "project name must not contain control characters")
	}

	return name, nil
}

func errNotFound(id int64) error {
	return apperr.New(apperr.NotFound, "project %d not found", id)
}

// wrap adds context to infrastructure errors and passes API errors through.
func wrap(op string, err error) error {
	if _, ok := errors.AsType[*apperr.Error](err); ok {
		return err
	}

	return fmt.Errorf("project: %s: %w", op, err)
}
