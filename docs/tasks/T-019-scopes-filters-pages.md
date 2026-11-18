---
id: T-019
title: Scopes and Filters pages, scope selector, preset chips
milestone: M1
role: frontend
status: todo
depends_on: [T-017, T-091]
owner: ""
branch: task/T-019-scopes-filters-pages
paths:
  - web/src/pages/scopes/**
  - web/src/pages/filters/**
  - web/src/lib/components/ScopeSelector*.svelte
  - web/src/lib/components/PresetChips*.svelte
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

## Out-of-scope change needed (if any)

## Handoff (implementer)

## Review (reviewer)
