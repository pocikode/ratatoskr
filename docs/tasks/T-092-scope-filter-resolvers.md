---
id: T-092
title: Scope and filter preset resolvers, app wiring, scope reorder
milestone: M1
role: backend
status: todo
depends_on: [T-091]
owner: ""
branch: task/T-092-scope-filter-resolvers
paths:
  - internal/api/graphql/resolvers/scope*.go
  - internal/api/graphql/resolvers/filter*.go
  - internal/api/graphql/resolvers/stubs_scope.go
  - internal/api/graphql/resolvers/stubs_filter.go
  - internal/api/graphql/resolvers/resolver.go
  - internal/scope/**
  - internal/store/migrations/project/0002_scope_sort_order.sql
  - internal/app/app.go
  - internal/app/app_test.go
spec:
  - docs/01-feature-spec.md#F-SCOPE-1, #F-FILTER-1
  - docs/03-architecture.md §6
---

## Goal
The T-091 contract works end to end: resolvers over `scope.Service` and `filterpreset.Service`, both built in
`internal/app`, and scope presets can be reordered (F-SCOPE-1).

## Scope
- In: `Deps.Scopes` / `Deps.Filters` interfaces in `resolver.go` (nil → `NOT_IMPLEMENTED`, like `History`); delete the
  stubs; migration adding `scope_presets.sort_order` (backfilled from `id`) and `scope.Service.Reorder`, `List` by sort order,
  new presets appended last.
- Out: wiring scopes/presets into history filtering (T-090); UI (T-019).

## Acceptance criteria
- [ ] Resolver tests through the real handler for every query/mutation in T-091, including one error case per error code.
- [ ] Migration test: existing rows keep their order (by id) after `0002`; `Reorder` with a missing/extra/duplicate id → `BAD_INPUT`.
- [ ] Export → delete all → import round-trip through GraphQL returns the same presets in the same order.
- [ ] `make check` green; `make test-int` green (migration).

## Notes from lead
- Services already exist (T-018); keep resolvers thin. IDs are decimal `int64` strings, non-numeric → `NOT_FOUND` (as history).
- Carry T-018's open review items here if cheap: name-before-alias test, preset name in import BAD_QUERY message.

## Plan (implementer, ≤15 lines)

## Out-of-scope change needed (if any)

## Handoff (implementer)
- What changed:
- How to verify:
- Known limitations:
- Follow-ups (proposed tasks):
