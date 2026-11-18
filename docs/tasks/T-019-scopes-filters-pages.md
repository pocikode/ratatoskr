---
id: T-019
title: Scopes and Filters pages, scope selector, preset chips
milestone: M1
role: frontend
status: in_progress
depends_on: [T-017, T-091]
owner: "opencode/deepseek-v4.1-flash-1017"
branch: task/T-019-scopes-filters-pages
paths:
  - web/src/pages/scopes/**
  - web/src/pages/filters/**
  - web/src/lib/components/ScopeSelector*.svelte
  - web/src/lib/components/PresetChips*.svelte
  # Added 2026-11-19 (operator-approved): AC2/AC3 need the History page's QueryBar
  # snippets and row commands; the original list could not satisfy them.
  - web/src/pages/http-history/**
spec:
  - docs/01-feature-spec.md#F-SCOPE-1, #F-FILTER-1
  - docs/02-ui-layout.md §3.3
---

## Notes from lead
Build against the T-091 mocks. Real backend: T-092 (presets), T-090 (`scopeId`/`preset:` in history). Match counters use
`requests(scopeId){totalCount}` for saved scopes and `requests(query){totalCount}` for filter drafts (T-091 Scope).

## Acceptance criteria
- [ ] CRUD + live "matches N requests" counter.
- [ ] Scope selector and preset chips integrated into QueryBar on History.
- [ ] "Add host to scope / exclude" context actions.

## Plan (implementer, ≤15 lines)
1. Paths: `pages/http-history/**` added (operator-approved) for AC2/AC3.
2. `pages/scopes/scopes.graphql` + `pages/filters/filters.graphql`: list, `requests(first: 1){totalCount}` counter,
   CRUD, reorder, export/import → `bun run codegen`.
3. `lib/components/ScopeSelector.svelte` (native select, "No scope") and `PresetChips.svelte` (apply `preset:"Name"`);
   presentational, data comes from the page state.
4. `pages/scopes/scopes.svelte.ts` + `ScopesPage.svelte`: §3.3 two-column CRUD, allow/deny chip lists, reorder /
   duplicate / delete, "matches N requests" for the saved preset.
5. `pages/filters/filters.svelte.ts` + `FiltersPage.svelte`: alias + HTTPQL draft, debounced draft counter, export/import.
6. History: `scopeId` through `HistoryRequests` and both subscriptions + `HistoryState.setScope`; selector in QueryBar
   `leading`, chips in `trailing`.
7. `history.scope.add` / `history.scope.exclude` commands (active scope only) → `updateScopePreset` with the merged list.
8. Tests: scopes/filters pages + both components against `createMockClient`; history scope + chips. E2E stays with T-021
   (Playwright is not a dependency yet, as in T-017).
9. `make check` green, Handoff.

## Out-of-scope change needed (if any)

## Handoff (implementer)

## Review (reviewer)
