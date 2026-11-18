package history

import (
	"context"
	"database/sql"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/pocikode/ratatoskr/internal/apperr"
	"github.com/pocikode/ratatoskr/internal/query/core"
	"github.com/pocikode/ratatoskr/internal/query/httpql"
	"github.com/pocikode/ratatoskr/internal/query/streamql"
)

// Completion limits (docs/04-query-language.md §7).
const (
	MaxCompletions = 100
	maxValues      = 50
)

// Language names a query language for [Service.Complete].
type Language string

// Languages.
const (
	HTTPQL   Language = "HTTPQL"
	StreamQL Language = "STREAMQL"
)

// CompletionKind is what a completion inserts.
type CompletionKind string

// Completion kinds, the values of the GraphQL enum CompletionKind.
const (
	KindNamespace CompletionKind = "NAMESPACE"
	KindField     CompletionKind = "FIELD"
	KindOperator  CompletionKind = "OPERATOR"
	KindPreset    CompletionKind = "PRESET"
	KindValue     CompletionKind = "VALUE"
)

// Completion is one suggestion.
type Completion struct {
	Label       string
	InsertText  string
	Kind        CompletionKind
	Detail      string
	IsExtension bool
}

// Completions replace text[From:To] (byte offsets) with one of Items.
type Completions struct {
	From, To int
	Items    []Completion
}

// extensions are the fields beyond the HTTPQL-compatible language (§2, §6).
var extensions = map[string]bool{
	"req.color": true, "req.note": true, "req.edited": true,
	"ws.direction": true, "ws.format": true, "ws.len": true,
	"stream.port": true, "stream.tls": true,
}

// fixedValues are the values of fields with a closed set.
var fixedValues = map[string][]string{
	"source":       httpql.Sources,
	"req.color":    Colors,
	"ws.direction": {"client", "server"},
	"ws.format":    {"text", "binary"},
}

// distinct maps a field to the table column its project values come from.
var distinct = map[string]string{
	"req.host": "requests.host", "req.method": "requests.method", "req.ext": "requests.ext",
	"req.port": "requests.port", "resp.code": "responses.code",
}

// Complete suggests continuations of text at the byte offset cursor. It never
// fails on unparsable text: it completes what the partial input allows.
func (s *Service) Complete(ctx context.Context, lang Language, text string, cursor int) (Completions, error) {
	if cursor < 0 || cursor > len(text) {
		return Completions{}, apperr.New(apperr.BadInput, "cursor %d is outside the text (0..%d)", cursor, len(text))
	}

	l := httpql.Language
	if lang == StreamQL {
		l = streamql.Language
	}

	// The token under the cursor: word characters around it.
	from, to := cursor, cursor
	for from > 0 && isWord(text[from-1]) {
		from--
	}

	for to < len(text) && isWord(text[to]) {
		to++
	}

	word := text[from:cursor]
	res := Completions{From: from, To: to}

	switch {
	case inString(text[:cursor]):
		res.From, res.To = cursor, cursor
	case from > 0 && text[from-1] == ':':
		res = s.completeValue(ctx, l, text, from, cursor)
	case strings.Contains(word, "."):
		res.Items = completeFieldOrOp(l, word)
	default:
		res.Items = completeStart(l, word)
	}

	if len(res.Items) > MaxCompletions {
		res.Items = res.Items[:MaxCompletions]
	}

	return res, nil
}

func isWord(c byte) bool {
	return c == '_' || c == '.' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// inString reports whether the end of s is inside an open double-quoted string.
func inString(s string) bool {
	open := false

	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			if open {
				i++
			}
		case '"':
			open = !open
		}
	}

	return open
}

// completeStart suggests how a clause begins: namespaces plus source:/preset:.
func completeStart(l *core.Language, word string) []Completion {
	seen := map[string]bool{}

	var items []Completion

	for _, name := range slices.Sorted(maps.Keys(l.Fields)) {
		ns, _, _ := strings.Cut(name, ".")
		if seen[ns] || !hasPrefixFold(ns, word) {
			continue
		}

		seen[ns] = true

		items = append(items, Completion{Label: ns, InsertText: ns + ".", Kind: KindNamespace})
	}

	for _, kw := range []string{"source", "preset"} {
		if kw == "source" && l.Sources == nil || !hasPrefixFold(kw, word) {
			continue
		}

		kind := KindField
		if kw == "preset" {
			kind = KindPreset
		}

		items = append(items, Completion{Label: kw + ":", InsertText: kw + ":", Kind: kind})
	}

	return items
}

// completeFieldOrOp handles `ns.`, `ns.fi` and `ns.field.`/`ns.field.op`.
func completeFieldOrOp(l *core.Language, word string) []Completion {
	var items []Completion

	for _, name := range slices.Sorted(maps.Keys(l.Fields)) {
		f := l.Fields[name]

		switch {
		case hasPrefixFold(name, word) && len(name) > len(word) && !strings.HasPrefix(word, name+"."):
			items = append(items, Completion{
				Label: name, InsertText: name, Kind: KindField, Detail: f.Type.String(), IsExtension: extensions[name],
			})
		case strings.HasPrefix(word, name+"."):
			prefix := word[len(name)+1:]
			for _, op := range f.Type.Ops() {
				if hasPrefixFold(string(op), prefix) {
					items = append(items, Completion{
						Label: string(op), InsertText: name + "." + string(op), Kind: KindOperator,
						IsExtension: extensions[name],
					})
				}
			}
		}
	}

	return items
}

// completeValue suggests values after `field.op:`; from is where the value
// word starts. A leading quote is part of the replaced range.
func (s *Service) completeValue(ctx context.Context, l *core.Language, text string, from, cursor int) Completions {
	// The clause key sits before the colon.
	k := from - 1
	for k > 0 && (isWord(text[k-1])) {
		k--
	}

	key := text[k : from-1]

	// Drop the operator suffix: req.host.eq -> req.host.
	field := key
	if _, isField := l.Fields[key]; !isField {
		if i := strings.LastIndexByte(key, '.'); i > 0 {
			field = key[:i]
		}
	}

	prefix := text[from:cursor]
	res := Completions{From: from, To: from}

	for res.To < len(text) && isWord(text[res.To]) {
		res.To++
	}

	if res.To < cursor {
		res.To = cursor
	}

	f := l.Fields[field] // zero Type for source: and preset:

	switch {
	case fixedValues[field] != nil:
		for _, v := range fixedValues[field] {
			if hasPrefixFold(v, prefix) {
				res.Items = append(res.Items, valueItem(v, f.Type == core.String, "", extensions[field]))
			}
		}
	case f.Type == core.Bool:
		for _, v := range []string{"true", "false"} {
			if hasPrefixFold(v, prefix) {
				res.Items = append(res.Items, valueItem(v, false, "", extensions[field]))
			}
		}
	case field == "preset":
		res.Items = s.presets(ctx, prefix)
	case distinct[field] != "":
		res.Items = s.distinctValues(ctx, field, f.Type == core.String, prefix)
	}

	return res
}

// valueItem is a value completion; quote wraps it as a string literal. A
// source name or boolean is a bare word.
func valueItem(v string, quote bool, detail string, ext bool) Completion {
	ins := v
	if quote {
		ins = strconv.Quote(v)
	}

	return Completion{Label: v, InsertText: ins, Kind: KindValue, Detail: detail, IsExtension: ext}
}

func (s *Service) distinctValues(ctx context.Context, field string, text bool, prefix string) []Completion {
	db, err := s.db()
	if err != nil {
		return nil
	}

	table, col, _ := strings.Cut(distinct[field], ".")
	q := `SELECT ` + col + `, count(*) c FROM ` + table +
		` WHERE substr(lower(CAST(` + col + ` AS TEXT)), 1, length(?)) = lower(?) GROUP BY ` + col + ` ORDER BY c DESC, ` + col + ` LIMIT ?`

	rows, err := db.Reader().QueryContext(ctx, q, prefix, prefix, maxValues)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var items []Completion

	for rows.Next() {
		var (
			v sql.NullString
			c int
		)

		if rows.Scan(&v, &c) != nil {
			break
		}

		items = append(items, valueItem(v.String, text, strconv.Itoa(c), extensions[field]))
	}

	return items
}

func (s *Service) presets(ctx context.Context, prefix string) []Completion {
	db, err := s.db()
	if err != nil {
		return nil
	}

	rows, err := db.Reader().QueryContext(ctx, `SELECT name, coalesce(alias, '') FROM filter_presets ORDER BY sort_order, name LIMIT ?`, MaxCompletions)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var items []Completion

	for rows.Next() {
		var name, alias string
		if rows.Scan(&name, &alias) != nil {
			break
		}

		switch {
		case alias != "" && hasPrefixFold(alias, prefix):
			items = append(items, Completion{Label: alias, InsertText: alias, Kind: KindPreset, Detail: name})
		case hasPrefixFold(name, prefix):
			items = append(items, Completion{Label: name, InsertText: strconv.Quote(name), Kind: KindPreset})
		}
	}

	return items
}

func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}
