package httpql

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pocikode/ratatoskr/internal/query/core"
	"github.com/pocikode/ratatoskr/internal/store"
)

func mustParse(tb testing.TB, query string) core.Node {
	tb.Helper()
	n, err := Parse(query)
	if err != nil {
		tb.Fatalf("%q: %v", query, err)
	}
	return n
}

// BenchmarkCompile measures parsing-free compilation of a simple and a
// compound query (docs/04-query-language.md §4).
func BenchmarkCompile(b *testing.B) {
	for _, q := range []struct{ name, query string }{
		{"simple", `req.host.eq:"shop.example.com"`},
		{"compound", `req.method.eq:"GET" AND resp.code.gte:400 AND NOT req.ext.eq:".js" AND req.header["cookie"].cont:"session"`},
	} {
		n := mustParse(b, q.query)
		b.Run(q.name, func(b *testing.B) {
			for b.Loop() {
				if _, err := Compile(n); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// benchRows is the history size of BenchmarkQuery100k (docs/04 §4 budget:
// simple filters under 300 ms, body scans under 2 s).
const benchRows = 100_000

// BenchmarkQuery100k runs the budgeted queries against a 100k-row project.
func BenchmarkQuery100k(b *testing.B) {
	db := seedBenchDB(b, benchRows)
	for _, q := range []struct{ name, query string }{
		{"host-eq", `req.host.eq:"host-42.example.com"`},
		{"code-gte", `resp.code.gte:500`},
		{"raw-cont", `req.raw.cont:"needle-xyz"`},
	} {
		n := mustParse(b, q.query)
		compiled, err := Compile(n)
		if err != nil {
			b.Fatal(err)
		}
		stmt := `SELECT count(*) FROM requests r LEFT JOIN responses s ON s.request_id = r.id WHERE ` + compiled.Where
		b.Run(q.name, func(b *testing.B) {
			var matched int
			if err := db.Reader().QueryRowContext(b.Context(), stmt, compiled.Args...).Scan(&matched); err != nil {
				b.Fatal(err)
			}
			b.Logf("%d of %d rows match", matched, benchRows)
			for b.Loop() {
				if err := db.Reader().QueryRowContext(b.Context(), stmt, compiled.Args...).Scan(&matched); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func seedBenchDB(b *testing.B, rows int) *store.DB {
	b.Helper()
	root, err := os.OpenRoot(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	db, err := store.OpenProject(b.Context(), root)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if err := db.Close(); err != nil {
			b.Errorf("close: %v", err)
		}
	})

	plain := []byte(strings.Repeat("a", 120))
	needle := []byte("prefix needle-xyz suffix" + strings.Repeat("b", 90))
	const respBody = `{"ok":true,"id":1}`
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	err = db.Write(b.Context(), func(tx *sql.Tx) error {
		for i := range rows {
			host := fmt.Sprintf("host-%d.example.com", i%1000)
			path := fmt.Sprintf("/api/v1/items/%d", i)
			body := plain
			if i%100 == 0 {
				body = needle
			}
			head := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nAccept: application/json\r\nContent-Length: %d\r\n\r\n",
				path, host, len(body))
			raw := append([]byte(head), body...)
			id, err := store.InsertRequestTx(b.Context(), tx, store.Request{
				Source: "proxy", Host: host, Port: 443, TLS: true, Method: "GET",
				Path: path, Query: "", Ext: ".json", Raw: raw, BodyOffset: len(head),
				CreatedAt: start.Add(time.Duration(i) * time.Second).UnixMilli(),
			})
			if err != nil {
				return err
			}
			respRaw := fmt.Sprintf("HTTP/1.1 %d X\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n%s",
				i%600, len(respBody), respBody)
			if _, err := tx.ExecContext(b.Context(), `INSERT INTO responses
				(request_id, code, raw, body_offset, len, roundtrip_ms, created_at)
				VALUES (?, ?, ?, ?, ?, ?, ?)`,
				id, i%600, []byte(respRaw), strings.Index(respRaw, "\r\n\r\n")+4, len(respRaw),
				i%500, start.UnixMilli()); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		b.Fatalf("seed: %v", err)
	}
	return db
}
