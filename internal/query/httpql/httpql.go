// Package httpql defines the HTTPQL-compatible query language over HTTP
// history (docs/04-query-language.md §2) on top of [core].
package httpql

import "github.com/pocikode/ratatoskr/internal/query/core"

// Sources are the values accepted by source: (Search page).
var Sources = []string{"proxy", "intercept", "replay", "automate", "workflow", "plugin"}

// Language is the HTTPQL field table. req.color, req.note, req.edited,
// finding.* and NOT are Ratatoskr extensions.
var Language = &core.Language{
	Fields: map[string]core.Field{
		"req.method":       {Type: core.String},
		"req.host":         {Type: core.String},
		"req.port":         {Type: core.Int},
		"req.path":         {Type: core.String},
		"req.query":        {Type: core.String},
		"req.ext":          {Type: core.String},
		"req.raw":          {Type: core.Bytes},
		"req.body":         {Type: core.Bytes},
		"req.header":       {Type: core.String, Header: true},
		"req.len":          {Type: core.Int},
		"req.tls":          {Type: core.Bool},
		"req.created_at":   {Type: core.Datetime},
		"req.color":        {Type: core.String},
		"req.note":         {Type: core.String},
		"req.edited":       {Type: core.Bool},
		"resp.code":        {Type: core.Int},
		"resp.len":         {Type: core.Int},
		"resp.roundtrip":   {Type: core.Int},
		"resp.raw":         {Type: core.Bytes},
		"resp.body":        {Type: core.Bytes},
		"resp.header":      {Type: core.String, Header: true},
		"row.id":           {Type: core.Int},
		"finding.title":    {Type: core.String},
		"finding.reporter": {Type: core.String},
	},
	Sources: Sources,
	// "x" means (req.raw.cont:"x" OR resp.raw.cont:"x").
	Shorthand: func(lit string, sp core.Span) core.Node {
		return &core.Or{L: raw("req", lit, sp), R: raw("resp", lit, sp), Loc: sp}
	},
}

func raw(ns, lit string, sp core.Span) *core.Clause {
	return &core.Clause{Namespace: ns, Field: "raw", Op: core.Cont, Value: core.Value{Type: core.Bytes, Str: lit}, Loc: sp}
}

// Parse parses an HTTPQL query; see [core.Language.Parse].
func Parse(src string) (core.Node, error) { return Language.Parse(src) }
