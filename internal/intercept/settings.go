package intercept

import (
	"context"
	"database/sql"
	"encoding/json/v2"
	"errors"

	"github.com/pocikode/ratatoskr/internal/apperr"
	"github.com/pocikode/ratatoskr/internal/filterpreset"
	"github.com/pocikode/ratatoskr/internal/query/core"
	"github.com/pocikode/ratatoskr/internal/query/httpql"
	"github.com/pocikode/ratatoskr/internal/scope"
	"github.com/pocikode/ratatoskr/internal/store"
)

// Settings controls which messages are held and how dropped messages are answered.
type Settings struct {
	Queuing        bool
	Requests       bool
	Responses      bool
	Query          string
	ScopeID        int64 // zero has no scope restriction
	DropStatusCode int
}

// Update changes only the supplied settings. A zero ScopeID clears the scope.
type Update struct {
	Queuing, Requests, Responses *bool
	Query                        *string
	ScopeID                      *int64
	DropStatusCode               *int
}

func defaults() Settings { return Settings{Requests: true, DropStatusCode: 502} }

func (s *Settings) fields() map[string]any {
	return map[string]any{
		"requests": &s.Requests, "responses": &s.Responses, "query": &s.Query,
		"scopeId": &s.ScopeID, "dropStatusCode": &s.DropStatusCode,
	}
}

func load(ctx context.Context, db *store.DB) (Settings, error) {
	settings := defaults()
	if db == nil {
		return settings, nil
	}

	fields := settings.fields()
	err := db.View(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT key, value_json FROM intercept_settings`)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var key, value string
			if err := rows.Scan(&key, &value); err != nil {
				return err
			}
			if field := fields[key]; field != nil {
				if err := json.Unmarshal([]byte(value), field); err != nil {
					return err
				}
			}
		}

		return rows.Err()
	})

	return settings, err
}

func save(ctx context.Context, db *store.DB, settings Settings) error {
	return db.Write(ctx, func(tx *sql.Tx) error {
		for key, field := range settings.fields() {
			raw, err := json.Marshal(field)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO intercept_settings (key, value_json) VALUES (?, ?)
				ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json`, key, string(raw)); err != nil {
				return err
			}
		}

		return nil
	})
}

func apply[T any](dst, src *T) {
	if src != nil {
		*dst = *src
	}
}

func (u Update) apply(to Settings) Settings {
	apply(&to.Queuing, u.Queuing)
	apply(&to.Requests, u.Requests)
	apply(&to.Responses, u.Responses)
	apply(&to.Query, u.Query)
	apply(&to.ScopeID, u.ScopeID)
	apply(&to.DropStatusCode, u.DropStatusCode)

	return to
}

func compile(ctx context.Context, db *store.DB, text string) (core.Node, error) {
	node, err := filterpreset.New(func() *store.DB { return db }).ExpandText(ctx, text)
	if err != nil {
		return nil, err
	}
	if _, err := httpql.Compile(node); err != nil {
		if e, ok := errors.AsType[*core.Error](err); ok {
			return nil, apperr.Query(apperr.Span{Start: e.Span.Start, End: e.Span.End}, "%s", e.Msg)
		}

		return nil, err
	}

	return node, nil
}

// requestCondition makes each response clause false before applying boolean operators.
func requestCondition(node core.Node) core.Node {
	switch n := node.(type) {
	case *core.And:
		return &core.And{L: requestCondition(n.L), R: requestCondition(n.R), Loc: n.Loc}
	case *core.Or:
		return &core.Or{L: requestCondition(n.L), R: requestCondition(n.R), Loc: n.Loc}
	case *core.Not:
		return &core.Not{X: requestCondition(n.X), Loc: n.Loc}
	case *core.Clause:
		if n.Namespace == "resp" {
			return &core.Not{Loc: n.Loc} // Eval(nil) is true.
		}
	}

	return node
}

// Settings returns the selected project's settings.
func (s *Service) Settings(ctx context.Context) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.selectProject(ctx); err != nil {
		return Settings{}, err
	}

	return s.rules.Load().settings, nil
}

// UpdateSettings validates and persists a settings change before releasing items.
func (s *Service) UpdateSettings(ctx context.Context, update Update) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.selectProject(ctx); err != nil {
		return Settings{}, err
	}

	old := s.rules.Load()
	db := old.db
	if db == nil {
		return Settings{}, apperr.New(apperr.Conflict, "no project is open")
	}

	next := update.apply(old.settings)
	if next.DropStatusCode < 100 || next.DropStatusCode > 599 {
		return Settings{}, apperr.New(apperr.BadInput, "dropStatusCode must be between 100 and 599")
	}

	node := old.responseQuery
	if update.Query != nil {
		var err error
		if node, err = compile(ctx, db, next.Query); err != nil {
			return Settings{}, err
		}
	}
	if update.ScopeID != nil && next.ScopeID != 0 {
		if _, err := scope.New(func() *store.DB { return db }).Get(ctx, next.ScopeID); err != nil {
			return Settings{}, err
		}
	}

	persisted := next
	persisted.Queuing = old.settings.Queuing
	if persisted != old.settings {
		if err := save(ctx, db, next); err != nil {
			return Settings{}, err
		}
	}
	if db != s.selected() {
		return Settings{}, apperr.New(apperr.Conflict, "selected project changed")
	}

	rules := &holdRules{db: db, settings: next, requestQuery: requestCondition(node), responseQuery: node}
	s.rules.Store(rules)
	s.releaseMatching(db, func(h *held) bool { return !rules.enabled(h.Kind) }, false, true)
	s.changes.Publish(settingsChange{db, next})

	return next, nil
}
