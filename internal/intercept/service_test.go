package intercept

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/pocikode/ratatoskr/internal/apperr"
	"github.com/pocikode/ratatoskr/internal/query"
	"github.com/pocikode/ratatoskr/internal/query/core"
	"github.com/pocikode/ratatoskr/internal/scope"
	"github.com/pocikode/ratatoskr/internal/store"
)

type message struct {
	query.Message
	id     int64
	method string
	path   string
	req    query.Side
	resp   query.Response
}

func (m message) ID() int64                 { return m.id }
func (m message) Host() string              { return "example.test" }
func (m message) Port() int                 { return 80 }
func (m message) Method() string            { return m.method }
func (m message) Path() string              { return m.path }
func (m message) Request() query.Side       { return m.req }
func (m message) Response() *query.Response { return &m.resp }

func projectDB(t *testing.T) *store.DB {
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

func service(t *testing.T, selected func() *store.DB) *Service {
	t.Helper()
	s, err := New(t.Context(), selected)
	if err != nil {
		t.Fatal(err)
	}

	return s
}

func updateSettings(t *testing.T, s *Service, update Update) Settings {
	t.Helper()
	settings, err := s.UpdateSettings(t.Context(), update)
	if err != nil {
		t.Fatal(err)
	}

	return settings
}

func TestHoldMatching(t *testing.T) {
	t.Parallel()
	db := projectDB(t)
	selected := func() *store.DB { return db }
	preset, err := scope.New(selected).Create(t.Context(), scope.Preset{
		Name: "Only allowed path", Allow: []string{"example.test/allowed"},
	})
	if err != nil {
		t.Fatal(err)
	}

	s := service(t, selected)
	for _, tt := range []struct {
		name                         string
		queuing, requests, responses bool
		kind                         Kind
		expr, path                   string
		want                         bool
	}{
		{"queuing off", false, true, true, Request, "", "/allowed", false},
		{"requests off", true, false, true, Request, "", "/allowed", false},
		{"responses off", true, true, false, Response, "", "/allowed", false},
		{"request match", true, true, false, Request, `req.path.eq:"/allowed"`, "/allowed", true},
		{"query mismatch", true, true, true, Request, `req.path.eq:"/different"`, "/allowed", false},
		{"scope mismatch", true, true, true, Request, "", "/outside", false},
		{"absent response not equal", true, true, true, Request, "resp.code.ne:404", "/allowed", false},
		{"absent response equals zero", true, true, true, Request, "resp.code.eq:0", "/allowed", false},
		{"response match", true, false, true, Response, "resp.code.eq:200", "/allowed", true},
		{"response scope mismatch", true, true, true, Response, "resp.code.eq:200", "/outside", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			updateSettings(t, s, Update{
				Queuing: &tt.queuing, Requests: &tt.requests, Responses: &tt.responses,
				Query: &tt.expr, ScopeID: &preset.ID,
			})
			if want := tt.queuing && (tt.kind == Request && tt.requests || tt.kind == Response && tt.responses); s.WouldHold(tt.kind) != want {
				t.Fatalf("WouldHold(%s) = %v, want %v", tt.kind, s.WouldHold(tt.kind), want)
			}

			synctest.Test(t, func(t *testing.T) {
				result := startHold(t.Context(), t, s, db, tt.kind, message{
					id: 1, path: tt.path, resp: query.Response{Code: 200},
				})
				if got := len(s.Queue("")); (got == 1) != tt.want {
					t.Fatalf("held %d items, want held=%v", got, tt.want)
				}
				s.ReleaseAll("", false)
				<-result
			})
		})
	}
}

type outcome struct {
	Decision
	err error
}

// startHold is called inside a synctest bubble.
func startHold(ctx context.Context, t *testing.T, s *Service, db *store.DB, kind Kind, msg message) <-chan outcome {
	t.Helper()
	out := make(chan outcome, 1)
	go func() {
		hold := s.HoldRequest
		if kind == Response {
			hold = s.HoldResponse
		}
		result, err := hold(ctx, db, msg)
		out <- outcome{result, err}
	}()
	synctest.Wait()

	return out
}

func TestReleaseOrderAndCancellation(t *testing.T) {
	t.Parallel()
	db := projectDB(t)

	synctest.Test(t, func(t *testing.T) {
		s := service(t, func() *store.DB { return db })
		updateSettings(t, s, Update{Queuing: new(true), Responses: new(true)})
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		events := s.SubscribeQueue(ctx)
		kinds := []Kind{Request, Response, Request, Response}
		raw := []byte("original bytes")
		results := make([]<-chan outcome, len(kinds))
		for i, kind := range kinds {
			results[i] = startHold(ctx, t, s, db, kind, message{
				id: int64(i + 1), req: query.Side{Raw: raw}, resp: query.Response{Side: query.Side{Raw: raw}},
			})
			if event := <-events; !event.Added || event.Item.RequestID != int64(i+1) {
				t.Fatalf("unexpected enqueue: %+v", event)
			}
		}
		updateSettings(t, s, Update{Query: new(`req.path.eq:"/other"`)})
		if len(s.Queue("")) != 4 {
			t.Fatal("changing the query released held items")
		}
		updateSettings(t, s, Update{Requests: new(false)})
		checkReleased := func(i int) {
			t.Helper()
			if event := <-events; event.Added || event.Item.RequestID != int64(i+1) {
				t.Fatalf("out-of-order removal: %+v", event)
			}
			if got := <-results[i]; got.err != nil || got.Edited || got.DropStatusCode != 0 || !bytes.Equal(got.Raw, raw) {
				t.Fatalf("release = %+v", got)
			}
		}
		checkReleased(0)
		checkReleased(2)
		if got := s.Queue(""); len(got) != 2 || got[0].Kind != Response || got[1].Kind != Response {
			t.Fatalf("queue after disabling requests: %+v", got)
		}
		updateSettings(t, s, Update{Queuing: new(false)})
		checkReleased(1)
		checkReleased(3)

		updateSettings(t, s, Update{Queuing: new(true), Query: new("")})
		client, disconnect := context.WithCancel(ctx)
		result := startHold(client, t, s, db, Response, message{id: 5})
		added := <-events
		disconnect()
		synctest.Wait()
		if got := <-result; !errors.Is(got.err, context.Canceled) {
			t.Fatalf("disconnected hold = %+v", got)
		}
		if got := <-events; got.Added || got.Item.ID != added.Item.ID || len(s.Queue("")) != 0 {
			t.Fatalf("disconnect removal = %+v", got)
		}
	})
}

func TestProjectSwitch(t *testing.T) {
	t.Parallel()
	a, b := projectDB(t), projectDB(t)
	var selected atomic.Pointer[store.DB]
	selected.Store(b)

	synctest.Test(t, func(t *testing.T) {
		s := service(t, selected.Load)
		updateSettings(t, s, Update{Queuing: new(true), Requests: new(false), DropStatusCode: new(418)})
		selected.Store(a)
		updateSettings(t, s, Update{Queuing: new(true)})
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		queueChanges := s.SubscribeQueue(ctx)
		settingsChanges := s.SubscribeSettings(ctx)
		held := startHold(ctx, t, s, a, Request, message{id: 1, req: query.Side{Raw: []byte("original")}})
		added := <-queueChanges
		if added.Item.DB != a {
			t.Fatal("hold lost its project identity")
		}

		selected.Store(b)
		if got := s.Queue(""); len(got) != 0 {
			t.Fatalf("visible old-project items = %+v", got)
		}
		if err := s.Forward(added.Item.ID, nil, true); apperr.CodeOf(err) != apperr.NotFound {
			t.Fatalf("forward from another DB = %v", err)
		}
		if err := s.Drop(added.Item.ID); apperr.CodeOf(err) != apperr.NotFound {
			t.Fatalf("drop from another DB = %v", err)
		}
		if s.ReleaseAll("", false) != 0 || s.ReleaseAll("", true) != 0 {
			t.Fatal("bulk release included old-project items")
		}
		if err := s.ProjectChanged(ctx); err != nil {
			t.Fatal(err)
		}
		if got := <-held; got.err != nil || string(got.Raw) != "original" {
			t.Fatalf("project release = %+v", got)
		}
		if got := <-settingsChanges; got.Queuing || got.Requests || got.DropStatusCode != 418 {
			t.Fatalf("new settings = %+v", got)
		}
		synctest.Wait()
		select {
		case event := <-queueChanges:
			t.Fatalf("unexpected project-switch queue event: %+v", event)
		default:
		}
	})
}

func TestSettingsValidationAndPersistence(t *testing.T) {
	t.Parallel()
	db := projectDB(t)
	selected := func() *store.DB { return db }
	s := service(t, selected)
	preset, err := scope.New(selected).Create(t.Context(), scope.Preset{Name: "Test scope"})
	if err != nil {
		t.Fatal(err)
	}
	want := updateSettings(t, s, Update{
		Queuing: new(true), Responses: new(true), Requests: new(false),
		Query: new(`req.path.eq:"/example"`), ScopeID: &preset.ID, DropStatusCode: new(400),
	})
	for _, tt := range []struct {
		change Update
		code   apperr.Code
	}{
		{Update{Queuing: new(false), DropStatusCode: new(99)}, apperr.BadInput},
		{Update{DropStatusCode: new(600)}, apperr.BadInput},
		{Update{Query: new("req.unknown.eq:0")}, apperr.BadQuery},
		{Update{ScopeID: new(preset.ID + 1)}, apperr.NotFound},
	} {
		_, err := s.UpdateSettings(t.Context(), tt.change)
		if apperr.CodeOf(err) != tt.code {
			t.Fatalf("update = %v, want %s", err, tt.code)
		}
		if tt.code == apperr.BadQuery {
			if e, ok := errors.AsType[*apperr.Error](err); !ok || e.Span == nil || e.Span.End <= e.Span.Start {
				t.Fatalf("missing query error span: %v", err)
			}
		}
	}
	if got, err := s.Settings(t.Context()); err != nil || got != want {
		t.Fatalf("settings after rejected updates = %+v, %v", got, err)
	}
	root, err := os.OpenRoot(filepath.Dir(db.Path()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	reopenedDB, err := store.OpenProject(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopenedDB.Close() })
	reopened := service(t, func() *store.DB { return reopenedDB })
	want.Queuing = false
	if got, err := reopened.Settings(t.Context()); err != nil || got != want {
		t.Fatalf("reopened settings = %+v, %v, want %+v", got, err, want)
	}
}

func TestForwardLimitAndRetainsRejectedItems(t *testing.T) {
	t.Parallel()
	db := projectDB(t)
	s := service(t, func() *store.DB { return db })
	updateSettings(t, s, Update{Queuing: new(true), DropStatusCode: new(418)})

	synctest.Test(t, func(t *testing.T) {
		head := "POST / HTTP/1.1\r\nContent-Length: 0\r\n\r\n"
		exact := append([]byte(head), bytes.Repeat([]byte("x"), MaxEditBytes-len(head))...)
		over := slices.Concat(exact, []byte("x"))
		conflict := []byte("POST / HTTP/1.1\r\nContent-Length: 1\r\nContent-Length: 2\r\n\r\nx")
		original := []byte(head + "original bytes")

		held := startHold(t.Context(), t, s, db, Request, message{id: 1, req: query.Side{Raw: original}})
		id := s.Queue("")[0].ID
		for i, raw := range [][]byte{over, conflict, {}, []byte("broken\r\n\r\n")} {
			err := s.Forward(id, raw, false)
			if apperr.CodeOf(err) != apperr.BadInput || len(s.Queue("")) != 1 {
				t.Fatalf("invalid replacement %d = %v, queue count %d", i, err, len(s.Queue("")))
			}
			if i == 1 && !strings.Contains(err.Error(), "conflicting Content-Length") {
				t.Fatalf("missing parse error: %v", err)
			}
		}
		if err := s.Forward(id, exact, false); err != nil {
			t.Fatal(err)
		}
		if got := <-held; got.err != nil || !got.Edited || !bytes.Equal(got.Raw, exact) {
			t.Fatalf("exact-limit forward: edited=%v, len=%d, err=%v", got.Edited, len(got.Raw), got.err)
		}
		for _, drop := range []bool{false, true} {
			held = startHold(t.Context(), t, s, db, Request, message{id: 2, req: query.Side{Raw: over}})
			item := s.Queue("")[0]
			if !bytes.Equal(item.Raw, over) {
				t.Fatal("oversized hold lost bytes")
			}
			if err := s.Forward(item.ID, original, true); apperr.CodeOf(err) != apperr.BadInput || len(s.Queue("")) != 1 {
				t.Fatalf("edit oversized hold = %v", err)
			}
			if drop {
				if err := s.Drop(item.ID); err != nil {
					t.Fatal(err)
				}
				if got := <-held; got.err != nil || got.DropStatusCode != 418 || got.Raw != nil {
					t.Fatalf("drop: status=%d, len=%d, err=%v", got.DropStatusCode, len(got.Raw), got.err)
				}
			} else {
				if err := s.Forward(item.ID, nil, true); err != nil {
					t.Fatal(err)
				}
				if got := <-held; got.err != nil || got.Edited || !bytes.Equal(got.Raw, over) {
					t.Fatalf("unchanged forward: edited=%v, len=%d, err=%v", got.Edited, len(got.Raw), got.err)
				}
			}
		}
	})
}

func TestQueueLimits(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name        string
		count, size int
	}{
		{"count", 1_000, 1},
		{"bytes", 4, 64 << 20},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := projectDB(t)
			// The holds can share a backing slice because their bytes are immutable.
			side := query.Side{Raw: make([]byte, tt.size)}
			synctest.Test(t, func(t *testing.T) {
				s := service(t, func() *store.DB { return db })
				updateSettings(t, s, Update{Queuing: new(true), Responses: new(true)})
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				events := s.SubscribeQueue(ctx)
				for i := range tt.count {
					kind := Request
					if i%2 == 1 {
						kind = Response
					}
					startHold(ctx, t, s, db, kind, message{
						id: int64(i + 1), req: side, resp: query.Response{Side: side},
					})
					select {
					case event := <-events:
						if !event.Added || event.Item.RequestID != int64(i+1) {
							t.Fatalf("unexpected add event for item %d", i+1)
						}
					default:
						t.Fatalf("no add event for item %d", i+1)
					}
				}
				extra := []byte("x")
				msg := message{id: int64(tt.count + 1), req: query.Side{Raw: extra}}
				result := startHold(ctx, t, s, db, Request, msg)
				assertPassThrough(t, result, extra)
				select {
				case <-events:
					t.Fatal("queue overflow emitted an event")
				default:
				}
				queue := s.Queue("")
				if len(queue) != tt.count || queue[0].RequestID != 1 || queue[len(queue)-1].RequestID != int64(tt.count) {
					t.Fatalf("overflow changed the queue of %d items", tt.count)
				}
				if err := s.Forward(queue[0].ID, nil, false); err != nil {
					t.Fatal(err)
				}
				client, disconnect := context.WithCancel(ctx)
				defer disconnect()
				result = startHold(client, t, s, db, Request, msg)
				if got := len(s.Queue("")); got != tt.count {
					t.Fatalf("queue after refilling = %d, want %d", got, tt.count)
				}
				disconnect()
				synctest.Wait()
				if got := <-result; !errors.Is(got.err, context.Canceled) {
					t.Fatalf("canceled hold: %v", got.err)
				}
				if got := s.ReleaseAll("", false); got != tt.count-1 || s.queueBytes != 0 {
					t.Fatalf("bulk release removed %d items, leaving %d bytes", got, s.queueBytes)
				}
			})
		})
	}
}

func TestForwardResponseContentLength(t *testing.T) {
	t.Parallel()
	db := projectDB(t)
	s := service(t, func() *store.DB { return db })
	updateSettings(t, s, Update{Queuing: new(true), Responses: new(true)})
	for _, tt := range []struct {
		method, status, wantLength string
	}{
		{"HEAD", "200 OK", "1234"},
		{"GET", "103 Early Hints", "1234"},
		{"GET", "204 No Content", "1234"},
		{"GET", "304 Not Modified", "1234"},
		{"GET", "200 OK", "0"},
	} {
		t.Run(tt.method+" "+tt.status, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				head := "HTTP/1.1 " + tt.status + "\r\n"
				original := []byte(head + "\r\n")
				edited := []byte(head + "Content-Length: 1234\r\n\r\n")
				want := head + "Content-Length: " + tt.wantLength + "\r\n\r\n"
				result := startHold(t.Context(), t, s, db, Response, message{
					id: 1, method: tt.method, resp: query.Response{Side: query.Side{Raw: original}},
				})
				if err := s.Forward(s.Queue("")[0].ID, edited, true); err != nil {
					t.Fatal(err)
				}
				if got := <-result; got.err != nil || !got.Edited || string(got.Raw) != want {
					t.Fatalf("edited response = %q, edited=%v, err=%v; want %q", got.Raw, got.Edited, got.err, want)
				}
			})
		})
	}
}

func TestHoldPassesThroughOnConditionError(t *testing.T) {
	t.Parallel()
	db := projectDB(t)
	selected := func() *store.DB { return db }
	scopes := scope.New(selected)
	preset, err := scopes.Create(t.Context(), scope.Preset{Name: "Active scope"})
	if err != nil {
		t.Fatal(err)
	}
	s := service(t, selected)
	updateSettings(t, s, Update{Queuing: new(true), ScopeID: &preset.ID})
	if err := scopes.Delete(t.Context(), preset.ID); err != nil {
		t.Fatal(err)
	}
	check := func(t *testing.T) {
		t.Helper()
		synctest.Test(t, func(t *testing.T) {
			raw := []byte("original")
			result := startHold(t.Context(), t, s, db, Request, message{id: 1, req: query.Side{Raw: raw}})
			assertPassThrough(t, result, raw)
		})
	}
	t.Run("deleted scope", check)
	rules := *s.rules.Load()
	rules.settings.ScopeID = 0
	rules.requestQuery = &core.Preset{Name: "unexpanded"}
	s.rules.Store(&rules)
	t.Run("evaluation error", check)
}

func assertPassThrough(t *testing.T, result <-chan outcome, raw []byte) {
	t.Helper()
	select {
	case got := <-result:
		if got.err != nil || got.Edited || got.DropStatusCode != 0 || !bytes.Equal(got.Raw, raw) {
			t.Fatalf("pass-through decision = %+v", got)
		}
	default:
		t.Fatal("hold blocked instead of passing through")
	}
}
