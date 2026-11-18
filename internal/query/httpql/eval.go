package httpql

import (
	"fmt"
	"slices"
	"strconv"

	"github.com/pocikode/ratatoskr/internal/query"
	"github.com/pocikode/ratatoskr/internal/query/core"
)

// Eval reports whether m satisfies n (docs/04-query-language.md §5). n must be
// a tree returned by [Parse] or [FindingsLanguage]; a nil n matches everything.
// n must not contain preset: nodes: they are expanded by the caller first.
//
// Errors carry a span and mean the query cannot be evaluated at all (an
// unexpanded preset, a field this evaluator has no source for).
func Eval(n core.Node, m query.Message) (bool, error) {
	switch n := n.(type) {
	case nil:
		return true, nil
	case *core.And:
		ok, err := Eval(n.L, m)
		if err != nil || !ok {
			return false, err
		}
		return Eval(n.R, m)
	case *core.Or:
		ok, err := Eval(n.L, m)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
		return Eval(n.R, m)
	case *core.Not:
		ok, err := Eval(n.X, m)
		return !ok, err
	case *core.Clause:
		f, ok := fields[n.Namespace+"."+n.Field]
		if !ok {
			return false, &core.Error{Span: n.Loc, Msg: "unknown field " + n.Namespace + "." + n.Field}
		}
		if f.headers == "" && n.Sub != core.SubNone {
			return false, &core.Error{Span: n.Loc, Msg: n.Namespace + "." + n.Field + " has no header subfields"}
		}
		return f.eval(n, m), nil
	case *core.Source:
		return m.Source() == n.Name, nil
	case *core.Preset:
		return false, &core.Error{Span: n.Loc, Msg: "preset " + strconv.Quote(n.Name) + " must be expanded before evaluating"}
	}
	return false, fmt.Errorf("query: cannot evaluate %T", n)
}

func matchText(op core.Op, want, got string) bool {
	switch op {
	case core.Eq:
		return got == want
	case core.Ne:
		return got != want
	case core.Cont:
		return containsFold(got, want)
	case core.Ncont:
		return !containsFold(got, want)
	case core.Like:
		return likeMatch(want, []byte(got))
	case core.Nlike:
		return !likeMatch(want, []byte(got))
	case core.Regex:
		return regexpMatch(want, []byte(got))
	case core.Nregex:
		return !regexpMatch(want, []byte(got))
	}
	return false
}

func matchBytes(op core.Op, want string, got []byte) bool {
	switch op {
	case core.Cont:
		return containsFold(got, []byte(want))
	case core.Ncont:
		return !containsFold(got, []byte(want))
	case core.Like:
		return likeMatch(want, got)
	case core.Nlike:
		return !likeMatch(want, got)
	case core.Regex:
		return regexpMatch(want, got)
	case core.Nregex:
		return !regexpMatch(want, got)
	}
	return false
}

func matchInt(op core.Op, want, got int64) bool {
	switch op {
	case core.Eq:
		return got == want
	case core.Ne:
		return got != want
	case core.Gt:
		return got > want
	case core.Gte:
		return got >= want
	case core.Lt:
		return got < want
	case core.Lte:
		return got <= want
	}
	return false
}

func matchBool(op core.Op, want, got bool) bool {
	switch op {
	case core.Eq:
		return got == want
	case core.Ne:
		return got != want
	}
	return false
}

// matchHeaders matches a header field: true when some header line matches, or,
// for a negated operator, when none does (docs/04 §2: header matching).
func matchHeaders(c *core.Clause, hs []query.Header) bool {
	op, negated := negative(c.Op)
	want := c.Value.Str
	var match func(query.Header) bool
	switch c.Sub {
	case core.SubNone:
		match = func(h query.Header) bool {
			return matchText(op, want, h.Name) || matchText(op, want, h.Value)
		}
	case core.SubName:
		match = func(h query.Header) bool { return matchText(op, want, h.Name) }
	case core.SubValue:
		match = func(h query.Header) bool { return matchText(op, want, h.Value) }
	case core.SubKey:
		key := asciiLower(c.Key)
		match = func(h query.Header) bool {
			return asciiLower(h.Name) == key && matchText(op, want, h.Value)
		}
	default:
		return false
	}
	return slices.ContainsFunc(hs, match) != negated
}
