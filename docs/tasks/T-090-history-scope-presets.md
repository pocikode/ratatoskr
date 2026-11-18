---
id: T-090
title: Wire scope presets and preset expansion into history filtering
milestone: M1
role: backend
status: todo
depends_on: [T-016, T-018]
owner: ""
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
- [ ] `scope` exposes a conversion from a preset/matcher to a `core.Node` following docs/01 F-SCOPE-1 (deny wins, empty
      allow = all, host glob `*`/`?` → LIKE with `%`/`_`/`\` escaped, port equality, case-sensitive path prefix).
- [ ] Agreement test: for a table of rules × (host, port, path) rows, the SQL result over a seeded DB equals
      `Matcher.InScope` for every row (wildcards, ports, path prefix, `_`/`%` in hosts, deny over allow, empty allow).
- [ ] `scopeId` that does not parse as an existing preset ID → `NOT_FOUND`; existing preset filters list, count, delete and
      `requestAdded` consistently (service test).
- [ ] `preset:alias` / `preset:"Name"` in a history query expands (cycles/unknown → `BAD_QUERY` with span).
- [ ] App wiring test: `internal/app` builds `history.Service` with `Scope` and preset expansion set.
- [ ] Remove `TODO(T-090)` in `internal/history/service.go`. `make check` green.

## Notes from lead
- Build the node from the AST constructors in `internal/query/core` directly, not by formatting HTTPQL text (no quoting bugs).
- Path prefix must stay case-sensitive: `like` is ASCII case-insensitive, so use `regex` with `^` + `regexp.QuoteMeta(prefix)`.
- IPv6: `Rule.Match` normalises via `netip`; SQL cannot. Match the stored `req.host` form; document any gap with a `shortcut:` comment.
- Preset lookup: `filterpreset.Service.ExpandText` / `query.ExpandPresets` already exist; inject as a func in `ServiceDeps`
  so `internal/history` does not import `filterpreset`.
- Perf budget: scoped list on 100k rows < 300 ms (docs/04 §2).

## Plan (implementer, ≤15 lines)

## Out-of-scope change needed (if any)

## Handoff (implementer)
- What changed:
- How to verify:
- Known limitations:
- Follow-ups (proposed tasks):
