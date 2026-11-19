package store_test

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/pocikode/ratatoskr/internal/store"
)

func TestInsertRequestTxRollsBack(t *testing.T) {
	db, _ := openProject(t)
	errBoom := errors.New("boom")

	err := db.Write(t.Context(), func(tx *sql.Tx) error {
		if _, err := store.InsertRequestTx(t.Context(), tx, store.Request{Source: "proxy", Host: "a.test", Method: "GET", Path: "/", Raw: []byte("x"), CreatedAt: 1}); err != nil {
			return err
		}
		return errBoom
	})
	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}

	var n int
	if err := db.Reader().QueryRow("SELECT count(*) FROM requests").Scan(&n); err != nil || n != 0 {
		t.Fatalf("count=%d err=%v", n, err)
	}
}

func TestInsertRequestTruncation(t *testing.T) {
	t.Parallel()
	db, _ := openProject(t)
	for _, truncated := range []bool{false, true} {
		ids, err := db.InsertRequests(t.Context(), []store.Request{{Source: "proxy", Host: "a.test", Port: 80, Method: "GET", Path: "/", Raw: []byte("x"), Truncated: truncated}})
		if err != nil {
			t.Fatal(err)
		}
		var combined, requestOnly bool
		if err := db.Reader().QueryRowContext(t.Context(), `SELECT truncated, request_truncated FROM requests WHERE id = ?`, ids[0]).Scan(&combined, &requestOnly); err != nil {
			t.Fatal(err)
		}
		if combined != truncated || requestOnly != truncated {
			t.Fatalf("truncated=%v: combined=%v requestOnly=%v", truncated, combined, requestOnly)
		}
	}
}
