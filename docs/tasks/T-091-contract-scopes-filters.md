---
id: T-091
title: Contract - scope presets and filter presets
milestone: M1
role: lead
status: done
depends_on: [T-018]
owner: "claude-code (opus-5.5)"
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
- [x] Every type/field/argument documented, including error codes: `BAD_INPUT` (empty/over-long name, bad rule with the
      offending entry, bad alias), `CONFLICT` (duplicate scope name, duplicate filter name/alias), `BAD_QUERY` with span for
      invalid query / preset cycle, `NOT_FOUND` for unknown IDs. Rule syntax and limits stated as in `internal/scope`/`internal/filterpreset`.
- [x] `reorder*` documents that `ids` must be exactly the current set (`BAD_INPUT` otherwise).
- [x] `make gen` clean; stubs compile; `make check` green.
- [x] Mock exchange serves both lists and mutations so T-019 can render with no backend (vitest for the mock).

## Notes from lead
- Same pattern as `project.graphqls` / `history.graphqls`: extend `Query`/`Mutation`, no new root files.
- Match existing service behaviour (T-018) rather than redesign: names unique per kind, filter alias `[a-z0-9][a-z0-9_-]*`,
  names resolved before aliases, ≤ 1000 presets, ≤ 1000 entries per scope list.

## Plan (implementer, ≤15 lines)
1. `scope.graphqls`: `ScopePreset` + queries/mutations; docs state rule syntax `host[:port][/path]`, limits and error codes
   exactly as `internal/scope` (name 1-200 chars trimmed, ≤ 1000 entries per list, `CONFLICT` on duplicate name,
   duplicate naming "<name> (copy N)"). Reorder documented; its service side lands in T-092.
2. `filter.graphqls`: `FilterPreset` + CRUD, reorder, export/import as `internal/filterpreset` (alias rule, name-before-alias
   resolution, cycles → `BAD_QUERY` with span, import all-or-nothing, existing names skipped, ≤ 1000 presets).
3. `stubs_scope.go` / `stubs_filter.go` return `apperr.ErrNotImplemented`; `make gen`.
4. `web/src/lib/api/mocks/presets.ts`: in-memory scope + filter preset backend with the same validation/error codes;
   merge its root into the existing mock exchange; vitest covering list, create, conflict, reorder, export/import.
5. `make check`, Handoff.

## Out-of-scope change needed (if any)

## Handoff (implementer)
- What changed: `scope.graphqls` (`ScopePreset`, `scopePresets`/`scopePreset`, create/update/duplicate/delete/reorder) and
  `filter.graphqls` (`FilterPreset`, `filterPresets`/`filterPreset`/`exportFilterPresets`, create/update/delete/reorder/import),
  every field documented with rule syntax, limits and error codes taken from `internal/scope` / `internal/filterpreset`.
  `stubs_scope.go` / `stubs_filter.go` return `ErrNotImplemented`; `make gen` output committed. `mocks/presets.ts` is an
  in-memory backend for both (seeded with reserved hosts) merged into `mockHistoryExchange`; `presets.test.ts` covers it.
- How to verify: `make gen && git diff --exit-code` (clean), `make check`, `cd web && bunx --bun vitest run src/lib/api/mocks`.
- Known limitations: mock rule validation only checks host presence + port range (`shortcut:` comment); mock BAD_QUERY
  only covers `preset:` references (no HTTPQL parse). History mock still ignores `scopeId`, so T-019's per-preset
  `requests(scopeId){totalCount}` shows the unscoped total against mocks.
- Follow-ups (proposed tasks): T-092 must add `scope_presets.sort_order` + `scope.Service.Reorder` (already in its scope);
  `createScopePreset` defaults `allow`/`deny` to `[]`, which T-092's resolver gets as empty slices.

## Review (reviewer)
Reviewer: deepseek/v4.1-flash-1017 · 2026-11-18 · branch `task/T-091-contract-scopes-filters` @ 4307b9c

**Checks**
- `make check`: green — `go fix -diff` empty, `go vet`, `golangci-lint`, `go test -race ./...` (every package ok),
  `svelte-check` 0 errors/0 warnings, eslint, prettier, vitest 29 files / 187 tests. `make test-int` not run:
  proxy/store/rawhttp unchanged.
- `make gen` leaves `git status` clean, so the executor, `generated/models.go` and
  `web/src/lib/api/generated/schema.graphql` are exactly what gqlgen + codegen produce from the new SDL. Nothing was
  hand-edited (hard rules 4/5); `gqlgen.yml` needed no change.
- Paths: all 11 changed files are inside `paths` (2 `schema/*.graphqls`, the 2 stub files, `generated/**`, web
  `generated/**`, `mocks/**`, this task file).
- Hard rules: no Caido names/assets/text; mock seeds use reserved `example.com` / `example.org` / `*.test` hosts only;
  no lint suppressions; no untracked `TODO(T-###)` — the two `shortcut:` comments carry a reason and follow the existing
  `history.ts` precedent.
- Contract vs code (AC1): every documented limit and error checked against `internal/scope/{service,match}.go`,
  `internal/filterpreset/service.go` and `internal/query/preset.go`. Matches: name 1-200 runes after trimming, ≤ 1000
  entries per rule list, `host[:port][/path]` with the exact `scope entry "…": bad port "…"` wording, ≤ 1000 presets,
  `[a-z0-9][a-z0-9_-]*` alias, name-before-alias resolution, cycle + depth-8 rejection, `(copy)`/`(copy N)` with the
  150-rune cut, export `{"version":1,"presets":[…]}` with `alias` omitted (`omitzero`), import all-or-nothing with
  existing-name skips, `BAD_INPUT` for reorder ID sets that are not exactly the current set. Exceptions: findings 1 and 7.
- Spec: F-SCOPE-1, F-FILTER-1, docs/03 §6 (stubs return `apperr.ErrNotImplemented`, IDs are opaque strings, `BAD_QUERY`
  carries `extensions.span`) and docs/04 §1 (`preset:` clause, alias grammar). The "Out" list of the task is respected:
  no subscriptions (the "refetch after mutations" reason is stated in both file headers), no draft-scope counter, no
  append-rule mutation, no WS filter presets.
- AC: 1 met except the missing `BAD_QUERY` cause (finding 1); 2 met (both `reorder*` document the exact-set rule and
  `BAD_INPUT`); 3 verified; 4 met (`presets.test.ts` drives lists and mutations through the merged SDL, not stubs).
- Security/concurrency/perf: nothing to flag — the diff contains no SQL, file, network or goroutine code, and the mocks
  only run in dev/tests.

**Findings**
- [ ] [minor] internal/api/graphql/schema/filter.graphqls:49-51 — The `BAD_QUERY` cause list is incomplete: a query
  also fails when it fans out past 1000 preset substitutions (`internal/query/preset.go:12,52-54`, message "presets
  expand more than 1000 times"), reachable with a preset that repeats nine references seven levels deep. The import list
  (lines 84-85) omits that cause *and* the depth limit, although `Import` expands the new presets. → Append "or expands
  preset references more than 1000 times" to both lists and "or nests deeper than 8 levels" to the import list, so AC1
  holds for every reachable `BAD_QUERY`.
- [ ] [minor] web/src/lib/api/mocks/presets.ts:28-31,129 — `String(a.name)` / `String(a.query)` turn a missing JSON field
  into the literal `undefined`: `importFilterPresets({"version":1,"presets":[{"query":"a"}]})` returns 1 and creates a
  preset named `undefined`, and `[{"name":"Q"}]` stores `query: "undefined"` (reproduced against the mock with
  `bun -e`). `filterpreset.Import` runs `clean()` first (`internal/filterpreset/service.go:363`), so the real backend
  answers `BAD_INPUT`. → Check `typeof … === 'string'` before trimming in `checkName` and for `query`, so T-019's import
  UI surfaces the error instead of garbage rows.
- [ ] [minor] web/src/lib/api/mocks/presets.ts:81,85 — Nesting limit is off by one. `checkRefs` seeds `path` with the
  preset's own id, so it throws once a chain holds 8 presets: importing nine chained presets (`c0→…→c8`, i.e. 8
  `preset:` refs) answers `BAD_QUERY` "presets nested deeper than 8 levels", while `filterpreset.check` expands the
  saved preset's own query as the top level and accepts 8 references, rejecting the 9th (`internal/query/preset.go:51`;
  cf. `TestExpandPresets/depth_8_ok`). The mock therefore rejects a query the backend stores. → Use
  `if (path.length > 8)` (cycle detection keeps working) or start at `walk(p, [], null)` with a separate visited set.
- [ ] [minor] web/src/lib/api/mocks/presets.ts:229-231 — Import validates only `name` before skipping an entry whose name
  already exists, but `filterpreset.Import` runs `clean()` — name, alias *and* query — before the skip
  (`internal/filterpreset/service.go:363-367`), so an entry with an existing name plus a bad alias or empty query is
  `BAD_INPUT` on the backend and silently skipped here. → Run the full `filterFrom`-style validation first, then skip on
  the name.
- [ ] [minor] web/src/lib/api/mocks/presets.test.ts:11-146 — Coverage gaps in the mock (the plan asked only for
  list/create/conflict/reorder/export/import, so this is beyond plan): no case for the `scopePreset`/`filterPreset`
  single lookups, successful `updateScopePreset`/`updateFilterPreset`, `deleteFilterPreset`, or "all or nothing" (a file
  that fails validation must leave the list untouched). Since the mock is T-019's only backend, add those while T-092 is
  in flight.
- [ ] [minor] web/src/lib/api/mocks/history.ts:370 — The history mock still ignores `scopeId`, so T-019's per-preset
  "matches N requests" counter (T-019 AC, docs/02 §3.3) renders the unscoped total. The handoff notes it, but no task
  tracks it and both mock files are inside this task's `paths`. → Wire `scopeId` against the seeded scopes (the
  rule parser in `presets.ts` is already there), or record the gap in T-019's notes so the frontend does not hunt a
  "wrong count" bug.
- [ ] [nit] internal/api/graphql/schema/scope.graphqls:18 — "Surrounding whitespace is trimmed. Entries are stored as
  given." reads as a contradiction, and clients care: `scope.Service.clean` stores entries verbatim
  (`internal/scope/service.go:35-58`), whitespace is only stripped when a rule is parsed for matching, so
  `createScopePreset(allow: [" example.com "])` echoes the spaces back. → "Surrounding whitespace is ignored when
  matching; entries are returned exactly as entered."
- [ ] [nit] internal/api/graphql/schema/scope.graphqls:47-48 — The parenthetical promises a quoted offending entry, but a
  blank entry yields the bare "empty scope entry" (`internal/scope/match.go:27`). → Add "…, or `empty scope entry` for a
  blank rule".
- [ ] [nit] internal/api/graphql/resolvers/stubs_filter.go:3,16 / stubs_scope.go:3,16 — The file comment sits between
  `package` and `import`, and the parameters are unnamed, so `FilterPreset(context.Context, string)` does not say which
  argument is the ID (`history.go`/`project.go` name theirs). T-092 copies these signatures. → Move the comment above
  the import block and name the parameters (`ctx context.Context, id string`).
- [ ] [nit] web/src/lib/api/mocks/exchange.ts:37,108 — `mockHistoryExchange`, `MockHistoryOptions` and `createMockClient`
  now also serve the preset roots, and `createPresetsMock()` takes no seeds. The names no longer describe the behaviour.
  → Rename to `mockExchange`/`MockOptions` (or add a `presets` option) when T-019 touches the mocks; the doc comment on
  line 36 is already updated, so this can wait.

**Verdict: approve.** No blocker or major findings: the contract matches the services and the spec, generation is clean
and both AC test requirements are met. The minors are a two-line doc fix plus mock-only divergences that can ride along
with T-092 (backend) or T-019 (frontend). Status stays `in_review` until the operator merges.
