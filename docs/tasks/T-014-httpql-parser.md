---
id: T-014
title: HTTPQL-compatible lexer, parser, AST and conformance corpus
milestone: M1
role: backend
status: in_review
depends_on: [T-001]
owner: "claude-code (opus-5.5)"
branch: task/T-014-httpql-parser
paths:
  - internal/query/**
spec:
  - docs/04-query-language.md §1–§3, §6
---

## Goal
`internal/query/core` + `httpql` parse queries into a typed AST with precise error spans; `streamql` reuses the core with its own field table.

## Acceptance criteria
- [x] Grammar of docs/04 §1 fully implemented incl. comments, escapes, string shorthand, `preset:`/`source:`, `header["X"]`, `NOT`.
- [x] Type checking of operator vs field type at parse time with helpful messages ("`cont` is not valid for integer field resp.code; use eq/gt/lt…").
- [x] Precedence: `A AND B OR C` == `(A AND B) OR C` (test).
- [x] `FuzzParseHTTPQL` 60 s clean; `String()` of AST re-parses to an equal AST (property test).
- [x] Conformance corpus `testdata/conformance/*.txtar` with ≥ 80 cases (query, sample message, expected bool) — consumed by T-015.

## Plan (implementer, ≤15 lines)
1. `core/lexer.go`: byte scanner (idents, punctuation, strings with `\" \\ \n \r \t \xHH`, `//` + `/* */` comments, AND/OR/NOT keywords); parser pulls a raw "value" token after `:`.
2. `core/ast.go`: `Span`, `Node` (`Span()`, `String()`), `And`/`Or`/`Not`/`Clause`/`Preset`/`Source`, typed `Value`; canonical `String()` that re-parses to an equal AST.
3. `core/types.go`: field types, operators, §3 validity table, value coercion (quoted/bare ints, bools, datetime formats from §2), regex compiled at parse time.
4. `core/parser.go`: recursive descent per §1 (OR < AND < NOT), `Language` field table + shorthand hook, `*Error{Span, Msg}`; bounds 64 KiB input, depth 128.
5. `httpql`: §2 field table + extensions, `source:` values, `"x"` → `(req.raw.cont:"x" OR resp.raw.cont:"x")`. `streamql`: §6 table, `"x"` → `ws.raw.cont:"x"`.
6. Tests: table tests (grammar, precedence, error spans/messages), `FuzzParseHTTPQL`/`FuzzStreamQL` with round-trip property, conformance corpus `httpql/testdata/conformance/*.txtar` (≥ 80 cases; `request`/`response`/`meta`/`cases` per archive).
Defaults (flagged in Handoff): unquoted string values rejected; `finding.*` only in `FindingsLanguage`; `NOT NOT x` rejected (grammar `[ NOT ]`).

## Out-of-scope change needed (if any)

## Handoff (implementer)
**What changed**
- `internal/query/core`: lexer (comments, escapes, keywords), recursive-descent parser per docs/04 §1, AST (`And`/`Or`/`Not`/`Clause`/`Preset`/`Source`, typed `Value`), `Language` field table, `*core.Error{Span, Msg}` with byte spans (end exclusive), `core.Equal` (ignores spans), canonical `String()`. Bounds: 64 KiB input, paren depth 128.
- `internal/query/httpql`: §2 table + extensions (`req.color/note/edited`), `source:` values, `"x"` → `(req.raw.cont:"x" OR resp.raw.cont:"x")`. `FindingsLanguage`/`ParseFindings` add `finding.title/reporter` (Findings page only); plain `Parse` rejects `finding.*`.
- `internal/query/streamql`: §6 table, `"x"` → `ws.raw.cont:"x"`, no `source:`.
- Conformance corpus `internal/query/httpql/testdata/conformance/*.txtar`: 3 sample messages, 135 cases. Format and pinned semantics are in each archive header; loader `loadConformance` in `httpql/conformance_test.go` is ready for T-015.

**How to verify**
`go test -race ./internal/query/...`; `go test -run '^$' -fuzz '^FuzzParseHTTPQL$' -fuzztime 60s ./internal/query/httpql` (ran 60 s clean, also `FuzzStreamQL`); `make check` green.

**Limitations / decisions for the reviewer**
- Unquoted string values are rejected (grammar allows only `string_lit` for strings); ints/bools/RFC3339 datetimes may be bare.
- `NOT NOT x` is rejected (grammar `[ NOT ]`); `NOT (NOT x)` is accepted.
- Findings page callers must use `httpql.ParseFindings`.
- Datetimes are stored UTC; zone-less input is taken as UTC. RFC 7231 dates accept only `GMT` (no other zone abbreviations); ISO forms `…T12:00Z` and `+0200` offsets are accepted. Regex is compiled at parse time to report errors, not cached.
- Header `ne` on a missing header was left out of the corpus (SQL `EXISTS` vs evaluator semantics is T-015's call).

**Follow-ups**
- T-015: evaluate the corpus on both back-ends. T-018: preset expansion (`core.Preset`, depth 8). Completions (§7) not in scope.

## Review (reviewer)
Reviewed `main...HEAD` (eae92ea). All changed files are inside `internal/query/**` plus this task file. `make check` is green. No `make test-int` run: proxy/store/rawhttp are untouched.
I checked the grammar against docs/04 §1, the type table against §2/§3, StreamQL against §6, the error spans, the round-trip and depth limits, and the conformance corpus. I recomputed every `*.len.eq` value from the wire bytes and checked the boolean/LIKE/escape cases by hand. They all match.

- [x] [major] internal/query/core/types.go:123 — The `time.RFC1123` layout (`… 15:04:05 MST`) accepts any zone abbreviation. `time.Parse` gives unknown abbreviations a zero offset, so `req.created_at.lt:"Wed, 31 Jan 2024 12:00:00 EST"` parses to `2024-01-31T12:00:00Z` with no error. The real time is 17:00Z, so the filter silently returns wrong rows. `PDT`, `XYZ` and so on behave the same (I confirmed this with a probe). → Replace `time.RFC1123` with the literal-GMT layout `"Mon, 02 Jan 2006 15:04:05 GMT"` (RFC 7231 IMF-fixdate is always GMT), so other abbreviations fall through to the "is not a datetime" error. Add an error-table case for `"… EST"`.
- [x] [minor] internal/query/httpql/httpql.go:36-37 — docs/04 §2 says `finding.*` is "Findings page only", but the one `httpql.Language` accepts it everywhere. As a result, `finding.title.cont:"x"` on HTTP history passes parsing and leaves T-015's SQL compiler to fail at run time (it has no findings join). §3 wants those to be parse errors with a span. → Remove `finding.*` from `Language` and export a `FindingsLanguage` that copies the fields and adds `finding.title`/`finding.reporter`, with a `ParseFindings` wrapper. Otherwise, set `blocked` and ask the lead to confirm the "caller's job" decision recorded in the Handoff.
- [x] [nit] internal/query/httpql/fuzz_test.go:19-20 — When the error is not a `*core.Error`, `qe` is nil and the `Fatalf` arguments dereference `qe.Span`. The test panics instead of printing the intended message. → Split the check: `if !ok { t.Fatalf("error %v is %T, want *core.Error", err, err) }`, then do the span checks.
- [x] [nit] internal/query/core/types.go:115-125 — Common ISO 8601 forms `2024-01-31T12:00Z` (no seconds) and `2024-01-31T12:00:00+0200` (offset without a colon) are rejected. The error message is clear, so this is not a correctness issue. → If wanted, add `"2006-01-02T15:04Z07:00"` and `"2006-01-02T15:04:05.999999999Z0700"`.

AC coverage: all five ACs are covered by tests (`TestParseCanonical`, `TestParseErrors`, `TestPrecedence`, `FuzzParseHTTPQL`/`FuzzStreamQL` with round-trip, `TestConformanceCorpusParses` with 135 cases). Security and limits: input is capped at 64 KiB, paren depth at 128, RE2 only, and no I/O. I did not rerun the 60 s fuzz.

**Verdict: changes_requested** (1 major).
