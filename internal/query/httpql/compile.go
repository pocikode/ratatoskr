package httpql

import (
	"fmt"
	"strings"

	"github.com/pocikode/ratatoskr/internal/query/core"
)

// SQL is a compiled query: the WHERE fragment and the values it binds. Where
// is empty for an empty query.
type SQL struct {
	Where string
	Args  []any
}

// Compile compiles n into a parameterized WHERE fragment over
// `requests r LEFT JOIN responses s ON s.request_id = r.id`
// (docs/04-query-language.md §4). Values always travel as bind parameters.
//
// A nil n means no filter. n must not contain preset: nodes: they are expanded
// by the caller first (max depth 8).
func Compile(n core.Node) (SQL, error) {
	if n == nil {
		return SQL{}, nil
	}
	c := compiler{}
	if err := c.node(n); err != nil {
		return SQL{}, err
	}
	return SQL{Where: c.b.String(), Args: c.args}, nil
}

type compiler struct {
	b    strings.Builder
	args []any
}

func (c *compiler) node(n core.Node) error {
	switch n := n.(type) {
	case *core.And:
		return c.binary(n.L, " AND ", n.R)
	case *core.Or:
		return c.binary(n.L, " OR ", n.R)
	case *core.Not:
		c.b.WriteString("(NOT (")
		if err := c.node(n.X); err != nil {
			return err
		}
		c.b.WriteString("))")
	case *core.Clause:
		return c.clause(n)
	case *core.Source:
		c.b.WriteString("r.source = ?")
		c.args = append(c.args, n.Name)
	case *core.Preset:
		return &core.Error{Span: n.Loc, Msg: fmt.Sprintf("preset %q must be expanded before compiling", n.Name)}
	default:
		return fmt.Errorf("query: cannot compile %T", n)
	}
	return nil
}

func (c *compiler) binary(l core.Node, op string, r core.Node) error {
	c.b.WriteString("(")
	if err := c.node(l); err != nil {
		return err
	}
	c.b.WriteString(op)
	if err := c.node(r); err != nil {
		return err
	}
	c.b.WriteString(")")
	return nil
}

func (c *compiler) clause(n *core.Clause) error {
	f, ok := fields[n.Namespace+"."+n.Field]
	if !ok {
		return &core.Error{Span: n.Loc, Msg: fmt.Sprintf("unknown field %s.%s", n.Namespace, n.Field)}
	}
	if err := checkLike(n); err != nil {
		return err
	}
	if f.headers != "" {
		return c.header(n, f)
	}
	if n.Sub != core.SubNone {
		return &core.Error{Span: n.Loc, Msg: fmt.Sprintf("%s.%s has no header subfields", n.Namespace, n.Field)}
	}
	switch f.typ {
	case core.String:
		return c.text(&c.b, n, n.Op, f.col, n.Value.Str)
	case core.Bytes:
		return c.bytes(&c.b, n, n.Op, f.col, n.Value.Str)
	case core.Int:
		return c.num(&c.b, n, n.Op, f.col, n.Value.Int)
	case core.Datetime:
		return c.num(&c.b, n, n.Op, f.col, n.Value.Time.UnixMilli())
	case core.Bool:
		return c.num(&c.b, n, n.Op, f.col, boolInt(n.Value.Bool))
	}
	return unsupported(n)
}

func (c *compiler) text(b *strings.Builder, n *core.Clause, op core.Op, col, v string) error {
	switch op {
	case core.Eq:
		b.WriteString(col + " = ?")
	case core.Ne:
		b.WriteString(col + " <> ?")
	case core.Cont:
		b.WriteString("instr(lower(" + col + "), lower(?)) > 0")
	case core.Ncont:
		b.WriteString("instr(lower(" + col + "), lower(?)) = 0")
	case core.Like:
		b.WriteString(col + ` LIKE ? ESCAPE '\'`)
	case core.Nlike:
		b.WriteString(col + ` NOT LIKE ? ESCAPE '\'`)
	case core.Regex:
		b.WriteString("regexp(?, " + col + ")")
	case core.Nregex:
		b.WriteString("NOT regexp(?, " + col + ")")
	default:
		return unsupported(n)
	}
	c.args = append(c.args, v)
	return nil
}

// bytes compiles a byte field. LIKE never matches a blob operand, so it is
// cast to text first (bytes are reinterpreted, not decoded).
func (c *compiler) bytes(b *strings.Builder, n *core.Clause, op core.Op, col, v string) error {
	switch op {
	case core.Cont:
		b.WriteString("bcontains_fold(" + col + ", ?)")
	case core.Ncont:
		b.WriteString("NOT bcontains_fold(" + col + ", ?)")
	case core.Like:
		b.WriteString("cast(" + col + " AS TEXT) LIKE ? ESCAPE '\\'")
	case core.Nlike:
		b.WriteString("cast(" + col + " AS TEXT) NOT LIKE ? ESCAPE '\\'")
	case core.Regex:
		b.WriteString("regexp(?, " + col + ")")
	case core.Nregex:
		b.WriteString("NOT regexp(?, " + col + ")")
	default:
		return unsupported(n)
	}
	c.args = append(c.args, v)
	return nil
}

func (c *compiler) num(b *strings.Builder, n *core.Clause, op core.Op, col string, v int64) error {
	switch op {
	case core.Eq:
		b.WriteString(col + " = ?")
	case core.Ne:
		b.WriteString(col + " <> ?")
	case core.Gt:
		b.WriteString(col + " > ?")
	case core.Gte:
		b.WriteString(col + " >= ?")
	case core.Lt:
		b.WriteString(col + " < ?")
	case core.Lte:
		b.WriteString(col + " <= ?")
	default:
		return unsupported(n)
	}
	c.args = append(c.args, v)
	return nil
}

// header compiles a header field to an EXISTS sub-select over its header
// table: the history tables keep one row per header line, so "any header
// matches" is a sub-select rather than a comparison.
func (c *compiler) header(n *core.Clause, f entry) error {
	op, negated := negative(n.Op)
	var cond strings.Builder
	switch n.Sub {
	case core.SubNone:
		cond.WriteString("(")
		if err := c.text(&cond, n, op, "h.name", n.Value.Str); err != nil {
			return err
		}
		cond.WriteString(" OR ")
		if err := c.text(&cond, n, op, "h."+f.col, n.Value.Str); err != nil {
			return err
		}
		cond.WriteString(")")
	case core.SubName:
		if err := c.text(&cond, n, op, "h.name", n.Value.Str); err != nil {
			return err
		}
	case core.SubValue:
		if err := c.text(&cond, n, op, "h."+f.col, n.Value.Str); err != nil {
			return err
		}
	case core.SubKey:
		// Header names are matched ASCII case-insensitively (docs/04 §2).
		cond.WriteString("h.name_lc = ? AND ")
		c.args = append(c.args, asciiLower(n.Key))
		if err := c.text(&cond, n, op, "h."+f.col, n.Value.Str); err != nil {
			return err
		}
	default:
		return unsupported(n)
	}
	if negated {
		c.b.WriteString("NOT ")
	}
	c.b.WriteString("EXISTS (SELECT 1 FROM " + f.headers + " h WHERE h.request_id = r.id AND ")
	c.b.WriteString(cond.String())
	c.b.WriteString(")")
	return nil
}

func unsupported(n *core.Clause) error {
	return &core.Error{Span: n.Loc, Msg: fmt.Sprintf("operator %q is not valid for field %s.%s", n.Op, n.Namespace, n.Field)}
}

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
