package replay

import (
	"context"
	"database/sql"
	"encoding/json/v2"
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/pocikode/ratatoskr/internal/apperr"
	"github.com/pocikode/ratatoskr/internal/history"
)

// MaxDraftBytes bounds decoded request drafts.
const MaxDraftBytes = 16 << 20

func cleanName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 200 || strings.ContainsFunc(name, unicode.IsControl) {
		return "", apperr.New(apperr.BadInput, "name must be 1-200 characters without controls")
	}
	return name, nil
}

func validConnection(c Connection) error {
	if c.Host == "" || !utf8.ValidString(c.Host) || strings.ContainsFunc(c.Host, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
		return apperr.New(apperr.BadInput, "host must not be empty or contain whitespace or controls")
	}
	if c.Port < 1 || c.Port > 65535 {
		return apperr.New(apperr.BadInput, "port must be 1-65535")
	}
	if c.SNI != nil && *c.SNI == "" {
		return apperr.New(apperr.BadInput, "sni must not be empty; use null for no override")
	}
	return nil
}

var requestLine = regexp.MustCompile("^([!#$%&'*+.^_`|~0-9A-Za-z-]+) ([^ \\r\\n]+) HTTP/1\\.[01]\\r?\\n")

func defaultName(raw []byte) string {
	parts := requestLine.FindSubmatch(raw)
	if parts == nil {
		return "Untitled"
	}
	name := strings.ToValidUTF8(string(parts[1])+" "+string(parts[2]), "\uFFFD")
	name = string([]rune(name)[:min(utf8.RuneCountInString(name), 60)])
	if strings.ContainsFunc(name, unicode.IsControl) {
		return "Untitled"
	}
	return name
}

// CreateInput copies a history request or requires both Connection and Raw.
// Nil Raw means absent; an empty non-nil slice is an empty draft.
type CreateInput struct {
	CollectionID  *int64
	FromRequestID *int64
	Connection    *Connection
	Raw           []byte
	Name          *string
}

func sourceRequest(ctx context.Context, tx *sql.Tx, id int64) ([]byte, Connection, error) {
	var c Connection
	var truncated bool
	var size int64
	err := tx.QueryRowContext(ctx, `SELECT host, port, tls, request_truncated, length(raw) FROM requests WHERE id = ?`, id).Scan(&c.Host, &c.Port, &c.TLS, &truncated, &size)
	if errors.Is(err, sql.ErrNoRows) {
		err = apperr.New(apperr.NotFound, "request %d not found", id)
	}
	if err != nil {
		return nil, c, err
	}
	if truncated || size > MaxDraftBytes {
		return nil, c, apperr.New(apperr.BadInput, "source request is truncated or larger than 16 MiB")
	}
	var raw []byte
	err = tx.QueryRowContext(ctx, `SELECT raw FROM requests WHERE id = ?`, id).Scan(&raw)
	return raw, c, err
}

func insertSession(ctx context.Context, tx *sql.Tx, out *Session, position int) error {
	if out.Draft == nil {
		out.Draft = []byte{}
	}
	state, err := json.Marshal(out.State)
	if err != nil {
		return err
	}
	out.CreatedAt = time.Now().UTC().Truncate(time.Millisecond)
	res, err := tx.ExecContext(ctx, `INSERT INTO replay_sessions(collection_id, kind, name, state_json, draft, sort_order, created_at) VALUES (?, 'http', ?, ?, ?, ?, ?)`, out.CollectionID, out.Name, string(state), out.Draft, position, out.CreatedAt.UnixMilli())
	if err != nil {
		return err
	}
	out.ID, err = res.LastInsertId()
	return err
}

// CreateSession appends a new HTTP draft to the specified or Default collection.
func (s *Service) CreateSession(ctx context.Context, input CreateInput) (out Session, err error) {
	fromHistory := input.FromRequestID != nil
	if fromHistory && (input.Connection != nil || input.Raw != nil) || !fromHistory && (input.Connection == nil || input.Raw == nil) {
		return out, apperr.New(apperr.BadInput, "provide fromRequestId or both connection and raw")
	}
	if len(input.Raw) > MaxDraftBytes {
		return out, apperr.New(apperr.BadInput, "draft exceeds 16 MiB")
	}
	db, err := s.db()
	if err != nil {
		return out, err
	}
	err = db.Write(ctx, func(tx *sql.Tx) error {
		def, err := ensureDefault(ctx, tx)
		if err != nil {
			return err
		}
		out.CollectionID = def.ID
		if input.CollectionID != nil {
			out.CollectionID = *input.CollectionID
		}
		if _, err := collection(ctx, tx, out.CollectionID); err != nil {
			return err
		}
		if fromHistory {
			out.Draft, out.Connection, err = sourceRequest(ctx, tx, *input.FromRequestID)
			if err != nil {
				return err
			}
		} else {
			out.Draft, out.Connection = input.Raw, *input.Connection
		}
		if err := validConnection(out.Connection); err != nil {
			return err
		}
		out.Name = defaultName(out.Draft)
		if input.Name != nil {
			out.Name, err = cleanName(*input.Name)
			if err != nil {
				return err
			}
		}
		out.Options = Options{UpdateContentLength: true, TimeoutMs: 60000}
		var position int
		if err := tx.QueryRowContext(ctx, `SELECT coalesce(max(sort_order), -1) + 1 FROM replay_sessions WHERE collection_id = ?`, out.CollectionID).Scan(&position); err != nil {
			return err
		}
		return insertSession(ctx, tx, &out, position)
	})
	return out, err
}

// OptionsInput merges non-nil values into existing options.
type OptionsInput struct {
	FollowRedirects     *bool
	UpdateContentLength *bool
	TimeoutMs           *int
	ApplyMatchReplace   *bool
}

// UpdateInput changes only non-nil fields; an empty non-nil Draft clears the draft.
type UpdateInput struct {
	Name       *string
	Connection *Connection
	Options    *OptionsInput
	Draft      []byte
}

func (o *Options) merge(p OptionsInput) {
	if p.FollowRedirects != nil {
		o.FollowRedirects = *p.FollowRedirects
	}
	if p.UpdateContentLength != nil {
		o.UpdateContentLength = *p.UpdateContentLength
	}
	if p.TimeoutMs != nil {
		o.TimeoutMs = *p.TimeoutMs
	}
	if p.ApplyMatchReplace != nil {
		o.ApplyMatchReplace = *p.ApplyMatchReplace
	}
}

func (s *Service) changeSession(ctx context.Context, id int64, change func(*Session) error) (out Session, err error) {
	db, err := s.db()
	if err != nil {
		return out, err
	}
	err = db.Write(ctx, func(tx *sql.Tx) error {
		out, err = session(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := change(&out); err != nil {
			return err
		}
		state, err := json.Marshal(out.State)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE replay_sessions SET name = ?, state_json = ?, draft = ? WHERE id = ?`, out.Name, string(state), out.Draft, id)
		return err
	})
	return out, err
}

// UpdateSession atomically saves valid changes; invalid input saves nothing.
func (s *Service) UpdateSession(ctx context.Context, id int64, input UpdateInput) (Session, error) {
	return s.changeSession(ctx, id, func(out *Session) (err error) {
		if input.Name != nil {
			out.Name, err = cleanName(*input.Name)
			if err != nil {
				return err
			}
		}
		if input.Connection != nil {
			if err := validConnection(*input.Connection); err != nil {
				return err
			}
			out.Connection = *input.Connection
		}
		if input.Options != nil {
			out.Options.merge(*input.Options)
		}
		if out.Options.TimeoutMs < 1000 || out.Options.TimeoutMs > 300000 {
			return apperr.New(apperr.BadInput, "timeoutMs must be 1000-300000")
		}
		if input.Draft != nil {
			if len(input.Draft) > MaxDraftBytes {
				return apperr.New(apperr.BadInput, "draft exceeds 16 MiB")
			}
			out.Draft = input.Draft
		}
		return nil
	})
}

// SetSessionColor sets a colour or clears it with an empty string.
func (s *Service) SetSessionColor(ctx context.Context, id int64, color string) (Session, error) {
	if color != "" && !slices.Contains(history.Colors, color) {
		return Session{}, apperr.New(apperr.BadInput, "unknown colour")
	}
	return s.changeSession(ctx, id, func(out *Session) error { out.Color = color; return nil })
}

// DuplicateSession copies all draft state, without entries, right after its source.
func (s *Service) DuplicateSession(ctx context.Context, id int64) (out Session, err error) {
	db, err := s.db()
	if err != nil {
		return out, err
	}
	err = db.Write(ctx, func(tx *sql.Tx) error {
		out, err = session(ctx, tx, id)
		if err != nil {
			return err
		}
		var pos int
		if err := tx.QueryRowContext(ctx, `SELECT sort_order FROM replay_sessions WHERE id = ?`, id).Scan(&pos); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE replay_sessions SET sort_order = sort_order + 1 WHERE collection_id = ? AND sort_order > ?`, out.CollectionID, pos); err != nil {
			return err
		}
		runes := []rune(out.Name)
		out.Name = string(runes[:min(len(runes), 193)]) + " (copy)"
		return insertSession(ctx, tx, &out, pos+1)
	})
	return out, err
}

// MoveSession moves to a zero-based index, clamped to the destination's end.
func (s *Service) MoveSession(ctx context.Context, id, collectionID int64, index int) (out Session, err error) {
	if index < 0 {
		return out, apperr.New(apperr.BadInput, "index must not be negative")
	}
	db, err := s.db()
	if err != nil {
		return out, err
	}
	err = db.Write(ctx, func(tx *sql.Tx) error {
		out, err = session(ctx, tx, id)
		if err != nil {
			return err
		}
		if _, err := collection(ctx, tx, collectionID); err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, `SELECT id FROM replay_sessions WHERE collection_id = ? AND id != ? ORDER BY sort_order, id`, collectionID, id)
		if err != nil {
			return err
		}
		ids := []int64{}
		for rows.Next() {
			var n int64
			if err := rows.Scan(&n); err != nil {
				_ = rows.Close()
				return err
			}
			ids = append(ids, n)
		}
		err = rows.Err()
		closeErr := rows.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		ids = slices.Insert(ids, min(index, len(ids)), id)
		for i, n := range ids {
			if _, err := tx.ExecContext(ctx, `UPDATE replay_sessions SET collection_id = ?, sort_order = ? WHERE id = ?`, collectionID, i, n); err != nil {
				return err
			}
		}
		out.CollectionID = collectionID
		return nil
	})
	return out, err
}

// DeleteSession removes the session and its entries, preserving history records.
func (s *Service) DeleteSession(ctx context.Context, id int64) error {
	db, err := s.db()
	if err != nil {
		return err
	}
	return db.Write(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `DELETE FROM replay_sessions WHERE id = ?`, id)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return apperr.New(apperr.NotFound, "replay session %d not found", id)
		}
		return nil
	})
}
