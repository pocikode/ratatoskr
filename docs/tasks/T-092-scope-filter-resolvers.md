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
- Review round 2: test file renamed to scope_filter_test.go, round-trip compares name/alias/query, delete returns normalized ID.
- Follow-ups (proposed tasks):

## Review (reviewer)
Reviewer: claude-opus-5-5 · 2026-11-18 · branch `task/T-092-scope-filter-resolvers` @ 4f123bf

**Checks**
- `make check`: green (exit 0): go fix/vet/golangci-lint, `go test -race ./...`, svelte-check, eslint, prettier, vitest 29 files / 187 tests.
- `make test-int`: green (exit 0). This was required because the store migration changed.
- Paths: 10 of 11 changed files are inside `paths`. `internal/api/graphql/resolvers/presets_test.go` is not (see finding 1).
- Hard rules: no generated code or schema touched by hand, no Caido text, test data only uses `*.test` hosts. Spec F-SCOPE-1 now
  supports "reorder".
- Correctness: `scope.Reorder` runs in a single write transaction. A missing, extra, duplicate or unknown ID returns BAD_INPUT and
  rolls back. The resolver test checks that a failed reorder leaves the order unchanged. Create and Duplicate append at
  `max(sort_order)+1`, and List orders by `sort_order, id`. Migration 0002 sets `sort_order = id` for existing rows, and the
  migration test checks that order with non-sequential IDs.
- AC: every query/mutation is tested through the real gqlgen handler. Error codes covered: BAD_INPUT, CONFLICT, NOT_FOUND,
  BAD_QUERY, NOT_IMPLEMENTED. Migration test and export → delete → import round-trip are present.

- Verdict: changes_requested
- Findings:
  - [x] [blocker] internal/api/graphql/resolvers/presets_test.go:1 — this file matches none of the task's `paths`
    (`scope*.go`, `filter*.go`, …). That breaks hard rule 1. → Rename it to `scope_filter_test.go`, which matches `scope*.go`,
    or split it into `scope_test.go` and `filter_test.go`. No code change is needed.
  - [x] [minor] internal/api/graphql/resolvers/presets_test.go:210 — the round-trip check only compares `Name`. The AC says
    "the same presets", so an import that dropped `alias` or changed `query` would still pass. → Compare all fields, e.g.
    `slices.EqualFunc(before.FilterPresets, after.FilterPresets, func(x, y fp) bool { return x.Name == y.Name && x.Query == y.Query && deref(x.Alias) == deref(y.Alias) })`.
    You could also just compare the two `exportFilterPresets` strings.
  - [x] [nit] internal/api/graphql/resolvers/scope.go:166, internal/api/graphql/resolvers/filter.go:138 — the delete mutations
    return the raw input ID. For `deleteScopePreset(id: "007")`, the response is `"007"` but the deleted preset's ID is `"7"`, so
    a client cache keyed on ID would not evict it. → Return `strconv.FormatInt(n, 10)`.
  - [ ] [nit] internal/scope/service.go:266 — `scope.Reorder` and `filterpreset.Reorder` check input in different ways and return
    different error messages for the same mistake. Both are correct. Aligning them is a possible follow-up outside this task's
    paths, so no action is needed here.

### Round 2
Reviewer: claude-opus-5-5 · 2026-11-18 · branch `task/T-092-scope-filter-resolvers` @ ad40bd8

**Checks**
- `make check`: green (exit 0). `make test-int`: green (exit 0).
- Paths: all 11 changed files are now inside `paths` (the test file is now `scope_filter_test.go`).
- Round 1 fixes confirmed: the round-trip test compares name, alias and query in order. Both delete mutations return the
  normalized ID (`strconv.FormatInt(n, 10)`).
- Callers: `scope_presets` is only used in `internal/scope/service.go`, and the embedded migration loader
  (`internal/store/migrate.go`, `migrations/project/*.sql`) picks up `0002` automatically. No other caller breaks.
- Rollback: the "unknown" reorder case (`{idA, idB, "99"}`) updates two rows and then fails. The test that follows confirms
  the order did not change, so the `Write` transaction rolls back.

- Verdict: approve
- Findings:
  - [ ] [nit] docs/tasks/T-092-scope-filter-resolvers.md:36-39 — the acceptance criteria boxes are still unticked, but each one
    is covered (see the round 1 AC note). → Tick them when you mark the task done.
  - [ ] [nit] internal/scope/service_test.go:975 — the migration test runs the raw `.sql` files from disk, not the embedded
    migrator, so it checks the SQL but not how the migrator orders and applies the files. The migrator already has its own tests in
    `internal/store`, so no action is needed.
