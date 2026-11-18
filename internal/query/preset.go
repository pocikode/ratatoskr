package query

import (
	"fmt"
	"slices"

	"github.com/pocikode/ratatoskr/internal/query/core"
)

// MaxPresetDepth is how deeply preset: references may nest (docs/04 §4).
const MaxPresetDepth = 8

// PresetLookup resolves a preset name or alias. key identifies the preset
// (for example its id) so a name and its alias are recognised as one preset
// when detecting cycles; src is its query text.
type PresetLookup func(ref string) (key, src string, ok bool)

// ExpandPresets replaces every preset: node in n with the parsed query of
// that preset, recursively. A nil n yields nil. Errors are *[core.Error]
// spanning the top-level preset: node that caused them, because spans of
// nested presets point into other texts.
func ExpandPresets(lang *core.Language, n core.Node, lookup PresetLookup) (core.Node, error) {
	return expand(lang, n, lookup, nil, nil)
}

func expand(lang *core.Language, n core.Node, lookup PresetLookup, stack []string, top *core.Span) (core.Node, error) {
	switch n := n.(type) {
	case *core.Preset:
		sp := n.Loc
		if top != nil {
			sp = *top
		}

		key, src, ok := lookup(n.Name)
		if !ok {
			return nil, &core.Error{Span: sp, Msg: fmt.Sprintf("unknown preset %q", n.Name)}
		}

		if slices.Contains(stack, key) {
			return nil, &core.Error{Span: sp, Msg: fmt.Sprintf("preset %q refers to itself (cycle)", n.Name)}
		}

		if len(stack) >= MaxPresetDepth {
			return nil, &core.Error{Span: sp, Msg: fmt.Sprintf("presets nested deeper than %d levels", MaxPresetDepth)}
		}

		sub, err := lang.Parse(src)
		if err != nil {
			return nil, &core.Error{Span: sp, Msg: fmt.Sprintf("preset %q: %v", n.Name, err)}
		}

		if sub == nil {
			return nil, &core.Error{Span: sp, Msg: fmt.Sprintf("preset %q is empty", n.Name)}
		}

		return expand(lang, sub, lookup, append(stack, key), &sp)
	case *core.And:
		return binary(lang, n.L, n.R, lookup, stack, top, func(l, r core.Node) core.Node { return &core.And{L: l, R: r, Loc: n.Loc} })
	case *core.Or:
		return binary(lang, n.L, n.R, lookup, stack, top, func(l, r core.Node) core.Node { return &core.Or{L: l, R: r, Loc: n.Loc} })
	case *core.Not:
		x, err := expand(lang, n.X, lookup, stack, top)
		if err != nil {
			return nil, err
		}

		return &core.Not{X: x, Loc: n.Loc}, nil
	}

	return n, nil
}

func binary(lang *core.Language, l, r core.Node, lookup PresetLookup, stack []string, top *core.Span, mk func(l, r core.Node) core.Node) (core.Node, error) {
	el, err := expand(lang, l, lookup, stack, top)
	if err != nil {
		return nil, err
	}

	er, err := expand(lang, r, lookup, stack, top)
	if err != nil {
		return nil, err
	}

	return mk(el, er), nil
}
