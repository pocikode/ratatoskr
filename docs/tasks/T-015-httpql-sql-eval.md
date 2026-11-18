---
id: T-015
title: HTTPQL SQL compiler, in-memory evaluator and benchmarks
milestone: M1
role: backend
status: changes_requested
depends_on: [T-014, T-005]
owner: "opencode (deepseek-v4.1-flash-1017)"
branch: task/T-015-httpql-sql-eval
paths:
  - internal/query/**
  - internal/store/functions*.go
spec:
  - docs/04-query-language.md §4, §5
---

## Goal
Compile ASTs to parameterized SQL over the project schema and evaluate them in memory; both pass the shared conformance corpus.

## Acceptance criteria
- [x] Conformance corpus passes for both back-ends (same results).
- [x] No value ever appears in SQL text (test asserts placeholders only).
- [x] `BenchmarkQuery100k`: `req.host.eq` and `resp.code.gte` < 300 ms; `req.raw.cont` < 2 s on CI hardware (document numbers).
- [x] Regex cache bounded (LRU 256).

## Plan (implementer, ≤15 lines)
1. `internal/query/message.go`: `query.Message` interface (accessors, aliased bytes) + `Side`/`Response` structs (docs/04 §5).
2. `httpql/fields.go`: one table per field: SQL expression over `requests r LEFT JOIN responses s`, header table, evaluator accessor. Single source for both back-ends.
3. `httpql/compile.go`: `Compile(ast) (SQL{Where, Args}, error)`; always `?` placeholders; `cont`→`instr(lower())`, blobs→`bcontains_fold`, `like`→`LIKE ? ESCAPE '\'`, regex→`regexp(?, col)`; header fields→`EXISTS`/`NOT EXISTS` sub-selects; absent response read as empty (coalesce).
4. `httpql/eval.go`: `Eval(ast, Message) (bool, error)`; ASCII-fold contains and SQLite-LIKE matcher (`like.go`); regex via bounded LRU 256 (`regexp.go`, golang-lru).
5. Both reject unexpanded `preset:`; negated header ops are `NOT EXISTS(match)`.
6. Tests: golden SQL txtar, placeholder-only assertion, corpus run on both back-ends, unit tables for like/fold/header/LRU, `BenchmarkCompile*`, `BenchmarkQuery100k`.

## Out-of-scope change needed (if any)
1. `internal/store/funcs.go:14-33` — the `regexp()` SQL-function cache is bounded at 256 but **clears wholesale**
   instead of evicting the least recently used pattern (docs/04 §4 says LRU). Fixing it means editing `funcs.go`,
   which my `paths` (`internal/store/functions*.go`) do not cover. Suggested: `clear(reCache)` → delete the oldest
   entry, or move to `hashicorp/golang-lru/v2` (already a dependency, used by `internal/ca`).
   The T-015 AC ("regex cache bounded, LRU 256") is met by the evaluator's cache (`httpql/regexp.go`, `lru.Cache`
   with a 256-entry test), and the SQL path's cache is bounded — only the eviction *policy* differs.
2. `docs/04-query-language.md` §5 — worth pinning the two semantics this task had to decide (see Handoff): an
   unanswered request reads as an empty response on both back-ends, and plain `req.header`/`resp.header` matches a
   header's name **or** value. `docs/` is outside my paths.

## Handoff (implementer)
**What changed** (all inside `internal/query/**`)
- `internal/query/message.go`: `query.Message` (docs/04 §5) with `Header`, `Side` (raw bytes aliased, body offset,
  header slice) and `Response` (side + code + round trip). One accessor per history field, so an intercept/workflow
  adapter over `rawhttp` costs no body copy.
- `internal/query/httpql/fields.go`: the only field table — SQL expression over `requests r LEFT JOIN responses s`,
  header table name, and the in-memory accessor, per field. Both back-ends read it, so a field or column cannot drift.
  `resp.*` columns are wrapped in `coalesce()`; `req.body`/`resp.body` are `coalesce(substr(...), x'')` because
  `substr(blob, n)` returns NULL for an empty blob (probed), which would turn `ncont`/`nregex` into NULL.
- `internal/query/httpql/compile.go`: `Compile(core.Node) (SQL{Where, Args}, error)`. Every value is a `?` bind
  parameter; `cont`/`ncont` → `instr(lower(col), lower(?))` (text) or `bcontains_fold` (bytes); `like`/`nlike` →
  `col LIKE ? ESCAPE '\'`, and for byte columns `cast(col AS TEXT) LIKE ...` because SQLite's LIKE never matches a
  BLOB operand (probed); `regex`/`nregex` → the registered `regexp(?, col)`; header fields → `EXISTS`/`NOT EXISTS`
  over `request_headers`/`response_headers` keyed by `r.id`; `header["X"]` compares `name_lc` with an ASCII-lowered
  key. `And`/`Or`/`Not` are always parenthesized.
- `internal/query/httpql/eval.go` + `match.go`: `Eval(core.Node, query.Message) (bool, error)` with the same
  operator table; ASCII-only fold (`containsFold`, a generic over `~string|~[]byte`, mirrors both `lower()` and
  `bcontains_fold`) and a `likeMatch` that follows SQLite: `%` any run, `_` one UTF-8 character, `\` escapes,
  ASCII case-insensitive, non-ASCII bytes compared exactly.
- `internal/query/httpql/regexp.go`: compiled-pattern `lru.Cache[string, *regexp.Regexp]` of 256 (`golang-lru/v2`,
  already a direct dependency); `TestRegexpCacheBounded` fills 512 patterns and asserts the bound and eviction.
- `internal/query/httpql/testdata/conformance/unicode-like.txtar`: 28 new corpus cases (non-ASCII fold, `_` over
  multi-byte characters, escaped `%`/`_`, bytes LIKE/regex). Total corpus 163 cases over 4 messages.
- `internal/query/httpql/testdata/sql.txtar` + `compile_test.go`: golden SQL text and bind values for 35 cases
  (`-update` rewrites it, same pattern as `internal/rawhttp`).

**Semantics pinned here** (both back-ends agree; corpus enforces it)
- An unanswered request reads as an empty response: code 0, len 0, empty raw/body, no headers. Implemented with
  `coalesce()` in SQL and `query.Message.Response() == nil` in the evaluator (`TestMissingResponse` runs both).
- Plain `req.header.cont:"x"` matches a header's **name or value**; `["X"]` matches the name ASCII
  case-insensitively and the operator applies to the value; `name`/`value` subfields keep byte-exact `eq`/`ne`
  and SQLite-LIKE rules. A negated header operator means "no header line matches" (`NOT EXISTS` of the positive
  form), so a missing header matches `ne`/`ncont`/`nlike`/`nregex`. Scalar fields keep plain `<>`/negation.
- `preset:` is not expanded here (T-018 owns AST substitution, depth 8): both back-ends return a `*core.Error`
  carrying the preset's span. `finding.*` is accepted by `ParseFindings` but has no source on either back-end, so it
  errors instead of silently returning nothing.
- `req.len`/`resp.len` are the raw byte lengths (`r.len`, evaluator `len(Raw)`); datetimes compare as Unix
  milliseconds (`r.created_at`, `Time.UnixMilli()`).

**Numbers** (`BenchmarkQuery100k`, Apple M4 / 10 cores, darwin/arm64, 100k rows seeded into a temp project DB;
`go test -run '^$' -bench 'BenchmarkQuery100k|BenchmarkCompile' -benchtime 5x ./internal/query/httpql`)
- `req.host.eq:"host-42.example.com"` — **0.17 ms** (100 rows matched; the `requests(host)` index is used).
- `resp.code.gte:500` — **22.5 ms** (16.6k rows matched; `coalesce(s.code, 0)` scans).
- `req.raw.cont:"needle-xyz"` — **64 ms** (1k rows matched; `bcontains_fold` over 100k bodies).
- `BenchmarkCompile`: simple clause ≈ 0.2 µs, four-clause query ≈ 1–3 µs.
All three budgets are met with wide margin (300 ms / 2 s). CI hardware is slower; the numbers are the local baseline.

**How to verify**
`go test -race -count=1 ./internal/query/...` (corpus runs both back-ends, 163 cases), the benchmark command above,
`go test ./internal/query/httpql -run TestSQLGolden -update` to regenerate goldens, `make check` (green: go fix
diff empty, vet, golangci-lint 0 issues, `go test -race ./...`, web check + 123 web tests).

**Limitations / decisions for the reviewer**
- `like`/`cont` worst case is O(len(pattern)·len(subject)), the same as SQLite's own `patternCompare`; the pattern is
  user-authored and capped at 64 KiB by the parser. No extra guard was added (a match against a 64 MiB body with a
  pathological pattern is slow on both back-ends equally).
- The evaluator's LIKE treats invalid UTF-8 bytes as one character each, which is what SQLite does on probed cases
  (`x'ff' LIKE '_'` → true).
- `req.query`/`req.ext`/`req.path` come from the stored columns in SQL and from `Message.Path/Query/Ext` in memory;
  T-016's adapter must derive them from `history.Entry.Target` exactly as storage does (`path.Ext` semantics, no
  leading `?`).
- Regexes are compiled by the parser (validation) and again in the evaluator cache; an AST built by hand with an
  invalid pattern simply never matches (both back-ends report the same `false`).
- Nothing here emits SQL for the WebSocket tables (`ws_messages`): docs/04 §6 SQL is not part of §4/§5.

**Follow-ups**
- T-016: write the `history.Entry` → `query.Message` adapter and use `Eval` for subscription filtering,
  `Compile` for paginated/delete-by-query SQL (`Where` + `Args` append to its own conditions).
- T-018: preset expansion before `Compile`/`Eval` (depth 8, cycle-checked).
- StreamQL SQL compiler over `ws_messages`/`ws_streams` (docs/04 §6) — a separate task.
- The `funcs.go` LRU nit under "Out-of-scope change needed" above.

## Review (reviewer)
Reviewer: Claude Code (reviewer agent), 2026-11-18. Branch `task/T-015-httpql-sql-eval` @ `fed40e6`.

**Checks**
- Diff scope: every changed file is under `internal/query/**` or this task file. OK.
- `make check`: green (go fix diff empty, vet, golangci-lint 0 issues, `go test -race ./...`, web check + 123 tests).
- `make test-int`: not run (no proxy/store/rawhttp change).
- Probed SQLite (modernc, the repo's version) for findings 1 and 2. Results are quoted below.

**AC coverage**
- Same results on both back-ends: corpus runs on both (`TestConformanceBackends`, 163 cases). But see finding 1: binary bodies give different results, and the corpus has no body with a NUL byte.
- Values bound, never in the SQL text: `TestSQLValuesAreBound` + placeholder count in golden/corpus tests. OK.
- Benchmarks: numbers are written down and well inside budget. OK.
- Regex LRU 256: `TestRegexpCacheBounded`. OK. (The store-side cache eviction was correctly raised as out of scope.)

**Findings**
- [ ] [major] internal/query/httpql/compile.go:137-139 + match.go:39 — `req.body`/`req.raw`/`resp.*` `like`/`nlike` give different results on the two back-ends for any body with a NUL byte. SQLite's LIKE reads the value as a C string and stops at the first `\x00`. Probe: `cast(x'62696e00206e6565646c65' AS TEXT) LIKE '%needle%'` → 0 (same bytes without the NUL → 1). `likeMatch` scans all bytes and returns true. Gzip, images and protobuf bodies have NUL bytes all the time. So a live subscription (Eval) shows a row that a reload of the Search page (SQL) hides, and the other way round for `nlike`. → Simplest in-lane fix: make `likeMatch` follow SQLite. At the top, cut `s` and `pattern` at their first NUL (`if i := bytes.IndexByte(s, 0); i >= 0 { s = s[:i] }`, and `strings.IndexByte` for the pattern). Add a corpus case with a NUL in the body (`like` + `nlike`, a needle after the NUL) so both back-ends are pinned. If the product wants LIKE to see the whole binary body, that is a spec question (a `blike` SQL function in `internal/store`): set `blocked` and ask the lead instead of guessing.
- [ ] [minor] internal/query/httpql/compile.go:111-114,137-139 — a `like` pattern longer than 50,000 bytes (SQLite's `SQLITE_MAX_LIKE_PATTERN_LENGTH`) makes the SQL query fail: `LIKE or GLOB pattern too complex`. The evaluator accepts it, and the parser allows 64 KiB. → In `Compile` (and `Eval`, so both agree), reject `like`/`nlike` values over 50,000 bytes with a `*core.Error` on the clause span. Add one test.
- [ ] [minor] internal/query/httpql/conformance_backends_test.go:148 — `insertSample` returns before the header loop when there is no response. So an unanswered sample never gets its `request_headers` rows. `TestMissingResponse` gives its sample a `Host` header but never queries `req.header`, so this does not show today. The first such query will fail on the SQL side only. → Insert request headers before the `respRaw == nil` early return. Add `req.header["host"].eq:"example.com"` → true to `TestMissingResponse`.
- [ ] [minor] internal/query/httpql/match.go:11-122 — `containsFold`, `lowerASCII`, `upperASCII` and `boolInt` copy `internal/store/funcs.go`. Conformance needs both copies to stay the same, and nothing enforces that. → Not fixable inside this task's paths (`functions*.go` does not match `funcs.go`). Record it as a follow-up: one shared package used by both `store` and `httpql`.
- [ ] [nit] internal/query/httpql/eval.go:37-39 — `Not` returns `(true, err)` when the child fails. A caller that logs the error and still uses the bool lets the message through. → `if err != nil { return false, err }`.
- [ ] [nit] internal/query/httpql/eval.go:143 — `asciiLower(h.Name)` allocates for every header with a capital letter (almost all of them), per message, per clause. The evaluator runs on the intercept/subscription hot path. → Compare with a small non-allocating ASCII equal-fold (`len(a)==len(b)` + byte loop with `lowerASCII`).
- [ ] [nit] docs/tasks/T-015-httpql-sql-eval.md:106-107 (Handoff) — "an invalid pattern simply never matches (both back-ends report the same `false`)" is wrong for SQL: `store.compileCached` returns an error, so the query fails. → Fix the sentence (parser-built ASTs cannot hit this).

**Verdict: changes_requested.** Fix finding 1 (major). Findings 2 and 3 are small and should go in the same pass.
