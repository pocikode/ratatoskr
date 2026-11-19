package resolvers_test

import (
	"context"
	"encoding/base64"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/99designs/gqlgen/client"

	"github.com/pocikode/ratatoskr/internal/api/graphql"
	"github.com/pocikode/ratatoskr/internal/api/graphql/generated"
	"github.com/pocikode/ratatoskr/internal/api/graphql/resolvers"
	"github.com/pocikode/ratatoskr/internal/history"
	"github.com/pocikode/ratatoskr/internal/intercept"
	"github.com/pocikode/ratatoskr/internal/query"
	"github.com/pocikode/ratatoskr/internal/store"
)

type interceptHistory struct {
	resolvers.HistoryService
	current func() *store.DB
	calls   atomic.Int32
}

func (h *interceptHistory) Get(_ context.Context, id int64) (history.Request, error) {
	h.calls.Add(1)

	return history.Request{ID: id, Host: h.current().Path(), Source: "proxy", Method: "POST"}, nil
}

type interceptMessage struct {
	query.Message
	id  int64
	raw []byte
}

func (m interceptMessage) ID() int64           { return m.id }
func (m interceptMessage) Request() query.Side { return query.Side{Raw: m.raw} }

func interceptDB(t *testing.T) *store.DB {
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

func TestInterceptResolvers(t *testing.T) {
	t.Parallel()

	a, b := interceptDB(t), interceptDB(t)
	var db atomic.Pointer[store.DB]
	db.Store(a)

	synctest.Test(t, func(t *testing.T) {
		svc, err := intercept.New(t.Context(), db.Load)
		if err != nil {
			t.Fatal(err)
		}
		history := &interceptHistory{current: db.Load}
		r := resolvers.New(resolvers.Deps{Intercept: svc, History: history})
		c := client.New(graphql.NewHandler(graphql.Options{Resolvers: r}))
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		queueChanges, err := r.Subscription().InterceptQueueChanged(ctx)
		if err != nil {
			t.Fatal(err)
		}
		settingsChanges, err := r.Subscription().InterceptSettingsChanged(ctx)
		if err != nil {
			t.Fatal(err)
		}

		var settings struct {
			InterceptSettings struct {
				Queuing, Requests, Responses bool
				DropStatusCode               int
			}
		}
		if err := c.Post(`{ interceptSettings { queuing requests responses dropStatusCode } }`, &settings); err != nil {
			t.Fatal(err)
		}
		if got := settings.InterceptSettings; got.Queuing || !got.Requests || got.Responses || got.DropStatusCode != 502 {
			t.Fatalf("intercept settings = %+v", got)
		}

		var update struct{ UpdateInterceptSettings struct{ Queuing bool } }
		if err := c.Post(`mutation { updateInterceptSettings(input: { queuing: true }) { queuing } }`, &update); err != nil || !update.UpdateInterceptSettings.Queuing {
			t.Fatalf("update settings = %+v, %v", update, err)
		}
		if got := <-settingsChanges; !got.Queuing {
			t.Fatalf("settings event = %+v", got)
		}

		raw := []byte("POST / HTTP/1.1\r\nContent-Length: 1\r\n\r\nx")
		start := func(id int64, raw []byte) <-chan intercept.Decision {
			result := make(chan intercept.Decision, 1)
			go func() {
				decision, err := svc.HoldRequest(ctx, a, interceptMessage{id: id, raw: raw})
				if err != nil && ctx.Err() == nil {
					t.Error(err)
				}
				result <- decision
			}()
			synctest.Wait()

			return result
		}

		edited := start(100, raw)
		editID := (<-queueChanges).Item.ID
		var single struct{ ForwardInterceptItem, DropInterceptItem string }
		const edit = `mutation($id: ID!, $raw: Bytes!) { forwardInterceptItem(id: $id, raw: $raw) }`
		err = c.Post(edit, &single, client.Var("id", editID), client.Var("raw", ""))
		if err == nil || !strings.Contains(err.Error(), "BAD_INPUT") {
			t.Fatalf("explicit empty replacement = %v", err)
		}
		replacement := base64.StdEncoding.EncodeToString([]byte("POST / HTTP/1.1\r\nContent-Length: 1\r\n\r\nabc"))
		err = c.Post(edit, &single, client.Var("id", editID), client.Var("raw", replacement))
		if err != nil || single.ForwardInterceptItem != editID {
			t.Fatalf("forward with replacement = %+v, %v", single, err)
		}
		if got := <-edited; !got.Edited || string(got.Raw) != "POST / HTTP/1.1\r\nContent-Length: 3\r\n\r\nabc" {
			t.Fatalf("forward with replacement = %+v", got)
		}
		if removed := <-queueChanges; removed.Kind != generated.InterceptQueueChangeKindRemoved || removed.Item.ID != editID {
			t.Fatalf("edited item removal = %+v", removed)
		}

		for i, drop := range []bool{false, true} {
			result := start(int64(i+1), raw)
			added := <-queueChanges
			if added.Kind != generated.InterceptQueueChangeKindAdded || added.Item.Request.Host != a.Path() {
				t.Fatalf("added event = %+v", added)
			}

			var queue struct {
				InterceptQueue []struct {
					ID, Raw string
					Request struct{ ID, Host string }
				}
			}
			if err := c.Post(`{ interceptQueue(kind: REQUEST) { id raw request { id host } } }`, &queue); err != nil {
				t.Fatal(err)
			}
			if len(queue.InterceptQueue) != 1 || queue.InterceptQueue[0].ID != added.Item.ID ||
				queue.InterceptQueue[0].Request.ID != added.Item.Request.ID ||
				queue.InterceptQueue[0].Raw != base64.StdEncoding.EncodeToString(raw) {
				t.Fatalf("queue = %+v", queue.InterceptQueue)
			}

			var released struct{ ForwardAllInterceptItems, DropAllInterceptItems int }
			if drop {
				err = c.Post(`mutation { dropAllInterceptItems(kind: REQUEST) }`, &released)
			} else {
				err = c.Post(`mutation { forwardAllInterceptItems }`, &released)
			}
			if err != nil || released.ForwardAllInterceptItems+released.DropAllInterceptItems != 1 {
				t.Fatalf("release all = %+v, %v", released, err)
			}
			if decision := <-result; drop && decision.DropStatusCode != 502 || !drop && string(decision.Raw) != string(raw) {
				t.Fatalf("release decision = %+v", decision)
			}
			if removed := <-queueChanges; removed.Kind != generated.InterceptQueueChangeKindRemoved || removed.Item.ID != added.Item.ID {
				t.Fatalf("removed event = %+v", removed)
			}
			for _, operation := range []string{"forwardInterceptItem", "dropInterceptItem"} {
				err := c.Post("mutation { "+operation+"(id: \""+added.Item.ID+"\") }", &single)
				if err == nil || !strings.Contains(err.Error(), "NOT_FOUND") {
					t.Fatalf("released item %s = %v", operation, err)
				}
			}
		}

		pending := start(3, raw)
		<-queueChanges
		queuedBeforeSwitch := start(4, raw)
		count := history.calls.Load()
		db.Store(b)
		var queue struct {
			InterceptQueue []struct {
				ID      string
				Request struct{ Host string }
			}
		}
		if err := c.Post(`{ interceptQueue { id request { host } } }`, &queue); err != nil || len(queue.InterceptQueue) != 0 {
			t.Fatalf("queue after switch = %+v, %v", queue, err)
		}
		if history.calls.Load() != count {
			t.Fatal("old queue item was resolved against the new project")
		}
		if err := svc.ProjectChanged(ctx); err != nil {
			t.Fatal(err)
		}
		<-pending
		<-queuedBeforeSwitch
		if settings := <-settingsChanges; settings.Queuing {
			t.Fatalf("settings event after switch = %+v", settings)
		}
		synctest.Wait()
		select {
		case got := <-queueChanges:
			t.Fatalf("old project event after switch: %+v", got)
		default:
		}
	})
}
