---
id: T-090
title: Wire scope presets and preset expansion into history filtering
milestone: M1
role: backend
status: in_review
depends_on: [T-016, T-018]
owner: "sinarcloud/deepseek-v4.1-flash-1017"
branch: task/T-090-history-scope-presets
paths:
  - internal/scope/node*.go
  - internal/history/service.go
  - internal/history/service_test.go
  - internal/history/scope*_test.go
  - internal/app/app.go
  - internal/app/app_test.go
spec:
  - docs/01-feature-spec.md#F-SCOPE-1, #F-FILTER-1, #F-HIST-1
  - docs/04-query-language.md §1, §2
---

## Goal
`requests(scopeId:)`, `deleteRequests(scopeId:)`, the history subscriptions and any query containing `preset:` work in the
running app, instead of returning `NOT_FOUND` / `BAD_QUERY` (follow-up of T-016 review).

## Scope
- In: scope preset → `core.Node` conversion; `history.ServiceDeps.Scope` wired in `internal/app`; `preset:` expanded
  in `history.Service.filter` (and wherever else the service parses a user query, e.g. subscriptions) before compile.
- Out: scope/filter GraphQL schema and resolvers; scope reorder; sitemap (T-020 reuses the same `ScopeFunc`).

## Acceptance criteria
- [x] `scope` exposes a conversion from a preset/matcher to a `core.Node` following docs/01 F-SCOPE-1 (deny wins, empty
      allow = all, host glob `*`/`?` → LIKE with `%`/`_`/`\` escaped, port equality, case-sensitive path prefix).
- [x] Agreement test: for a table of rules × (host, port, path) rows, the SQL result over a seeded DB equals
      `Matcher.InScope` for every row (wildcards, ports, path prefix, `_`/`%` in hosts, deny over allow, empty allow).
- [x] `scopeId` that does not parse as an existing preset ID → `NOT_FOUND`; existing preset filters list, count, delete and
      `requestAdded` consistently (service test).
- [x] `preset:alias` / `preset:"Name"` in a history query expands (cycles/unknown → `BAD_QUERY` with span).
- [x] App wiring test: `internal/app` builds `history.Service` with `Scope` and preset expansion set.
- [x] Remove `TODO(T-090)` in `internal/history/service.go`. `make check` green.

## Notes from lead
- Build the node from the AST constructors in `internal/query/core` directly, not by formatting HTTPQL text (no quoting bugs).
- Path prefix must stay case-sensitive: `like` is ASCII case-insensitive, so use `regex` with `^` + `regexp.QuoteMeta(prefix)`.
- IPv6: `Rule.Match` normalises via `netip`; SQL cannot. Match the stored `req.host` form; document any gap with a `shortcut:` comment.
- Preset lookup: `filterpreset.Service.ExpandText` / `query.ExpandPresets` already exist; inject as a func in `ServiceDeps`
  so `internal/history` does not import `filterpreset`.
- Perf budget: scoped list on 100k rows < 300 ms (docs/04 §2).

## Plan (implementer, ≤15 lines)
1. `internal/scope/node.go`: `Matcher.Node()`/`Preset.Node()` build `(allow ORs) AND NOT (deny ORs)` from `core.Clause` —
   host `like` (`*`→`%`, `?`→`_`, escape `\ % _`), port `eq`, path `regex ^QuoteMeta(prefix)`; `Service.Node(ctx, id)`
   turns a GraphQL `scopeId` into that node, `NOT_FOUND` for bad/unknown IDs (matches `history.ScopeFunc`).
2. `internal/history/service.go`: new `ServiceDeps.ExpandText`; `filter` parses through it so `preset:` expands once for
   list, count, delete and both subscriptions; drop `TODO(T-090)`.
3. `internal/app/app.go`: build `scope.Service` + `filterpreset.Service` over `project.DB`; pass
   `Scope: scopes.Node`, `ExpandText: presets.ExpandText` to `history.NewService`.
4. Tests: rule-set × (host, port, path) agreement SQL vs `Matcher.InScope`; scopeId list/total/delete/requestAdded;
   `preset:` name/alias/nested/unknown/cycle; GraphQL wiring test in `internal/app`.

## Out-of-scope change needed (if any)

## Handoff (implementer)
- What changed:
  - `internal/scope/node.go` (new): `Matcher.Node`/`Preset.Node` compile the allow/deny rules into a `core.Node` — host
    `like` (`*`→`%`, `?`→`_`, `\ % _` escaped), `req.port.eq` when set, `req.path regex ^QuoteMeta` (case-sensitive).
    `Service.Node(ctx, id)` turns a GraphQL `scopeId` into that node, `NOT_FOUND` for malformed/unknown IDs.
  - `internal/history/service.go`: `ServiceDeps.ExpandText` injected; `filter` parses user queries through it, so `preset:`
    expands once for list/total, delete-by-query and both subscriptions. `TODO(T-090)` removed.
  - `internal/app/app.go`: builds `scope.Service` and `filterpreset.Service` over `project.DB`; passes
    `Scope: scope.New(...).Node` and `ExpandText: filterpreset.New(...).ExpandText`.
  - Tests: `internal/history/scope_test.go` — 12 rule sets × 13 (host, port, path) rows where the SQL result equals
    `Matcher.InScope`; scopeId list/total/delete/`requestAdded` consistency and NOT_FOUND; preset name/alias/nested/
    unknown/cycle. `internal/app/app_test.go` — GraphQL wiring test (`scopeId` filters, unknown/malformed scope →
    NOT_FOUND, `preset:alias` filters, unknown preset → BAD_QUERY).
- How to verify: `make check` (green, incl. web) and `go test ./internal/history -run 'TestScope|TestPresetExpansion' -v`.
- Known limitations: SQL compares the stored `req.host` text, while `Matcher.Match` normalises it via `netip` and matches
  `?` byte-wise (`shortcut:` comment in `node.go`), so IPv6 spellings and non-ASCII hosts can disagree. A scoped or
  `preset:` query costs two small DB reads (preset lookup, preset list).
- Follow-ups (proposed tasks): T-092 should hoist the two services into `App` fields for the resolvers (same paths).

## Review (reviewer)
Reviewer: claude-code/opus-5.5 · 2026-11-18 · branch `task/T-090-history-scope-presets` @ a608916

**Checks**
- `make check`: green on the second run (go fix, vet, golangci-lint, `go test -race`, web check, 183 web unit tests).
  The first run failed in a package this branch does not touch: `--- FAIL: TestUpstreamErrorIs502AndRecorded (0.73s) proxy_test.go:175: entries = []`.
  It passed `-race -count=10` when run alone, so it is a flaky test (it sometimes fails, sometimes passes). It is not caused by T-090; see the last finding.
  `make test-int` was not run because proxy, store and rawhttp are unchanged.
- Paths: all 7 changed files are inside `paths` (the task file is included). OK.
- Hard rules: no generated code or schema touched. No `net/http` in the traffic path. No new dependencies. The one `nolint`
  gives a reason and a task id. `TODO(T-090)` is removed, and no new untracked TODOs were added. OK.
- Lead notes: the node is built from `core` AST constructors, not text. The path uses `regex ^QuoteMeta` (case-sensitive).
  The SQL compiler emits `LIKE ? ESCAPE '\'`, so the `\ % _` escaping in `toLikePattern` is right. The Replacer works in one pass,
  so it never escapes its own output twice. The IPv6 and non-ASCII gap has a `shortcut:` comment. Preset expansion is injected as a func, so `history` does not import `filterpreset`. OK.
- Correctness: `requests.host/port/path` are `NOT NULL`, so `NOT (deny)` never hits SQL's NULL three-valued logic.
  Empty allow and empty deny gives a nil node, which matches everything (like `InScope`). A deny-only preset gives `NOT (...)` alone. OK.
- AC: all six are covered. `TestScopeNodeAgreesWithMatcher` checks 12 rule sets × 13 rows (SQL result vs `InScope`).
  `TestScopeFiltersListCountDeleteAndAdded` covers list, total, delete, `requestAdded` and NOT_FOUND. `TestPresetExpansion` covers name,
  alias, nested, unknown and cycle (BAD_QUERY with a span) plus the subscription. `TestWiresScopeAndPresetExpansion` goes through the real GraphQL handler.
- Load assumed: one pentester per instance, with 100k–1M rows (docs/00 §3).

**Findings**
- [ ] [minor] internal/history/service.go:104-108 — `filter` now calls `ExpandText` / `Scope` **before** the caller checks
  `s.db()` (list.go:164 then :169, mutate.go:108 then :113). Both `filterpreset.Service` and `scope.Service` call `s.db().View(...)`
  without a nil check. When no project DB is open, `project.DB()` returns nil. That happens after `project.Close()`, for example a request still running when the shutdown grace period ends.
  Example: `requests(query: "preset:x")` or `requests(scopeId: "1")` then panics on a nil `*store.DB` (`d.readers`).
  gqlgen recovers the panic, so the client gets INTERNAL instead of the old CONFLICT "no project is open". → At the top of `filter`, add
  `if _, err := s.db(); err != nil { return filter{}, err }` (3 lines, inside this task's paths). Add one test case:
  `NewService` with `DB: func() *store.DB { return nil }` plus a scope/preset dep must return `apperr.Conflict`.
- [ ] [minor] internal/history/scope_test.go — The lead note gives a budget: a scoped list on 100k rows must take < 300 ms. Nothing measures it.
  The scope node adds a Go `regexp()` UDF (user-defined function) call per row whose host matches, plus OR'd `LIKE`s. That is probably fine, but it is unproven.
  → Add `BenchmarkScopedList100k`: seed 100k rows and use a preset with 2 allow globs, a path prefix and 1 deny. Write the measured time in the Handoff.
  A follow-up task is acceptable if seeding 100k rows here is too slow for `make check` (benchmarks do not run there anyway).
- [ ] [nit] internal/scope/node.go:102-122 — `and` is a copy of `internal/history/service.go:147`. `core` is outside both lanes,
  so leave it for now. If a third copy appears (T-020 sitemap), move `And`/`Or` helpers into `internal/query/core`.
- [ ] [nit] (not T-090) internal/proxy/proxy_test.go:175 — `TestUpstreamErrorIs502AndRecorded` is flaky under `make check`
  (all packages run in parallel). → The lead should open a small task. The test probably reads entries before the async capture
  write is done, so it should wait for the bus event or poll with a deadline instead of reading once.

**Verdict: approve.** There are no blocker or major findings. The two minors are cheap: the nil-DB guard is 3 lines plus a test case, and the benchmark is optional and can be a follow-up.
Status stays `in_review` until the operator merges it and sets `done`.
