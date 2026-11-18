---
id: T-014
title: HTTPQL-compatible lexer, parser, AST and conformance corpus
milestone: M1
role: backend
status: in_progress
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
- [ ] Grammar of docs/04 §1 fully implemented incl. comments, escapes, string shorthand, `preset:`/`source:`, `header["X"]`, `NOT`.
- [ ] Type checking of operator vs field type at parse time with helpful messages ("`cont` is not valid for integer field resp.code; use eq/gt/lt…").
- [ ] Precedence: `A AND B OR C` == `(A AND B) OR C` (test).
- [ ] `FuzzParseHTTPQL` 60 s clean; `String()` of AST re-parses to an equal AST (property test).
- [ ] Conformance corpus `testdata/conformance/*.txtar` with ≥ 80 cases (query, sample message, expected bool) — consumed by T-015.

## Plan (implementer, ≤15 lines)
1. `core/lexer.go`: byte scanner (idents, punctuation, strings with `\" \\ \n \r \t \xHH`, `//` + `/* */` comments, AND/OR/NOT keywords); parser pulls a raw "value" token after `:`.
2. `core/ast.go`: `Span`, `Node` (`Span()`, `String()`), `And`/`Or`/`Not`/`Clause`/`Preset`/`Source`, typed `Value`; canonical `String()` that re-parses to an equal AST.
3. `core/types.go`: field types, operators, §3 validity table, value coercion (quoted/bare ints, bools, datetime formats from §2), regex compiled at parse time.
4. `core/parser.go`: recursive descent per §1 (OR < AND < NOT), `Language` field table + shorthand hook, `*Error{Span, Msg}`; bounds 64 KiB input, depth 128.
5. `httpql`: §2 field table + extensions, `source:` values, `"x"` → `(req.raw.cont:"x" OR resp.raw.cont:"x")`. `streamql`: §6 table, `"x"` → `ws.raw.cont:"x"`.
6. Tests: table tests (grammar, precedence, error spans/messages), `FuzzParseHTTPQL`/`FuzzStreamQL` with round-trip property, conformance corpus `httpql/testdata/conformance/*.txtar` (≥ 80 cases; `request`/`response`/`meta`/`cases` per archive).
Defaults (flagged in Handoff): unquoted string values rejected; `finding.*` accepted everywhere; `NOT NOT x` rejected (grammar `[ NOT ]`).

## Out-of-scope change needed (if any)

## Handoff (implementer)

## Review (reviewer)
