package replay

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/pocikode/ratatoskr/internal/apperr"
)

// Entry links one send to its History request and optional failure text.
type Entry struct {
	ID        int64
	RequestID int64
	Error     *string
	CreatedAt time.Time
	Cursor    string
}

// EntryPage is a stable page ordered by decreasing entry ID.
type EntryPage struct {
	Items       []Entry
	Total       int
	HasNext     bool
	HasPrevious bool
}

// AddEntry records a send, using its request's creation time as the send start.
// Empty errText stores NULL; failures retain their text even without a response.
func (s *Service) AddEntry(ctx context.Context, sessionID, requestID int64, errText string) (out Entry, err error) {
	db, err := s.db()
	if err != nil {
		return out, err
	}
	err = db.Write(ctx, func(tx *sql.Tx) error {
		var exists int
		err := tx.QueryRowContext(ctx, `SELECT 1 FROM replay_sessions WHERE id = ?`, sessionID).Scan(&exists)
		if errors.Is(err, sql.ErrNoRows) {
			return apperr.New(apperr.NotFound, "replay session %d not found", sessionID)
		}
		if err != nil {
			return err
		}
		var created int64
		err = tx.QueryRowContext(ctx, `SELECT created_at FROM requests WHERE id = ?`, requestID).Scan(&created)
		if errors.Is(err, sql.ErrNoRows) {
			return apperr.New(apperr.NotFound, "request %d not found", requestID)
		}
		if err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO replay_entries(session_id, request_id, created_at, error) VALUES (?, ?, ?, nullif(?, ''))`, sessionID, requestID, created, errText)
		if err != nil {
			return err
		}
		out.ID, err = res.LastInsertId()
		out.RequestID, out.CreatedAt = requestID, time.UnixMilli(created).UTC()
		if errText != "" {
			out.Error = new(errText)
		}
		out.Cursor = entryCursor(sessionID, out.ID)
		return err
	})
	return out, err
}

func entryCursor(sessionID, id int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(sessionID, 10) + ":" + strconv.FormatInt(id, 10)))
}

func cursorID(sessionID int64, after *string) (int64, error) {
	if after == nil {
		return 0, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(*after)
	parts := strings.Split(string(raw), ":")
	if err == nil && len(parts) == 2 && parts[0] == strconv.FormatInt(sessionID, 10) {
		id, err := strconv.ParseInt(parts[1], 10, 64)
		if err == nil && id > 0 {
			return id, nil
		}
	}
	return 0, apperr.New(apperr.BadInput, "invalid entry cursor for this session")
}

// Entries returns a snapshot page; deleted cursor entries remain valid boundaries.
func (s *Service) Entries(ctx context.Context, sessionID int64, first int, after *string) (out EntryPage, err error) {
	if first < 1 || first > 1000 {
		return out, apperr.New(apperr.BadInput, "first must be 1-1000")
	}
	boundary, err := cursorID(sessionID, after)
	if err != nil {
		return out, err
	}
	db, err := s.db()
	if err != nil {
		return out, err
	}
	err = db.View(ctx, func(tx *sql.Tx) error {
		var exists int
		err := tx.QueryRowContext(ctx, `SELECT 1 FROM replay_sessions WHERE id = ?`, sessionID).Scan(&exists)
		if errors.Is(err, sql.ErrNoRows) {
			return apperr.New(apperr.NotFound, "replay session %d not found", sessionID)
		}
		if err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM replay_entries WHERE session_id = ?`, sessionID).Scan(&out.Total); err != nil {
			return err
		}
		if after != nil {
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM replay_entries WHERE session_id = ? AND id >= ?)`, sessionID, boundary).Scan(&out.HasPrevious); err != nil {
				return err
			}
		}
		rows, err := tx.QueryContext(ctx, `SELECT id, request_id, error, created_at FROM replay_entries WHERE session_id = ? AND (? = 0 OR id < ?) ORDER BY id DESC LIMIT ?`, sessionID, boundary, boundary, first+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		out.Items = []Entry{}
		for rows.Next() {
			var entry Entry
			var created int64
			if err := rows.Scan(&entry.ID, &entry.RequestID, &entry.Error, &created); err != nil {
				return err
			}
			entry.CreatedAt, entry.Cursor = time.UnixMilli(created).UTC(), entryCursor(sessionID, entry.ID)
			out.Items = append(out.Items, entry)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		out.HasNext = len(out.Items) > first
		if out.HasNext {
			out.Items = out.Items[:first]
		}
		return nil
	})
	return out, err
}
