---
id: T-091
title: Contract - scope presets and filter presets
milestone: M1
role: lead
status: todo
depends_on: [T-018]
owner: ""
branch: task/T-091-contract-scopes-filters
paths:
  - internal/api/graphql/schema/scope.graphqls
  - internal/api/graphql/schema/filter.graphqls
  - internal/api/graphql/resolvers/stubs_scope.go
  - internal/api/graphql/resolvers/stubs_filter.go
  - internal/api/graphql/generated/**
  - web/src/lib/api/generated/**
  - web/src/lib/api/mocks/**
  - gqlgen.yml
  - docs/tasks/T-091-contract-scopes-filters.md
spec:
  - docs/01-feature-spec.md#F-SCOPE-1, #F-FILTER-1
  - docs/03-architecture.md §6
  - docs/04-query-language.md §1
---

## Goal
GraphQL contract for scope presets and filter presets so T-019 can build against mocks and T-092 can implement it.
Follow-up of T-018, whose resolvers could not be written because these schema files did not exist.

## Scope
- In (`scope.graphqls`): `ScopePreset implements Node {id, name, allow: [String!]!, deny: [String!]!}`;
  `scopePresets: [ScopePreset!]!` (preset order), `scopePreset(id): ScopePreset`;
  `createScopePreset(name, allow, deny)`, `updateScopePreset(id, name, allow, deny)`, `duplicateScopePreset(id)`,
  `deleteScopePreset(id): ID!`, `reorderScopePresets(ids: [ID!]!): [ScopePreset!]!`.
- In (`filter.graphqls`): `FilterPreset implements Node {id, name, alias (nullable), query}`;
  `filterPresets: [FilterPreset!]!` (preset order), `filterPreset(id)`; `createFilterPreset(name, alias, query)`,
  `updateFilterPreset(id, name, alias, query)`, `deleteFilterPreset(id): ID!`, `reorderFilterPresets(ids): [FilterPreset!]!`,
  `exportFilterPresets: String!` (query; JSON `{"version":1,"presets":[…]}`), `importFilterPresets(json: String!): Int!` (number added).
- In: stub resolvers returning `ErrNotImplemented`, `make gen`, mock data + mock exchange handlers for both (reserved hosts only).
- Out: subscriptions (lists are small; pages refetch after their own mutations); a draft-scope match counter
  (T-019 counts saved presets with `requests(scopeId){totalCount}` and filter drafts with `requests(query){totalCount}`);
  WebSocket filter presets (`ws_filter_presets`, later milestone); an "append rule" mutation (UI uses `updateScopePreset`).

## Acceptance criteria
- [ ] Every type/field/argument documented, including error codes: `BAD_INPUT` (empty/over-long name, bad rule with the
      offending entry, bad alias), `CONFLICT` (duplicate scope name, duplicate filter name/alias), `BAD_QUERY` with span for
      invalid query / preset cycle, `NOT_FOUND` for unknown IDs. Rule syntax and limits stated as in `internal/scope`/`internal/filterpreset`.
- [ ] `reorder*` documents that `ids` must be exactly the current set (`BAD_INPUT` otherwise).
- [ ] `make gen` clean; stubs compile; `make check` green.
- [ ] Mock exchange serves both lists and mutations so T-019 can render with no backend (vitest for the mock).

## Notes from lead
- Same pattern as `project.graphqls` / `history.graphqls`: extend `Query`/`Mutation`, no new root files.
- Match existing service behaviour (T-018) rather than redesign: names unique per kind, filter alias `[a-z0-9][a-z0-9_-]*`,
  names resolved before aliases, ≤ 1000 presets, ≤ 1000 entries per scope list.

## Plan (implementer, ≤15 lines)

## Out-of-scope change needed (if any)

## Handoff (implementer)
- What changed:
- How to verify:
- Known limitations:
- Follow-ups (proposed tasks):
