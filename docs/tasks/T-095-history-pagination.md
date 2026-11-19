---
id: T-095
title: HTTP History paging via TrafficTable near-end hook (follow-up of T-017, unblocks T-021)
milestone: M1
role: frontend
status: todo
depends_on: [T-017]
owner: ""
branch: task/T-095-history-pagination
paths:
  - web/src/lib/components/TrafficTable/**
  - web/src/pages/http-history/**
  - web/src/lib/api/mocks/history.ts
spec:
  - docs/01-feature-spec.md#F-HIST-1
  - docs/06-frontend-conventions.md §7
  - docs/tasks/T-017-http-history-page.md (Known limitations, Out-of-scope #3)
---

## Goal
The HTTP History page can scroll through every matching request (100k+), not just the first 1000, by loading further
pages with the existing `requests(first, after)` cursor when the table nears its end.

## Scope
- In: a near-end callback prop on `TrafficTable`; cursor paging in `HistoryState`; `activeKey` so prepended live rows
  do not move the keyboard cursor (T-017 Out-of-scope #3.2); unit/component tests.
- Out: schema or backend changes (the contract already has `after`, `pageInfo.hasNextPage/endCursor`, `totalCount`);
  E2E specs (T-021 owns `web/tests/e2e/**`); scroll-accurate "↓ N new" pill.

## Acceptance criteria
- [ ] `TrafficTable` takes an optional `onNearEnd?: () => void` callback prop, called when the rendered window comes
      within one viewport of the last loaded row. It is not called again until the row count changes.
- [ ] `HistoryState` keeps `pageInfo.endCursor`/`hasNextPage` from each response and, on `onNearEnd`, loads the next
      page with `first: 1000, after: endCursor` and the same `query`/`scopeId`/`sort`. It appends the edges and drops any
      whose id is already loaded (live rows can overlap a page boundary).
- [ ] At most one page request is in flight. A reload (new query, scope, sort or project) cancels paging the same way
      `reloadToken` cancels stale reloads today, so no stale page is appended.
- [ ] No request once `hasNextPage` is false. A paging error sets `message` and keeps the loaded rows.
- [ ] The keyboard cursor and selection follow the row key, not the index: a live row prepended above the cursor does
      not change which request is active.
- [ ] Vitest: paging over a mock of ≥ 3,000 rows reaches the last row, with no duplicates and no request after the last
      page. A reload during an in-flight page discards that page. Prepending a row does not move `activeKey`.
- [ ] Rendering stays virtualized: DOM row count does not grow with loaded rows (assert in the component test).
- [ ] `make check` green.

## Notes from lead
- Use the existing mock in `lib/api/mocks/history.ts` (it already handles `after`). Extend it only if needed for
  `hasNextPage`.
- No new dependencies. Keep `PAGE = 1000` (schema max). Update the comment at `history.svelte.ts:29` and remove the
  T-017 known-limitation wording you make obsolete.
- T-021 runs in parallel and adds the 100k-row E2E after this merges. Keep the table's existing roles/labels stable
  so its selectors keep working.

## Plan (implementer, ≤15 lines)

## Out-of-scope change needed (if any)

## Handoff (implementer)
- What changed:
- How to verify:
- Known limitations:
- Follow-ups (proposed tasks):

## Review (reviewer)
- Verdict: approve | changes_requested
- Findings:
