package matchreplace

import (
	"errors"
	"fmt"
	"path"
	"time"

	"github.com/pocikode/ratatoskr/internal/apperr"
	"github.com/pocikode/ratatoskr/internal/query"
	"github.com/pocikode/ratatoskr/internal/query/core"
	"github.com/pocikode/ratatoskr/internal/query/httpql"
)

// condFields are the HTTPQL fields the message bytes carry (F-MR-1, Condition
// fields); every other field and source: is rejected, so no value is guessed.
var condFields = map[string]bool{
	"req.method": true, "req.host": true, "req.path": true, "req.query": true, "req.ext": true,
	"req.raw": true, "req.body": true, "req.header": true, "req.len": true,
	"resp.code": true, "resp.len": true, "resp.raw": true, "resp.body": true, "resp.header": true,
}

// checkFields returns a BAD_QUERY error for the first clause of an expanded
// condition that reads something the message bytes do not carry.
func checkFields(n core.Node) error {
	switch n := n.(type) {
	case *core.And:
		if err := checkFields(n.L); err != nil {
			return err
		}
		return checkFields(n.R)
	case *core.Or:
		if err := checkFields(n.L); err != nil {
			return err
		}
		return checkFields(n.R)
	case *core.Not:
		return checkFields(n.X)
	case *core.Clause:
		if f := n.Namespace + "." + n.Field; !condFields[f] {
			return notAvailable(n.Loc, f)
		}
	case *core.Source:
		return notAvailable(n.Loc, "source:")
	}
	return nil
}

func notAvailable(sp core.Span, field string) error {
	return apperr.Query(apperr.Span{Start: sp.Start, End: sp.End}, "%s is not available in Match & Replace conditions", field)
}

// eval evaluates a checked condition on v. A clause about a side v does not
// have is false. Errors are BAD_QUERY with span.
func eval(n core.Node, v view) (bool, error) {
	switch n := n.(type) {
	case *core.And:
		ok, err := eval(n.L, v)
		if err != nil || !ok {
			return false, err
		}
		return eval(n.R, v)
	case *core.Or:
		ok, err := eval(n.L, v)
		if err != nil || ok {
			return ok, err
		}
		return eval(n.R, v)
	case *core.Not:
		ok, err := eval(n.X, v)
		return err == nil && !ok, err
	case *core.Clause:
		if n.Namespace == "req" && v.req == nil || n.Namespace == "resp" && v.resp == nil {
			return false, nil
		}
	}
	ok, err := httpql.Eval(n, v)
	if ce, isCore := errors.AsType[*core.Error](err); isCore {
		return false, apperr.Query(apperr.Span{Start: ce.Span.Start, End: ce.Span.End}, "%s", ce.Msg)
	}
	if err != nil {
		return false, fmt.Errorf("matchreplace: condition: %w", err)
	}
	return ok, nil
}

// view is the query view of a message (docs/04 §5). Only the allowed fields
// are ever read (checkFields); the others return zero values.
type view struct{ req, resp *msg }

var _ query.Message = view{}

func (v view) ID() int64            { return 0 }
func (v view) Source() string       { return "" }
func (v view) Port() int            { return 0 }
func (v view) TLS() bool            { return false }
func (v view) CreatedAt() time.Time { return time.Time{} }
func (v view) Color() string        { return "" }
func (v view) Note() string         { return "" }
func (v view) Edited() bool         { return false }
func (v view) Host() string         { return host(v.req.req) }
func (v view) Method() string       { return v.req.req.Method() }
func (v view) Path() string         { return splitTarget(v.req.req.Target()).path }
func (v view) Query() string        { return splitTarget(v.req.req.Target()).query }
func (v view) Ext() string          { return path.Ext(v.Path()) }

func (v view) Request() query.Side {
	if v.req == nil {
		return query.Side{}
	}
	return side(v.req)
}

func (v view) Response() *query.Response {
	if v.resp == nil {
		return nil
	}
	return &query.Response{Side: side(v.resp), Code: v.resp.res.Status()}
}

func side(x *msg) query.Side {
	s := query.Side{Raw: x.whole(), BodyOffset: len(x.whole()) - len(x.body)}
	for _, h := range x.m.Headers() {
		s.Headers = append(s.Headers, query.Header{Name: h.Name, Value: h.Value})
	}
	return s
}
