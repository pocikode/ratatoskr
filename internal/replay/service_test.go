package replay_test

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/pocikode/ratatoskr/internal/apperr"
	"github.com/pocikode/ratatoskr/internal/replay"
	"github.com/pocikode/ratatoskr/internal/store"
)

func openDB(t *testing.T) *store.DB {
	t.Helper()
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	db, err := store.OpenProject(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func newService(t *testing.T) (*replay.Service, *store.DB) {
	t.Helper()
	db := openDB(t)
	return replay.New(func() *store.DB { return db }), db
}

func scratch() replay.CreateInput {
	return replay.CreateInput{Connection: &replay.Connection{Host: "example.test", Port: 443, TLS: true}, Raw: []byte("GET /café?q=1 HTTP/1.1\r\nHost: preserved.test\r\n\r\n\x00\xff")}
}

func create(t *testing.T, s *replay.Service, in replay.CreateInput) replay.Session {
	t.Helper()
	out, err := s.CreateSession(t.Context(), in)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func seedRequest(t *testing.T, db *store.DB, raw []byte, truncated bool) int64 {
	t.Helper()
	var id int64
	err := db.Write(t.Context(), func(tx *sql.Tx) error {
		res, err := tx.ExecContext(t.Context(), `INSERT INTO requests(source, host, port, tls, method, path, raw, len, created_at, truncated) VALUES ('replay', 'source.test', 8443, 1, 'POST', '/raw', ?, ?, 1234, ?)`, raw, len(raw), truncated)
		if err != nil {
			return err
		}
		id, err = res.LastInsertId()
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func sessionIDs(t *testing.T, s *replay.Service, c int64) []int64 {
	t.Helper()
	out, err := s.Sessions(t.Context(), c)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]int64, len(out))
	for i, item := range out {
		ids[i] = item.ID
	}
	return ids
}

func TestDefaultConcurrentAndCollections(t *testing.T) {
	t.Parallel()
	s, _ := newService(t)
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			cs, err := s.Collections(t.Context())
			if err != nil || len(cs) != 1 || !cs[0].IsDefault {
				t.Errorf("collections = %+v, %v", cs, err)
			}
		})
	}
	wg.Wait()
	cs, err := s.Collections(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	def := cs[0]
	renamed, err := s.RenameCollection(t.Context(), def.ID, "  Main  ")
	if err != nil || renamed.Name != "Main" || !renamed.IsDefault {
		t.Fatalf("rename = %+v, %v", renamed, err)
	}
	if err := s.DeleteCollection(t.Context(), def.ID); apperr.CodeOf(err) != apperr.Conflict {
		t.Fatalf("delete Default: %v", err)
	}
	a, err := s.CreateCollection(t.Context(), "A")
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateCollection(t.Context(), "A")
	if err != nil {
		t.Fatal(err)
	}
	order := []int64{b.ID, def.ID, a.ID}
	cs, err = s.ReorderCollections(t.Context(), order)
	if err != nil || cs[0].ID != b.ID || cs[1].ID != def.ID {
		t.Fatalf("reorder = %+v, %v", cs, err)
	}
	for _, bad := range [][]int64{nil, {a.ID}, {a.ID, b.ID, def.ID, 99}, {a.ID, a.ID, def.ID}, {a.ID, b.ID, 99}} {
		if _, err := s.ReorderCollections(t.Context(), bad); apperr.CodeOf(err) != apperr.BadInput {
			t.Errorf("reorder %v: %v", bad, err)
		}
	}
	cs, err = s.Collections(t.Context())
	if err != nil || cs[0].ID != b.ID {
		t.Fatalf("invalid reorder changed order: %+v %v", cs, err)
	}
	created := create(t, s, scratch())
	if created.CollectionID != def.ID {
		t.Fatal("lost Default after reorder/rename")
	}
	if _, err := s.RenameCollection(t.Context(), 999, "x"); apperr.CodeOf(err) != apperr.NotFound {
		t.Fatal(err)
	}
	if err := s.DeleteCollection(t.Context(), 999); apperr.CodeOf(err) != apperr.NotFound {
		t.Fatal(err)
	}
}

func TestHistoryAndDraftFidelity(t *testing.T) {
	t.Parallel()
	s, db := newService(t)
	raw := []byte("M-SEARCH /à?q=東京 HTTP/1.1\nHoST: unchanged\nContent-Length: 99\n\n\xff\x00\r\n")
	id := seedRequest(t, db, raw, false)
	out := create(t, s, replay.CreateInput{FromRequestID: &id})
	if !bytes.Equal(out.Draft, raw) || out.Name != "M-SEARCH /à?q=東京" || out.Connection.Host != "source.test" || out.Connection.Port != 8443 || !out.Connection.TLS || out.Connection.SNI == nil || *out.Connection.SNI != "source.test" {
		t.Fatalf("history copy = %+v", out)
	}
	stored, err := s.Session(t.Context(), out.ID)
	if err != nil || !reflect.DeepEqual(stored, out) {
		t.Fatalf("stored = %+v, %v", stored, err)
	}
	for _, source := range []int64{seedRequest(t, db, raw, true), seedRequest(t, db, make([]byte, replay.MaxDraftBytes+1), false)} {
		if _, err := s.CreateSession(t.Context(), replay.CreateInput{FromRequestID: &source}); apperr.CodeOf(err) != apperr.BadInput {
			t.Errorf("source %d: %v", source, err)
		}
	}
	if _, err := s.CreateSession(t.Context(), replay.CreateInput{FromRequestID: new(int64(999))}); apperr.CodeOf(err) != apperr.NotFound {
		t.Fatal(err)
	}
	in := scratch()
	in.Raw = []byte("malformed\xff")
	if got := create(t, s, in); got.Name != "Untitled" || !bytes.Equal(got.Draft, in.Raw) {
		t.Fatal(got)
	}
	in.Raw = []byte("GET /" + strings.Repeat("界", 70) + " HTTP/1.0\r\n\r\n")
	if got := create(t, s, in); len([]rune(got.Name)) != 60 {
		t.Fatal(got.Name)
	}
	in.Raw = []byte{}
	empty := create(t, s, in)
	if len(empty.Draft) != 0 || empty.Name != "Untitled" {
		t.Fatal(empty)
	}
}

func TestUpdateDuplicateMoveAndCascade(t *testing.T) {
	t.Parallel()
	s, db := newService(t)
	a, b, c := create(t, s, scratch()), create(t, s, scratch()), create(t, s, scratch())
	req := seedRequest(t, db, []byte("GET / HTTP/1.1\r\n\r\n"), false)
	if _, err := s.AddEntry(t.Context(), a.ID, req, "dial failed"); err != nil {
		t.Fatal(err)
	}
	next := replay.UpdateInput{Name: new(strings.Repeat("界", 200)), Connection: &replay.Connection{Host: "new.test", Port: 80}, Options: &replay.OptionsInput{FollowRedirects: new(true), UpdateContentLength: new(false), TimeoutMs: new(1000), ApplyMatchReplace: new(true)}, Draft: []byte("draft\x00\xff")}
	updated, err := s.UpdateSession(t.Context(), a.ID, next)
	if err != nil {
		t.Fatal(err)
	}
	updated, err = s.SetSessionColor(t.Context(), a.ID, "BLUE")
	if err != nil {
		t.Fatal(err)
	}
	dup, err := s.DuplicateSession(t.Context(), a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if dup.Name != strings.Repeat("界", 193)+" (copy)" || !reflect.DeepEqual(dup.State, updated.State) || !bytes.Equal(dup.Draft, updated.Draft) || dup.ID == a.ID {
		t.Fatalf("copy = %+v", dup)
	}
	p, err := s.Entries(t.Context(), dup.ID, 100, nil)
	if err != nil || p.Total != 0 {
		t.Fatalf("copy entries: %+v %v", p, err)
	}
	if ids := sessionIDs(t, s, a.CollectionID); !slices.Equal(ids, []int64{a.ID, dup.ID, b.ID, c.ID}) {
		t.Fatal(ids)
	}
	if _, err := s.MoveSession(t.Context(), c.ID, a.CollectionID, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MoveSession(t.Context(), a.ID, a.CollectionID, 999); err != nil {
		t.Fatal(err)
	}
	if ids := sessionIDs(t, s, a.CollectionID); !slices.Equal(ids, []int64{c.ID, dup.ID, b.ID, a.ID}) {
		t.Fatal(ids)
	}
	target, err := s.CreateCollection(t.Context(), "Target")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.MoveSession(t.Context(), a.ID, target.ID, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MoveSession(t.Context(), b.ID, target.ID, 0); err != nil {
		t.Fatal(err)
	}
	if ids := sessionIDs(t, s, target.ID); !slices.Equal(ids, []int64{b.ID, a.ID}) {
		t.Fatal(ids)
	}
	if _, err := s.MoveSession(t.Context(), b.ID, target.ID, -1); apperr.CodeOf(err) != apperr.BadInput {
		t.Fatal(err)
	}
	if _, err := s.MoveSession(t.Context(), b.ID, 999, 0); apperr.CodeOf(err) != apperr.NotFound {
		t.Fatal(err)
	}
	if _, err := s.SetSessionColor(t.Context(), dup.ID, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteCollection(t.Context(), target.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Session(t.Context(), a.ID); apperr.CodeOf(err) != apperr.NotFound {
		t.Fatal(err)
	}
	var entries, requests int
	if err := db.Reader().QueryRowContext(t.Context(), `SELECT count(*) FROM replay_entries`).Scan(&entries); err != nil {
		t.Fatal(err)
	}
	if err := db.Reader().QueryRowContext(t.Context(), `SELECT count(*) FROM requests WHERE id = ?`, req).Scan(&requests); err != nil {
		t.Fatal(err)
	}
	if entries != 0 || requests != 1 {
		t.Fatalf("entries=%d requests=%d", entries, requests)
	}
	if _, err := s.AddEntry(t.Context(), dup.ID, req, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteSession(t.Context(), dup.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.Reader().QueryRowContext(t.Context(), `SELECT count(*) FROM replay_entries`).Scan(&entries); err != nil || entries != 0 {
		t.Fatalf("session cascade = %d %v", entries, err)
	}
	if err := s.DeleteSession(t.Context(), dup.ID); apperr.CodeOf(err) != apperr.NotFound {
		t.Fatal(err)
	}
}

func TestValidationAndAtomicUpdate(t *testing.T) {
	t.Parallel()
	s, _ := newService(t)
	original := create(t, s, scratch())
	for _, name := range []string{"", "  ", strings.Repeat("界", 201), "a\x00b", "a\nb"} {
		if _, err := s.CreateCollection(t.Context(), name); apperr.CodeOf(err) != apperr.BadInput {
			t.Errorf("name %q: %v", name, err)
		}
		in := scratch()
		in.Name = &name
		if _, err := s.CreateSession(t.Context(), in); apperr.CodeOf(err) != apperr.BadInput {
			t.Errorf("session name: %v", err)
		}
	}
	for _, in := range []replay.CreateInput{{}, {Connection: scratch().Connection}, {Raw: []byte{}}, {FromRequestID: new(int64(1)), Raw: []byte{}}, {FromRequestID: new(int64(1)), Connection: scratch().Connection}, {Connection: scratch().Connection, Raw: make([]byte, replay.MaxDraftBytes+1)}, {CollectionID: new(int64(999)), Connection: scratch().Connection, Raw: []byte{}}} {
		_, err := s.CreateSession(t.Context(), in)
		if apperr.CodeOf(err) != apperr.BadInput && apperr.CodeOf(err) != apperr.NotFound {
			t.Errorf("invalid source: %v", err)
		}
	}
	for _, c := range []replay.Connection{{Host: "", Port: 80}, {Host: "a b", Port: 80}, {Host: "a\x00", Port: 80}, {Host: "a\u2003b", Port: 80}, {Host: "a", Port: 0}, {Host: "a", Port: 65536}, {Host: "a", Port: 80, SNI: new("")}} {
		in := scratch()
		in.Connection = &c
		if _, err := s.CreateSession(t.Context(), in); apperr.CodeOf(err) != apperr.BadInput {
			t.Fatal(err)
		}
		if _, err := s.UpdateSession(t.Context(), original.ID, replay.UpdateInput{Name: new("changed"), Connection: &c}); apperr.CodeOf(err) != apperr.BadInput {
			t.Fatal(err)
		}
	}
	for _, timeout := range []int{999, 300001} {
		if _, err := s.UpdateSession(t.Context(), original.ID, replay.UpdateInput{Name: new("changed"), Options: &replay.OptionsInput{TimeoutMs: &timeout}}); apperr.CodeOf(err) != apperr.BadInput {
			t.Fatal(err)
		}
	}
	if _, err := s.UpdateSession(t.Context(), original.ID, replay.UpdateInput{Draft: make([]byte, replay.MaxDraftBytes+1)}); apperr.CodeOf(err) != apperr.BadInput {
		t.Fatal(err)
	}
	stored, err := s.Session(t.Context(), original.ID)
	if err != nil || !reflect.DeepEqual(stored, original) {
		t.Fatalf("invalid updates changed draft: %+v %v", stored, err)
	}
	for _, timeout := range []int{1000, 300000} {
		if _, err := s.UpdateSession(t.Context(), original.ID, replay.UpdateInput{Options: &replay.OptionsInput{TimeoutMs: &timeout}}); err != nil {
			t.Fatal(err)
		}
	}
	out, err := s.UpdateSession(t.Context(), original.ID, replay.UpdateInput{Draft: []byte{}})
	if err != nil || len(out.Draft) != 0 {
		t.Fatalf("clear: %+v %v", out, err)
	}
	out, err = s.UpdateSession(t.Context(), original.ID, replay.UpdateInput{})
	if err != nil || len(out.Draft) != 0 || out.Options.TimeoutMs != 300000 {
		t.Fatalf("no-op: %+v %v", out, err)
	}
	if _, err := s.SetSessionColor(t.Context(), original.ID, "bogus"); apperr.CodeOf(err) != apperr.BadInput {
		t.Fatal(err)
	}
	for _, call := range []func() error{
		func() error { _, err := s.UpdateSession(t.Context(), 999, replay.UpdateInput{}); return err },
		func() error { _, err := s.DuplicateSession(t.Context(), 999); return err },
		func() error { _, err := s.MoveSession(t.Context(), 999, original.CollectionID, 0); return err },
		func() error { _, err := s.SetSessionColor(t.Context(), 999, ""); return err },
	} {
		if err := call(); apperr.CodeOf(err) != apperr.NotFound {
			t.Fatal(err)
		}
	}
}

func TestEntriesPagingAndDeletedCursor(t *testing.T) {
	t.Parallel()
	s, db := newService(t)
	a, b := create(t, s, scratch()), create(t, s, scratch())
	var entries []replay.Entry
	var requests []int64
	for range 4 {
		id := seedRequest(t, db, []byte{}, false)
		requests = append(requests, id)
		e, err := s.AddEntry(t.Context(), a.ID, id, "timeout after 1000 ms")
		if err != nil {
			t.Fatal(err)
		}
		if e.Error == nil || *e.Error != "timeout after 1000 ms" || e.CreatedAt.UnixMilli() != 1234 {
			t.Fatal(e)
		}
		entries = append(entries, e)
	}
	p, err := s.Entries(t.Context(), a.ID, 2, nil)
	if err != nil || p.Total != 4 || len(p.Items) != 2 || p.Items[0].ID != entries[3].ID || p.Items[1].ID != entries[2].ID || !p.HasNext || p.HasPrevious {
		t.Fatalf("page = %+v %v", p, err)
	}
	after := p.Items[1].Cursor
	// Delete the cursor entry through History and insert a newer entry between pages.
	err = db.Write(t.Context(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(t.Context(), `DELETE FROM requests WHERE id = ?`, requests[2])
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	e, err := s.AddEntry(t.Context(), a.ID, requests[0], "")
	if err != nil || e.Error != nil {
		t.Fatalf("success = %+v %v", e, err)
	}
	p, err = s.Entries(t.Context(), a.ID, 2, &after)
	if err != nil || p.Total != 4 || len(p.Items) != 2 || p.Items[0].ID != entries[1].ID || p.Items[1].ID != entries[0].ID || p.HasNext || !p.HasPrevious {
		t.Fatalf("next = %+v %v", p, err)
	}
	for _, first := range []int{0, 1001} {
		if _, err := s.Entries(t.Context(), a.ID, first, nil); apperr.CodeOf(err) != apperr.BadInput {
			t.Fatal(err)
		}
	}
	for _, cursor := range []string{"bad", entries[0].Cursor} {
		if _, err := s.Entries(t.Context(), b.ID, 1, &cursor); apperr.CodeOf(err) != apperr.BadInput {
			t.Fatal(err)
		}
	}
	if _, err := s.Entries(t.Context(), 999, 1, nil); apperr.CodeOf(err) != apperr.NotFound {
		t.Fatal(err)
	}
	if _, err := s.AddEntry(t.Context(), 999, requests[0], ""); apperr.CodeOf(err) != apperr.NotFound {
		t.Fatal(err)
	}
	if _, err := s.AddEntry(t.Context(), a.ID, 999, ""); apperr.CodeOf(err) != apperr.NotFound {
		t.Fatal(err)
	}
	// Removing the newest entry must not allow its cursor ID to be reused.
	err = db.Write(t.Context(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(t.Context(), `DELETE FROM replay_entries WHERE id = ?`, e.ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	newer, err := s.AddEntry(t.Context(), a.ID, requests[0], "")
	if err != nil || newer.ID <= e.ID {
		t.Fatalf("reused id: %+v %v", newer, err)
	}
}

func TestProjectIsolationAndNoProject(t *testing.T) {
	t.Parallel()
	one, two := openDB(t), openDB(t)
	current := one
	s := replay.New(func() *store.DB { return current })
	a := create(t, s, scratch())
	current = two
	cs, err := s.Collections(t.Context())
	if err != nil || len(cs) != 1 {
		t.Fatal(cs, err)
	}
	ss, err := s.Sessions(t.Context(), cs[0].ID)
	if err != nil || len(ss) != 0 {
		t.Fatal(ss, err)
	}
	current = one
	if out, err := s.Session(t.Context(), a.ID); err != nil || !bytes.Equal(out.Draft, a.Draft) {
		t.Fatal(out, err)
	}
	current = nil
	if _, err := s.Collections(t.Context()); apperr.CodeOf(err) != apperr.Conflict {
		t.Fatal(err)
	}
	if _, err := replay.New(nil).Session(context.Background(), a.ID); apperr.CodeOf(err) != apperr.Conflict {
		t.Fatal(err)
	}
}
