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
