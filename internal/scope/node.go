package scope

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/pocikode/ratatoskr/internal/apperr"
	"github.com/pocikode/ratatoskr/internal/query/core"
)

// Node resolves the preset with id (a GraphQL scopeId) to the query node it
// filters by. A non-numeric or unknown ID is [apperr.NotFound], like [Service.Get].
func (s *Service) Node(ctx context.Context, id string) (core.Node, error) {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return nil, apperr.New(apperr.NotFound, "scope %q not found", id)
	}

	p, err := s.Get(ctx, n)
	if err != nil {
		return nil, err
	}

	return p.Node()
}

// Node compiles the preset's entries into the query node it stands for.
func (p Preset) Node() (core.Node, error) {
	m, err := p.Matcher()
	if err != nil {
		return nil, err
	}

	return m.Node(), nil
}

// Node compiles the matcher into the query node it stands for (F-SCOPE-1):
// (allow₁ OR …) AND NOT (deny₁ OR …), either side left out when empty. A nil
// matcher matches everything, like [Matcher.InScope].
func (m *Matcher) Node() core.Node {
	if m == nil {
		return nil
	}

	node := rules(m.allow)
	if deny := rules(m.deny); deny != nil {
		node = and(node, &core.Not{X: deny})
	}

	return node
}

// rules ORs one clause per rule; nil when there are none.
func rules(rs []Rule) core.Node {
	var node core.Node
	for _, r := range rs {
		node = or(node, ruleNode(r))
	}

	return node
}

// ruleNode compiles one rule into "req.host like AND req.port.eq AND req.path
// regex". Paths use a regex because LIKE folds ASCII case.
//
// shortcut: only the rule side is normalised (ParseRule lowercases, punycodes
// and unwraps bracketed IPv6). Matcher.Match also normalises the request host
// through netip and matches "?" byte-wise, while SQL compares the stored text
// as is and matches "_" per rune, so IPv6 or non-ASCII request hosts can
// differ; normalise req.host on capture when that matters.
func ruleNode(r Rule) core.Node {
	var node core.Node = &core.Clause{
		Namespace: "req", Field: "host", Op: core.Like,
		Value: core.Value{Type: core.String, Str: toLikePattern(r.Host)},
	}

	if r.Port != 0 {
		node = and(node, &core.Clause{
			Namespace: "req", Field: "port", Op: core.Eq,
			Value: core.Value{Type: core.Int, Int: int64(r.Port)},
		})
	}

	if r.Path != "" {
		node = and(node, &core.Clause{
			Namespace: "req", Field: "path", Op: core.Regex,
			Value: core.Value{Type: core.String, Str: "^" + regexp.QuoteMeta(r.Path)},
		})
	}

	return node
}

// toLikePattern renders a rule host glob as a LIKE pattern. Replacer is one
// pass, so an escape it writes is never escaped again.
var toLikePattern = strings.NewReplacer(
	`\`, `\\`, `%`, `\%`, `_`, `\_`, `*`, `%`, `?`, `_`,
).Replace

func and(l, r core.Node) core.Node {
	switch {
	case l == nil:
		return r
	case r == nil:
		return l
	}

	return &core.And{L: l, R: r}
}

func or(l, r core.Node) core.Node {
	switch {
	case l == nil:
		return r
	case r == nil:
		return l
	}

	return &core.Or{L: l, R: r}
}
