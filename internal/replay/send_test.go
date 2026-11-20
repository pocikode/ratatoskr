package replay_test

import (
	"bufio"
	"context"
	"net"
	"strings"
	"sync"
	"testing"

	"github.com/pocikode/ratatoskr/internal/apperr"
	"github.com/pocikode/ratatoskr/internal/events"
	"github.com/pocikode/ratatoskr/internal/history"
	"github.com/pocikode/ratatoskr/internal/replay"
	"github.com/pocikode/ratatoskr/internal/store"
	"github.com/pocikode/ratatoskr/internal/testutil"
)

type fakeEnv map[string]string

func (f fakeEnv) Vars(context.Context) (map[string]string, error) { return f, nil }

type fakeRewriter struct {
	mu      sync.Mutex
	sources []string
}

func (f *fakeRewriter) Request(_ context.Context, source string, raw []byte) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sources = append(f.sources, source)
	return []byte(strings.Replace(string(raw), "\r\n\r\n", "\r\nX-Mr: 1\r\n\r\n", 1)), nil
}

type sendRig struct {
	svc     *replay.Service
	sender  *replay.Sender
	db      *store.DB
	capture *history.Capture
	events  <-chan events.Msg[history.Event]
}

func newSendRig(t *testing.T, deps replay.SendDeps) *sendRig {
	t.Helper()
	svc, db := newService(t)
	current := func() *store.DB { return db }
	bus := &events.Bus[history.Event]{}
	c := history.New(history.Deps{DB: current, Bus: bus})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	deps.Capture = c
	return &sendRig{svc: svc, sender: replay.NewSender(svc, deps), db: db, capture: c, events: bus.Subscribe(ctx)}
}

func (r *sendRig) session(t *testing.T, host string, port int) replay.Session {
	t.Helper()
	return create(t, r.svc, replay.CreateInput{Connection: &replay.Connection{Host: host, Port: port}, Raw: []byte("GET / HTTP/1.1\r\n\r\n")})
}

func (r *sendRig) send(t *testing.T, s replay.Session, raw string, opts *replay.OptionsInput) replay.Entry {
	t.Helper()
	e, err := r.sender.Send(t.Context(), s.ID, replay.SendInput{Raw: []byte(raw), Connection: s.Connection, Options: opts})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

type record struct {
	source, raw string
	code        *int
}

func (r *sendRig) record(t *testing.T, id int64) record {
	t.Helper()
	var out record
	err := r.db.Reader().QueryRowContext(t.Context(),
		`SELECT r.source, r.raw, s.code FROM requests r LEFT JOIN responses s ON s.request_id = r.id WHERE r.id = ?`, id).
		Scan(&out.source, &out.raw, &out.code)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func (r *sendRig) count(t *testing.T) int {
	t.Helper()
	var n int
	if err := r.db.Reader().QueryRowContext(t.Context(), `SELECT count(*) FROM requests`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// serve answers each request head with reply(target); an empty reply holds the
// connection until the test ends. accepted, when set, is signalled per request.
func serve(t *testing.T, reply func(target string) string, accepted chan<- struct{}) (string, int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	var wg sync.WaitGroup
	t.Cleanup(func() { _ = ln.Close(); wg.Wait() })
	wg.Go(func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			wg.Go(func() {
				defer conn.Close()
				stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
				defer stop()
				br := bufio.NewReader(conn)
				line, _ := br.ReadString('\n')
				for {
					l, err := br.ReadString('\n')
					if err != nil || l == "\r\n" {
						break
					}
				}
				if accepted != nil {
					accepted <- struct{}{}
				}
				parts := strings.Fields(line)
				if len(parts) < 2 {
					return
				}
				if out := reply(parts[1]); out != "" {
					_, _ = conn.Write([]byte(out))
					return
				}
				<-ctx.Done()
			})
		}
	})
	addr := ln.Addr().(*net.TCPAddr)
	return addr.IP.String(), addr.Port
}

func TestSendStoresDraftRecordAndEntry(t *testing.T) {
	t.Parallel()
	srv := testutil.NewRawServer(t)
	rig := newSendRig(t, replay.SendDeps{})
	s := rig.session(t, srv.Host, srv.Port)
	raw := "GET /a HTTP/1.1\r\nHost: x.test\r\n\r\n"

	e := rig.send(t, s, raw, nil)
	if e.Error != nil || e.RequestID == 0 {
		t.Fatalf("entry = %+v", e)
	}
	if got := rig.record(t, e.RequestID); got.source != "replay" || got.raw != raw || got.code == nil || *got.code != 200 {
		t.Fatalf("record = %+v", got)
	}
	if got, _ := rig.svc.Session(t.Context(), s.ID); string(got.Draft) != raw {
		t.Fatalf("draft = %q", got.Draft)
	}
	if page, _ := rig.svc.Entries(t.Context(), s.ID, 10, nil); page.Total != 1 || page.Items[0].ID != e.ID {
		t.Fatalf("entries = %+v", page)
	}
	for _, kind := range []history.Kind{history.Added, history.Updated} {
		if m := <-rig.events; m.Value.Kind != kind || m.Value.RequestID != e.RequestID {
			t.Fatalf("event = %+v, want %v", m, kind)
		}
	}
	if rig.sender.Sending(s.ID) {
		t.Fatal("still sending")
	}
}

func TestSendPreparesBytes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		env         replay.Env
		rewriter    bool
		opts        replay.OptionsInput
		raw, want   string
		wantSources []string
	}{
		{
			name: "interpolation, unknown verbatim",
			env:  fakeEnv{"p": "sel", "h": "host.test"},
			raw:  "GET /{{p}}/{{ x }} HTTP/1.1\r\nHost: {{ h }}\r\n\r\n",
			want: "GET /sel/{{ x }} HTTP/1.1\r\nHost: host.test\r\n\r\n",
		},
		{
			name: "match and replace on", rewriter: true, opts: replay.OptionsInput{ApplyMatchReplace: new(true)},
			raw: "GET / HTTP/1.1\r\n\r\n", want: "GET / HTTP/1.1\r\nX-Mr: 1\r\n\r\n", wantSources: []string{"replay"},
		},
		{
			name: "match and replace off", rewriter: true,
			raw: "GET / HTTP/1.1\r\n\r\n", want: "GET / HTTP/1.1\r\n\r\n",
		},
		{
			name: "match and replace on, nil rewriter", opts: replay.OptionsInput{ApplyMatchReplace: new(true)},
			raw: "GET / HTTP/1.1\r\n\r\n", want: "GET / HTTP/1.1\r\n\r\n",
		},
		{
			name: "content-length updated",
			raw:  "POST / HTTP/1.1\r\nContent-Length: 9\r\n\r\nab", want: "POST / HTTP/1.1\r\nContent-Length: 2\r\n\r\nab",
		},
		{
			name: "content-length kept when off", opts: replay.OptionsInput{UpdateContentLength: new(false)},
			raw: "GET / HTTP/1.1\r\nContent-Length: 0\r\n\r\nab", want: "GET / HTTP/1.1\r\nContent-Length: 0\r\n\r\nab",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := testutil.NewRawServer(t)
			rw := &fakeRewriter{}
			deps := replay.SendDeps{Env: tt.env}
			if tt.rewriter {
				deps.Rewriter = rw
			}
			rig := newSendRig(t, deps)
			e := rig.send(t, rig.session(t, srv.Host, srv.Port), tt.raw, &tt.opts)
			if e.Error != nil {
				t.Fatal(*e.Error)
			}
			if got := rig.record(t, e.RequestID); got.raw != tt.want {
				t.Fatalf("stored %q, want %q", got.raw, tt.want)
			}
			// The server frames by Content-Length, so bytes past it may arrive as a second message.
			if got := srv.Requests(); len(got) == 0 || !strings.HasPrefix(string(got[0]), strings.TrimSuffix(tt.want, "ab")) {
				t.Fatalf("server got %q", got)
			}
			if strings.Join(rw.sources, ",") != strings.Join(tt.wantSources, ",") {
				t.Fatalf("rewriter sources = %q", rw.sources)
			}
		})
	}
}

func TestSendConflictAndCancel(t *testing.T) {
	t.Parallel()
	accepted := make(chan struct{}, 1)
	host, port := serve(t, func(string) string { return "" }, accepted)
	rig := newSendRig(t, replay.SendDeps{})
	s := rig.session(t, host, port)

	if ok, err := rig.sender.Cancel(t.Context(), s.ID); ok || err != nil {
		t.Fatalf("idle cancel = %t, %v", ok, err)
	}
	if _, err := rig.sender.Cancel(t.Context(), s.ID+100); apperr.CodeOf(err) != apperr.NotFound {
		t.Fatalf("unknown cancel err = %v", err)
	}

	type result struct {
		e   replay.Entry
		err error
	}
	done := make(chan result, 1)
	go func() {
		e, err := rig.sender.Send(t.Context(), s.ID, replay.SendInput{Raw: []byte("GET /slow HTTP/1.1\r\n\r\n"), Connection: s.Connection})
		done <- result{e, err}
	}()
	<-accepted

	if !rig.sender.Sending(s.ID) {
		t.Fatal("not sending")
	}
	_, err := rig.sender.Send(t.Context(), s.ID, replay.SendInput{Raw: []byte("GET / HTTP/1.1\r\n\r\n"), Connection: s.Connection})
	if apperr.CodeOf(err) != apperr.Conflict {
		t.Fatalf("second send err = %v", err)
	}
	if ok, err := rig.sender.Cancel(t.Context(), s.ID); !ok || err != nil {
		t.Fatalf("cancel = %t, %v", ok, err)
	}
	got := <-done
	if got.err != nil || got.e.Error == nil || *got.e.Error != "cancelled" {
		t.Fatalf("cancelled send = %+v, %v", got.e, got.err)
	}
	if rec := rig.record(t, got.e.RequestID); rec.code != nil || rec.raw != "GET /slow HTTP/1.1\r\n\r\n" {
		t.Fatalf("record = %+v", rec)
	}
	if got, _ := rig.svc.Session(t.Context(), s.ID); string(got.Draft) != "GET /slow HTTP/1.1\r\n\r\n" {
		t.Fatalf("draft = %q (conflicting send must not save)", got.Draft)
	}
}

func TestSendRedirectHops(t *testing.T) {
	t.Parallel()
	host, port := serve(t, func(target string) string {
		if target == "/start" {
			return "HTTP/1.1 302 Found\r\nLocation: /final\r\nContent-Length: 0\r\n\r\n"
		}
		return "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok"
	}, nil)
	rig := newSendRig(t, replay.SendDeps{})
	e := rig.send(t, rig.session(t, host, port), "GET /start HTTP/1.1\r\nHost: a\r\n\r\n", &replay.OptionsInput{FollowRedirects: new(true)})

	if n := rig.count(t); n != 2 {
		t.Fatalf("records = %d, want 2", n)
	}
	if last := rig.record(t, e.RequestID); !strings.HasPrefix(last.raw, "GET /final ") || last.code == nil || *last.code != 200 {
		t.Fatalf("last hop = %+v", last)
	}
	if first := rig.record(t, e.RequestID-1); first.code == nil || *first.code != 302 {
		t.Fatalf("first hop = %+v", first)
	}
}

func TestSendDialFailure(t *testing.T) {
	t.Parallel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	rig := newSendRig(t, replay.SendDeps{})
	e := rig.send(t, rig.session(t, "127.0.0.1", port), "GET / HTTP/1.1\r\n\r\n", nil)

	if e.Error == nil || !strings.HasPrefix(*e.Error, "dial failed: ") {
		t.Fatalf("entry error = %v", e.Error)
	}
	if rec := rig.record(t, e.RequestID); rec.code != nil || rec.source != "replay" {
		t.Fatalf("record = %+v", rec)
	}
}

func TestSendRejects(t *testing.T) {
	t.Parallel()
	srv := testutil.NewRawServer(t)
	rig := newSendRig(t, replay.SendDeps{})
	s := rig.session(t, srv.Host, srv.Port)

	_, err := rig.sender.Send(t.Context(), s.ID, replay.SendInput{Raw: []byte("not http"), Connection: s.Connection})
	if apperr.CodeOf(err) != apperr.BadInput {
		t.Fatalf("unparsable err = %v", err)
	}
	if got, _ := rig.svc.Session(t.Context(), s.ID); string(got.Draft) != "not http" {
		t.Fatalf("draft = %q, want it saved", got.Draft)
	}
	_, err = rig.sender.Send(t.Context(), s.ID, replay.SendInput{Raw: []byte("x"), Connection: replay.Connection{Host: "h", Port: 0}})
	if apperr.CodeOf(err) != apperr.BadInput {
		t.Fatalf("bad connection err = %v", err)
	}
	if got, _ := rig.svc.Session(t.Context(), s.ID); string(got.Draft) != "not http" {
		t.Fatalf("draft = %q, invalid input must save nothing", got.Draft)
	}
	_, err = rig.sender.Send(t.Context(), s.ID+100, replay.SendInput{Raw: []byte("x"), Connection: s.Connection})
	if apperr.CodeOf(err) != apperr.NotFound {
		t.Fatalf("unknown session err = %v", err)
	}

	rig.capture.SetPaused(true)
	_, err = rig.sender.Send(t.Context(), s.ID, replay.SendInput{Raw: []byte("GET / HTTP/1.1\r\n\r\n"), Connection: s.Connection})
	if apperr.CodeOf(err) != apperr.Conflict || len(srv.Requests()) != 0 || rig.count(t) != 0 {
		t.Fatalf("paused capture err = %v, sent %d, records %d", err, len(srv.Requests()), rig.count(t))
	}
}
