---
id: T-018
title: Scopes and filter presets backend with preset expansion
milestone: M1
role: backend
status: in_review
depends_on: [T-015]
owner: "claude-sonnet-5-5"
branch: task/T-018-scopes-presets-backend
paths:
  - internal/scope/**
  - internal/filterpreset/**
  - internal/query/preset*.go
  - internal/api/graphql/resolvers/scope*.go
  - internal/api/graphql/resolvers/filter*.go
spec:
  - docs/01-feature-spec.md#F-SCOPE-1, #F-FILTER-1
  - docs/04-query-language.md §2, §4
---

## Notes from lead
Schema for scopes/filters will be added by the lead to `scope.graphqls`/`filter.graphqls` before work starts (same pattern as T-013).

## Acceptance criteria
- [x] Host glob matching table tests (ports, wildcards, IDN, IPv6), deny over allow.
- [x] `preset:` expansion with cycle detection and depth limit 8.
- [x] Import/export JSON round-trip (service level).

## Plan (implementer, ≤15 lines)
- `internal/query/preset.go`: `ExpandPresets(lang, node, lookup)` AST substitution, cycle detection, depth 8.
- `internal/scope`: rule parser (`host[:port][/path]`, IPv6 `[::1]:80`, IDN→punycode), `Matcher` (deny wins, empty allow = all), `Service` CRUD/duplicate over `scope_presets`.
- `internal/filterpreset`: `Service` CRUD/reorder/import/export JSON over `filter_presets`, validates alias + query, rejects cycles, `Expand(query)` helper.
- Resolvers: schema files `scope.graphqls`/`filter.graphqls` do not exist yet (lead) -> resolvers not written (see out-of-scope).
- `scope_presets` has no sort_order column -> reorder of scopes needs a migration (store paths not mine).

## Out-of-scope change needed (if any)
- Lead: add `internal/api/graphql/schema/scope.graphqls` + `filter.graphqls` (absent on main) so resolvers `scope*.go`/`filter*.go` can be written.
- Lead: migration `scope_presets.sort_order` for F-SCOPE-1 reorder (internal/store/migrations).

## Handoff (implementer)
**Changed**
- `internal/query/preset.go`: `ExpandPresets` (AST substitution, cycle detection, depth 8, errors span the top-level `preset:` node).
- `internal/scope`: `ParseRule`/`Matcher` (host globs `*`/`?`, port, path prefix, IPv6 `[::1]:80`, IDN via built-in punycode, deny wins, empty allow = all) and `Service` (List/Get/Create/Update/Duplicate/Delete).
- `internal/filterpreset`: `Service` CRUD, Reorder, Export/Import JSON (`{"version":1,"presets":[...]}`, all-or-nothing, existing names skipped), `ExpandText` for callers; saves reject bad alias/query/cycles.

**Verify**: `go test ./internal/scope ./internal/filterpreset ./internal/query/...`; `make check` green.

**Limitations / not done**
- Resolvers `scope*.go`/`filter*.go` NOT written: `scope.graphqls`/`filter.graphqls` are not on main (see Out-of-scope). Services are ready to wire (`New(projectSvc.DB)`).
- Scope reorder unsupported: `scope_presets` has no `sort_order` (list is by id).
- Deleting/renaming a filter preset can leave dependents failing with BAD_QUERY on expansion; not blocked.
- Preset names are matched exactly (case-sensitive).

## Review (reviewer)
