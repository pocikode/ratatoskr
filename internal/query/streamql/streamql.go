// Package streamql defines the WebSocket query language
// (docs/04-query-language.md §6) on top of [core].
package streamql

import "github.com/pocikode/ratatoskr/internal/query/core"

// Language is the StreamQL field table. ws.direction, ws.format, ws.len,
// stream.port and stream.tls are Ratatoskr extensions.
var Language = &core.Language{
	Fields: map[string]core.Field{
		"ws.raw":       {Type: core.Bytes},
		"ws.direction": {Type: core.String},
		"ws.format":    {Type: core.String},
		"ws.len":       {Type: core.Int},
		"stream.host":  {Type: core.String},
		"stream.path":  {Type: core.String},
		"stream.port":  {Type: core.Int},
		"stream.tls":   {Type: core.Bool},
		"row.id":       {Type: core.Int},
	},
	// "x" means ws.raw.cont:"x".
	Shorthand: func(lit string, sp core.Span) core.Node {
		return &core.Clause{Namespace: "ws", Field: "raw", Op: core.Cont, Value: core.Value{Type: core.Bytes, Str: lit}, Loc: sp}
	},
}

// Parse parses a StreamQL query; see [core.Language.Parse].
func Parse(src string) (core.Node, error) { return Language.Parse(src) }
