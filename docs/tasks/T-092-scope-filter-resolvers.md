---
id: T-092
title: Scope and filter preset resolvers, app wiring, scope reorder
milestone: M1
role: backend
status: in_review
depends_on: [T-091, T-090]  # serialized: both own internal/app
owner: "claude-sonnet-5-5"
branch: task/T-092-scope-filter-resolvers
paths:
  - internal/api/graphql/resolvers/scope*.go
  - internal/api/graphql/resolvers/filter*.go
  - internal/api/graphql/resolvers/stubs_scope.go
  - internal/api/graphql/resolvers/stubs_filter.go
  - internal/api/graphql/resolvers/resolver.go
  - internal/scope/service*.go
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
1. Migration 0002 adds `scope_presets.sort_order` (= id for existing rows); List orders by it, Create/Duplicate append last.
2. `scope.Service.Reorder` (BAD_INPUT on missing/extra/duplicate/unknown, transactional).
3. Thin resolvers `scope.go`/`filter.go` over `ScopeService`/`FilterService` interfaces; nil dep → NOT_IMPLEMENTED; stubs deleted.
4. Wire both services in `internal/app`. Tests through the real handler + migration test.

## Out-of-scope change needed (if any)

## Handoff (implementer)
- What changed: migration 0002, `scope.Service.Reorder`, scope/filter resolvers (stubs removed), `Deps.Scopes/Filters`, app wiring, tests (`presets_test.go`, scope service tests incl. migration order).
- How to verify: `make check`, `make test-int`.
- Known limitations: `app_test.go` unchanged (wiring covered by the resolver tests, not an app-level test). Unparsable IDs in reorder are BAD_INPUT, in get/update/delete NOT_FOUND. T-018 carry-overs (name-before-alias test, preset name in import BAD_QUERY message) not done; filterpreset is outside this task's paths.
- Follow-ups (proposed tasks):
