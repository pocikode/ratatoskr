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
