package replay

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/pocikode/ratatoskr/internal/apperr"
	"github.com/pocikode/ratatoskr/internal/environment"
	"github.com/pocikode/ratatoskr/internal/history"
	"github.com/pocikode/ratatoskr/internal/rawhttp"
	"github.com/pocikode/ratatoskr/internal/replay/sender"
	"github.com/pocikode/ratatoskr/internal/store"
)

// Source is the history source of Replay records and the Match & Replace source.
const Source = "replay"

// storeTimeout bounds storing a finished send, which runs even after a cancel.
const storeTimeout = 10 * time.Second

// Env supplies interpolation variables: Global overlaid by the selected
// environment. *environment.Service implements it.
type Env interface {
	Vars(ctx context.Context) (map[string]string, error)
}

// Rewriter is the request side of Match & Replace (proxy.Rewriter's shape).
type Rewriter interface {
	Request(ctx context.Context, source string, raw []byte) ([]byte, error)
}

// Recorder stores history records. *history.Capture implements it.
type Recorder interface {
	Paused() bool
	NewPending(ctx context.Context, e history.Entry) int64
	Complete(ctx context.Context, id int64, resp *history.Response)
	Written(ctx context.Context, id int64) error
}

// SendDeps are the inputs to [NewSender]. Env and Rewriter may be nil (skipped).
type SendDeps struct {
	Env      Env
	Rewriter Rewriter
	Capture  Recorder
}

// Sender runs Replay sends, at most one per session (F-REP-2).
type Sender struct {
	svc  *Service
	deps SendDeps

	mu       sync.Mutex
	inFlight map[flightKey]*flight
}

// flightKey scopes a session ID to its project: IDs repeat across projects.
type flightKey struct {
	db *store.DB
	id int64
}

// flight is one send; cancellable only while it is on the network.
type flight struct {
	cancel      context.CancelCauseFunc
	cancellable bool
}

var errCancelled = errors.New("cancelled")

// NewSender returns a Sender storing sessions and entries through svc.
func NewSender(svc *Service, deps SendDeps) *Sender {
	return &Sender{svc: svc, deps: deps, inFlight: map[flightKey]*flight{}}
}

// SendInput is what a send saves on the session before sending.
type SendInput struct {
	Raw        []byte
	Connection Connection
	Options    *OptionsInput
}

// Sending reports whether a send of the session in the open project is in flight.
func (s *Sender) Sending(id int64) bool {
	db, err := s.svc.db()
	if err != nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.inFlight[flightKey{db, id}]
	return ok
}

// Cancel cancels the session's send while it is on the network and reports
// whether it did; the send then stores an entry with error "cancelled". During
// preparation and storing a send cannot be cancelled and Cancel reports false.
func (s *Sender) Cancel(ctx context.Context, id int64) (bool, error) {
	db, err := s.svc.db()
	if err != nil {
		return false, err
	}
	s.mu.Lock()
	f := s.inFlight[flightKey{db, id}]
	cancelled := f != nil && f.cancellable
	if cancelled {
		f.cancel(errCancelled)
	}
	s.mu.Unlock()
	if f != nil {
		return cancelled, nil
	}
	_, err = s.svc.Session(ctx, id)
	return false, err
}

// Send saves the input on the session, sends the prepared request and stores
// each hop as a history record plus one entry for the last hop. Network
// failures are entries with an error, not Go errors. Every step uses the
// project open when the send starts.
func (s *Sender) Send(ctx context.Context, id int64, in SendInput) (Entry, error) {
	db, err := s.svc.db()
	if err != nil {
		return Entry{}, err
	}
	pinned := New(func() *store.DB { return db })
	key := flightKey{db, id}
	netCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	f := &flight{cancel: cancel}
	s.mu.Lock()
	if s.inFlight[key] != nil {
		s.mu.Unlock()
		return Entry{}, apperr.New(apperr.Conflict, "a send of replay session %d is already in flight", id)
	}
	s.inFlight[key] = f
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.inFlight, key)
		s.mu.Unlock()
	}()

	if in.Raw == nil {
		in.Raw = []byte{} // nil would keep the old draft
	}
	sess, err := pinned.UpdateSession(ctx, id, UpdateInput{Draft: in.Raw, Connection: &in.Connection, Options: in.Options})
	if err != nil {
		return Entry{}, err
	}
	raw, err := s.prepare(ctx, sess)
	if err != nil {
		return Entry{}, err
	}
	if s.deps.Capture.Paused() {
		// shortcut: no record can be stored while capture is paused, so refuse before sending; revisit if Replay must bypass the pause.
		return Entry{}, apperr.New(apperr.Conflict, "history capture is paused; the send could not be stored")
	}

	s.setCancellable(f, true)
	start := time.Now()
	res, sendErr := sender.Send(netCtx, sender.Request{
		Host: sess.Connection.Host, Port: sess.Connection.Port, TLS: sess.Connection.TLS, SNI: deref(sess.Connection.SNI),
		Raw:     raw,
		Options: sender.Options{Timeout: time.Duration(sess.Options.TimeoutMs) * time.Millisecond, FollowRedirects: sess.Options.FollowRedirects},
	})
	s.setCancellable(f, false)
	if errors.Is(context.Cause(netCtx), errCancelled) {
		// Cancel reported true, possibly just after the response arrived: honour it.
		res.Hops[len(res.Hops)-1].Response = nil
		sendErr = &sender.Error{Kind: sender.KindCancelled, Err: errCancelled}
	}

	// A cancelled send still stores what happened.
	sctx, stop := context.WithTimeout(context.WithoutCancel(ctx), storeTimeout)
	defer stop()
	var last int64
	for _, hop := range res.Hops {
		e, resp := record(db, hop, start)
		start = start.Add(hop.Elapsed)
		if last = s.deps.Capture.NewPending(sctx, e); last == 0 {
			return Entry{}, errors.New("replay: history capture dropped the record (project closed or switched)")
		}
		s.deps.Capture.Complete(sctx, last, resp)
	}
	if err := s.deps.Capture.Written(sctx, last); err != nil {
		return Entry{}, fmt.Errorf("replay: store record: %w", err)
	}
	return pinned.AddEntry(sctx, id, last, failure(sendErr, sess.Options.TimeoutMs))
}

func (s *Sender) setCancellable(f *flight, on bool) {
	s.mu.Lock()
	f.cancellable = on
	s.mu.Unlock()
}

// prepare applies interpolation, Match & Replace and the Content-Length rule,
// then checks the result parses as an HTTP/1.x request head.
func (s *Sender) prepare(ctx context.Context, sess Session) ([]byte, error) {
	raw := sess.Draft
	if s.deps.Env != nil {
		vars, err := s.deps.Env.Vars(ctx)
		if err != nil {
			return nil, err
		}
		raw = environment.Interpolate(raw, vars)
	}
	if sess.Options.ApplyMatchReplace && s.deps.Rewriter != nil {
		var err error
		if raw, err = s.deps.Rewriter.Request(ctx, Source, raw); err != nil {
			return nil, err
		}
	}
	if sess.Options.UpdateContentLength {
		raw = sender.UpdateContentLength(raw)
	}
	if _, err := rawhttp.ReadRequest(bufio.NewReader(bytes.NewReader(raw)), rawhttp.Limits{}); err != nil {
		return nil, apperr.New(apperr.BadInput, "request does not parse: %v", err)
	}
	return raw, nil
}

// record builds the history entry and response of one hop starting at start.
// ProjectDB pins the record to the session's project across a switch.
func record(db *store.DB, hop sender.Hop, start time.Time) (history.Entry, *history.Response) {
	e := history.Entry{
		ProjectDB: db, Source: Source, Host: hop.Target.Host, Port: hop.Target.Port, TLS: hop.Target.TLS,
		CreatedAt: start, Req: history.Side{Raw: hop.Raw, BodyOffset: len(hop.Raw)},
	}
	input := bytes.NewReader(hop.Raw)
	br := bufio.NewReader(input)
	if req, err := rawhttp.ReadRequest(br, rawhttp.Limits{}); err == nil {
		e.Method, e.Target = req.Method(), req.Target()
		e.Req = side(&req.Message, hop.Raw, len(hop.Raw)-input.Len()-br.Buffered())
	}
	if hop.Response == nil {
		return e, nil
	}
	resp := &history.Response{
		Side:      side(&hop.Response.Message, hop.Response.Raw(), len(hop.Response.Head())),
		Code:      hop.Response.Status(),
		RoundTrip: hop.Elapsed,
		CreatedAt: start.Add(hop.Elapsed),
		Truncated: hop.Response.Truncated(),
	}
	return e, resp
}

func side(m *rawhttp.Message, raw []byte, bodyAt int) history.Side {
	s := history.Side{Raw: raw, BodyOffset: bodyAt}
	for _, h := range m.Headers() {
		s.Headers = append(s.Headers, history.Field{Name: h.Name, Value: h.Value})
	}
	return s
}

// failure is the entry's short error text; empty on success.
func failure(err error, timeoutMs int) string {
	if err == nil {
		return ""
	}
	e, ok := errors.AsType[*sender.Error](err)
	if !ok {
		return err.Error()
	}
	switch e.Kind {
	case sender.KindCancelled:
		return "cancelled"
	case sender.KindTimeout:
		return fmt.Sprintf("timeout after %d ms", timeoutMs)
	case sender.KindDial:
		return "dial failed: " + e.Err.Error()
	case sender.KindTLS:
		return "TLS handshake failed: " + e.Err.Error()
	default:
		return "protocol error: " + e.Err.Error()
	}
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
