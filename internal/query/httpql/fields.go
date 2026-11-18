package httpql

import (
	"time"

	"github.com/pocikode/ratatoskr/internal/query"
	"github.com/pocikode/ratatoskr/internal/query/core"
)

// entry is where one field lives on both back-ends (docs/04-query-language.md
// §4-§5): the SQL expression and the in-memory accessor must agree.
type entry struct {
	typ core.Type
	// col is the SQL expression over `requests r LEFT JOIN responses s`.
	col string
	// headers is the header table for header fields; col is then the name of
	// a value column inside it.
	headers string
	// eval reports whether m matches a clause on this field.
	eval func(c *core.Clause, m query.Message) bool
}

func textField(col string, get func(query.Message) string) entry {
	return entry{typ: core.String, col: col, eval: func(c *core.Clause, m query.Message) bool {
		return matchText(c.Op, c.Value.Str, get(m))
	}}
}

func bytesField(col string, get func(query.Message) []byte) entry {
	return entry{typ: core.Bytes, col: col, eval: func(c *core.Clause, m query.Message) bool {
		return matchBytes(c.Op, c.Value.Str, get(m))
	}}
}

func intField(col string, get func(query.Message) int64) entry {
	return entry{typ: core.Int, col: col, eval: func(c *core.Clause, m query.Message) bool {
		return matchInt(c.Op, c.Value.Int, get(m))
	}}
}

func boolField(col string, get func(query.Message) bool) entry {
	return entry{typ: core.Bool, col: col, eval: func(c *core.Clause, m query.Message) bool {
		return matchBool(c.Op, c.Value.Bool, get(m))
	}}
}

func timeField(col string, get func(query.Message) time.Time) entry {
	return entry{typ: core.Datetime, col: col, eval: func(c *core.Clause, m query.Message) bool {
		return matchInt(c.Op, c.Value.Time.UnixMilli(), get(m).UnixMilli())
	}}
}

// headerField is a header field. Without a subfield the operator matches a
// header's name or its value; a negated operator means "no header line
// matches", so both back-ends ask for NOT EXISTS of the positive form.
func headerField(table string, side func(query.Message) []query.Header) entry {
	return entry{typ: core.String, col: "value", headers: table, eval: func(c *core.Clause, m query.Message) bool {
		return matchHeaders(c, side(m))
	}}
}

// fields maps "namespace.field" to its entry. The set matches [Language] and
// is read-only after init.
var fields = map[string]entry{
	"req.method":     textField("r.method", query.Message.Method),
	"req.host":       textField("r.host", query.Message.Host),
	"req.port":       intField("r.port", func(m query.Message) int64 { return int64(m.Port()) }),
	"req.path":       textField("r.path", query.Message.Path),
	"req.query":      textField("r.query", query.Message.Query),
	"req.ext":        textField("r.ext", query.Message.Ext),
	"req.raw":        bytesField("r.raw", func(m query.Message) []byte { return m.Request().Raw }),
	"req.body":       bytesField("coalesce(substr(r.raw, r.body_offset + 1), x'')", reqBody),
	"req.header":     headerField("request_headers", reqHeaders),
	"req.len":        intField("r.len", func(m query.Message) int64 { return int64(len(m.Request().Raw)) }),
	"req.tls":        boolField("r.tls", query.Message.TLS),
	"req.created_at": timeField("r.created_at", query.Message.CreatedAt),
	"req.color":      textField("coalesce(r.color, '')", query.Message.Color),
	"req.note":       textField("coalesce(r.note, '')", query.Message.Note),
	"req.edited":     boolField("(r.edited_from IS NOT NULL)", query.Message.Edited),
	"resp.code":      intField("coalesce(s.code, 0)", respCode),
	"resp.len":       intField("coalesce(s.len, 0)", respLen),
	"resp.roundtrip": intField("coalesce(s.roundtrip_ms, 0)", respRoundTrip),
	"resp.raw":       bytesField("coalesce(s.raw, x'')", func(m query.Message) []byte { return respSide(m).Raw }),
	"resp.body":      bytesField("coalesce(substr(coalesce(s.raw, x''), coalesce(s.body_offset, 0) + 1), x'')", respBody),
	"resp.header":    headerField("response_headers", respHeaders),
	"row.id":         intField("r.id", query.Message.ID),
}

// negative maps a negated operator to the positive form it negates. For
// header fields a negated operator means "no header matches": EXISTS and
// NOT EXISTS of the positive form would otherwise both be useless.
func negative(op core.Op) (core.Op, bool) {
	switch op {
	case core.Ne:
		return core.Eq, true
	case core.Ncont:
		return core.Cont, true
	case core.Nlike:
		return core.Like, true
	case core.Nregex:
		return core.Regex, true
	}
	return op, false
}

// respSide returns the response of m, or a zero one when it was never
// captured: the SQL back-end reads the same exchange through coalesce().
func respSide(m query.Message) query.Side {
	if r := m.Response(); r != nil {
		return r.Side
	}
	return query.Side{}
}

func reqBody(m query.Message) []byte { return m.Request().Body() }

func respBody(m query.Message) []byte { return respSide(m).Body() }

func reqHeaders(m query.Message) []query.Header { return m.Request().Headers }

func respHeaders(m query.Message) []query.Header { return respSide(m).Headers }

func respCode(m query.Message) int64 {
	if r := m.Response(); r != nil {
		return int64(r.Code)
	}
	return 0
}

func respLen(m query.Message) int64 { return int64(len(respSide(m).Raw)) }

func respRoundTrip(m query.Message) int64 {
	if r := m.Response(); r != nil {
		return r.RoundTrip.Milliseconds()
	}
	return 0
}
