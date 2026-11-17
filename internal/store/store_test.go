package store_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"os"

	"github.com/pocikode/ratatoskr/internal/store"
)

func newRoot(t *testing.T) *os.Root {
	t.Helper()
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Close() })
	return root
}

func openProject(t *testing.T) (*store.DB, *os.Root) {
	t.Helper()
	root := newRoot(t)
	db, err := store.OpenProject(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db, root
}

func TestPragmas(t *testing.T) {
	db, _ := openProject(t)
	for _, tc := range []struct {
		name   string
		pool   *sql.DB
		pragma string
		want   string
	}{
		{"reader journal", db.Reader(), "journal_mode", "wal"},
		{"reader fk", db.Reader(), "foreign_keys", "1"},
		{"reader busy", db.Reader(), "busy_timeout", "5000"},
		{"reader query_only", db.Reader(), "query_only", "1"},
	} {
		var got string
		if err := tc.pool.QueryRowContext(t.Context(), "PRAGMA "+tc.pragma).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
	err := db.Write(t.Context(), func(tx *sql.Tx) error {
		for pragma, want := range map[string]string{
			"synchronous": "1", "foreign_keys": "1", "busy_timeout": "5000", "journal_mode": "wal", "query_only": "0",
		} {
			var got string
			if err := tx.QueryRow("PRAGMA " + pragma).Scan(&got); err != nil {
				return err
			}
			if got != want {
				t.Errorf("writer %s: got %q want %q", pragma, got, want)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestInstanceTables(t *testing.T) {
	root := newRoot(t)
	db, err := store.OpenInstance(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, tbl := range []string{"settings", "projects", "ca", "workflows", "plugins", "plugin_storage",
		"hosted_files", "env_global", "ai_providers", "shortcuts"} {
		if _, err := db.Reader().ExecContext(t.Context(), "SELECT 1 FROM "+tbl+" LIMIT 1"); err != nil {
			t.Errorf("table %s: %v", tbl, err)
		}
	}
}

func TestProjectTables(t *testing.T) {
	db, _ := openProject(t)
	for _, tbl := range []string{"requests", "responses", "request_headers", "response_headers", "ws_streams",
		"ws_messages", "scope_presets", "filter_presets", "ws_filter_presets", "mr_collections", "mr_rules",
		"replay_collections", "replay_sessions", "replay_entries", "automate_sessions", "automate_attacks",
		"automate_results", "findings", "exports", "environments", "env_vars", "assistant_sessions",
		"assistant_messages", "intercept_settings", "ui_state"} {
		if _, err := db.Reader().ExecContext(t.Context(), "SELECT 1 FROM "+tbl+" LIMIT 1"); err != nil {
			t.Errorf("table %s: %v", tbl, err)
		}
	}
}

func TestMigrationsIdempotentAndNewer(t *testing.T) {
	root := newRoot(t)
	db, err := store.OpenProject(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	v := db.SchemaVersion()
	if v < 1 {
		t.Fatalf("version %d", v)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = store.OpenProject(t.Context(), root) // reopen: no-op
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.Reader().QueryRow("SELECT COUNT(*) FROM schema_version").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != v {
		t.Errorf("schema_version rows = %d, want %d", n, v)
	}
	err = db.Write(t.Context(), func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO schema_version VALUES (?, 'future', 0)", v+5)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	_, err = store.OpenProject(t.Context(), root)
	if !errors.Is(err, store.ErrNewerSchema) {
		t.Fatalf("want ErrNewerSchema, got %v", err)
	}
	if nse, ok := errors.AsType[*store.NewerSchemaError](err); !ok || nse.Have != v+5 {
		t.Errorf("NewerSchemaError = %v, %v", nse, ok)
	}
}

func TestConcurrentWriters(t *testing.T) {
	db, _ := openProject(t)
	const writers, perWriter = 100, 1000
	if testing.Short() {
		t.Skip("long")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()

	var wg sync.WaitGroup
	errc := make(chan error, writers)
	for w := range writers {
		wg.Go(func() {
			errc <- db.Write(ctx, func(tx *sql.Tx) error {
				stmt, err := tx.PrepareContext(ctx, "INSERT INTO ui_state (key, value_json) VALUES (?, '1')")
				if err != nil {
					return err
				}
				defer stmt.Close()
				for i := range perWriter {
					if _, err := stmt.ExecContext(ctx, fmt.Sprintf("%d/%d", w, i)); err != nil {
						return err
					}
				}
				return nil
			})
		})
	}
	wg.Wait()
	close(errc)
	for err := range errc {
		if err != nil {
			t.Fatal(err)
		}
	}
	var n int
	if err := db.Reader().QueryRow("SELECT COUNT(*) FROM ui_state").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != writers*perWriter {
		t.Errorf("rows = %d, want %d", n, writers*perWriter)
	}
}

func TestWriteRollbackAndReadDuringWrite(t *testing.T) {
	db, _ := openProject(t)
	boom := errors.New("boom")
	err := db.Write(t.Context(), func(tx *sql.Tx) error {
		if _, err := tx.Exec("INSERT INTO ui_state VALUES ('a','1')"); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
	var n int
	if err := db.Reader().QueryRow("SELECT COUNT(*) FROM ui_state").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("rollback failed, rows=%d", n)
	}
	// Readers are not blocked by an open write transaction (WAL).
	err = db.Write(t.Context(), func(tx *sql.Tx) error {
		if _, err := tx.Exec("INSERT INTO ui_state VALUES ('a','1')"); err != nil {
			return err
		}
		return db.Reader().QueryRow("SELECT COUNT(*) FROM ui_state").Scan(&n)
	})
	if err != nil || n != 0 {
		t.Errorf("read during write: n=%d err=%v", n, err)
	}
}

func TestSQLFunctions(t *testing.T) {
	db, _ := openProject(t)
	tests := []struct {
		query string
		want  any
	}{
		{`SELECT regexp('^ab+c$', 'abbbc')`, int64(1)},
		{`SELECT regexp('^ab+c$', 'abbbd')`, int64(0)},
		{`SELECT regexp('b+', CAST('xxbb' AS BLOB))`, int64(1)},
		{`SELECT regexp('a', NULL)`, nil},
		{`SELECT 'abbbc' REGEXP 'b{3}'`, int64(1)},
		{`SELECT bcontains_fold(CAST('Hello WORLD' AS BLOB), 'world')`, int64(1)},
		{`SELECT bcontains_fold(CAST('Hello' AS BLOB), 'xyz')`, int64(0)},
		{`SELECT bcontains_fold(x'00ff41', 'a')`, int64(1)},
		{`SELECT bcontains_fold('abc', '')`, int64(1)},
		{`SELECT bcontains_fold(NULL, 'a')`, nil},
	}
	for _, tc := range tests {
		var got any
		if err := db.Reader().QueryRowContext(t.Context(), tc.query).Scan(&got); err != nil {
			t.Errorf("%s: %v", tc.query, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s = %#v, want %#v", tc.query, got, tc.want)
		}
	}
	var x any
	if err := db.Reader().QueryRow(`SELECT regexp('(', 'x')`).Scan(&x); err == nil {
		t.Error("invalid pattern should error")
	}
}

func TestInsertRequests(t *testing.T) {
	db, _ := openProject(t)
	ids, err := db.InsertRequests(t.Context(), []store.Request{
		{Source: "proxy", Host: "a.test", Port: 443, TLS: true, Method: "GET", Path: "/", Raw: []byte("GET / HTTP/1.1\r\n\r\n"), CreatedAt: 1},
		{Source: "proxy", Host: "b.test", Port: 80, Method: "POST", Path: "/x", Raw: []byte("POST"), CreatedAt: 2},
	})
	if err != nil || len(ids) != 2 || ids[1] != ids[0]+1 {
		t.Fatalf("ids=%v err=%v", ids, err)
	}
	var tls, l int
	if err := db.Reader().QueryRow("SELECT tls, len FROM requests WHERE id = ?", ids[0]).Scan(&tls, &l); err != nil {
		t.Fatal(err)
	}
	if tls != 1 || l != 18 {
		t.Errorf("tls=%d len=%d", tls, l)
	}
}

func BenchmarkInsertRequests(b *testing.B) {
	root, err := os.OpenRoot(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	defer root.Close()
	db, err := store.OpenProject(b.Context(), root)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	raw := make([]byte, 512)
	batch := make([]store.Request, 200)
	for i := range batch {
		batch[i] = store.Request{Source: "proxy", Host: "example.test", Port: 443, TLS: true,
			Method: "GET", Path: "/bench", Raw: raw, CreatedAt: int64(i)}
	}
	b.ResetTimer()
	for b.Loop() {
		if _, err := db.InsertRequests(b.Context(), batch); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(len(batch))*float64(b.N)/b.Elapsed().Seconds(), "rows/s")
}
