---
id: T-095
title: HTTP History paging via TrafficTable near-end hook (follow-up of T-017, unblocks T-021)
milestone: M1
role: frontend
status: in_review
depends_on: [T-017]
owner: "Codex"
branch: task/T-095-history-pagination
paths:
  - web/src/lib/components/TrafficTable/**
  - web/src/pages/http-history/**
  - web/src/lib/api/mocks/history.ts
  - web/src/lib/api/generated/**   # via `make gen` only, never hand-edited
  - docs/tasks/T-017-http-history-page.md   # Known limitations only
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
- [x] `TrafficTable` takes an optional `onNearEnd?: () => void` callback prop, called when the rendered window comes
      within one viewport of the last loaded row. It is not called again until the row count changes.
- [x] `HistoryState` keeps `pageInfo.endCursor`/`hasNextPage` from each response and, on `onNearEnd`, loads the next
      page with `first: 1000, after: endCursor` and the same `query`/`scopeId`/`sort`. It appends the edges and drops any
      whose id is already loaded (live rows can overlap a page boundary).
- [x] At most one page request is in flight. A reload (new query, scope, sort or project) cancels paging the same way
      `reloadToken` cancels stale reloads today, so no stale page is appended.
- [x] No request once `hasNextPage` is false. A paging error sets `message` and keeps the loaded rows.
- [x] The keyboard cursor and selection follow the row key, not the index: a live row prepended above the cursor does
      not change which request is active.
- [x] Vitest: paging over a mock of ≥ 3,000 rows reaches the last row, with no duplicates and no request after the last
      page. A reload during an in-flight page discards that page. Prepending a row does not move `activeKey`.
- [x] Rendering stays virtualized: DOM row count does not grow with loaded rows (assert in the component test).
- [x] `make check` green.

## Notes from lead
- `HistoryRequests` in `http-history.graphql` does not send `after` yet: add `$after: String` and pass it to
  `requests(...)`, then run `make gen` and commit the regenerated `web/src/lib/api/generated/**`. No schema change.
- Use the existing mock in `lib/api/mocks/history.ts` (it already handles `after`). Extend it only if needed for
  `hasNextPage`.
- No new dependencies. Keep `PAGE = 1000` (schema max). Update the comment at `history.svelte.ts:29` and remove the
  T-017 known-limitation wording you make obsolete.
- T-021 runs in parallel and adds the 100k-row E2E after this merges. Keep the table's existing roles/labels stable
  so its selectors keep working.

## Plan (implementer, ≤15 lines)

1. Add the cursor to the history GraphQL operation and regenerate the frontend documents.
2. Trigger paging from the virtual window and track the active row by key.
3. Reuse the successful reload's query arguments for paging and cancel superseded requests.
4. Cover scrolling, virtualized row counts, live overlap, paging errors, and reloads in Vitest.

## Out-of-scope change needed (if any)

- **Resolved (lead, 2026-11-19):** both granted, see `paths` and Notes from lead.
- Add `web/src/lib/api/generated/**` to `paths`. The schema already accepts `after`, but the `HistoryRequests`
  operation omits it. Updating `web/src/pages/http-history/http-history.graphql` to send the cursor requires
  regenerating the checked-in frontend GraphQL document and types.
- Add `docs/tasks/T-017-http-history-page.md` to `paths` so the first-page-only limitation can be removed as requested.

## Handoff (implementer)

- The virtual table requests 1,000-row cursor pages as it nears the end. Live overlaps are deduplicated in sort order,
  and live deletions take precedence over an in-flight page. Reloads cancel paging, and the active row follows its key.
- Successful reloads keep the grid mounted, preserving header focus and the active row when its key is still present.
  A new first page re-arms paging even at the same row count. Project switches reset the grid because row IDs are
  project-scoped.
- A paging error preserves loaded rows. The next row count change or successful reload re-arms automatic paging.
  A successful page clears the paging alert, and any newer alert from another action stays visible.
- `make check` passed with local test sockets enabled. Vitest passed 231 tests covering 3,050-row paging,
  cancellation, bounded DOM rows, keyboard sorting, and paging error recovery. The initial implementation ran
  `make gen`.
- T-021 owns the 100,000-row browser test.

## Review (reviewer)
- Verdict: changes_requested
- Checks: `make check` green (Go tests ok, svelte-check 0 errors, eslint/prettier clean, Vitest 36 files / 229 tests).
  `make test-int` not run (no proxy/store/rawhttp change). All changed files are inside `paths`.
- AC coverage: all eight AC have tests (3,050-row paging + no request after the last page, in-flight cancel on
  reload and project reset, paging error, deleted-during-page, prepend keeps `activeKey`, bounded DOM rows).
  The `compareRows` tie-break change (ties by ID in the sort direction) now matches `internal/history/list.go:82,210`.
- Findings:
- [x] [major] web/src/pages/http-history/HttpHistoryPage.svelte:193 + history.svelte.ts:237 — `{#key history.tableVersion}`
      remounts the whole `TrafficTable` after every successful reload. That includes a header sort (`setSort` → `reload`),
      the reload command and `addHostToScope`. Case: a keyboard user tabs to the "Host" header and presses Enter. The
      focused header button is destroyed, focus falls to `<body>`, and the user loses their place. The grid's
      `activeKey` is also reset, so the next ArrowDown starts at row 0, not at the still-selected row. This breaks
      docs/02 §203 "Full keyboard navigation" and worked before this branch. → Remove `tableVersion` and the `{#key}`.
      In `TableView.svelte:86-92`, re-arm when the last row changes, not only the count:
      `const last = rows.at(-1); if (count !== previousCount || last !== previousLast) notified = false;`.
      A reload creates new row objects, so a same-count reload re-arms. A prepended live row leaves the last row
      unchanged, so it does not. AC 1 says "row count". Note in Handoff that it is "row count or last row" (or ask
      lead). Add a test: sort by header with keyboard focus → `document.activeElement` is still the header button.
  - Implementer: a successful first-page reload explicitly re-arms the callback. Within that result set, row count
    controls re-arming. This keeps a same-count note or color update from triggering another request. The keyboard
    sort test checks focus and the active request after reloading.
- [x] [minor] web/src/pages/http-history/history.svelte.ts:259-263 — after a paging error, a later page can succeed.
      For example, a live row changes the count, `onNearEnd` fires again, and the page loads. `message` still shows
      "Page unavailable" because only `reload` clears it (line 241). → After `page.info = connection.pageInfo`, add
      `if (this.message === pagingError) this.message = null`, keeping the failed text in a local or field. Or clear
      it only when the last message came from paging. Extend the existing error test with a second, successful call.
  - Implementer: paging recovery clears its own alert. The test also checks that it preserves a newer alert.
- [x] [nit] web/src/pages/http-history/history.svelte.ts:259 — after a paging error, `TableView` stays `notified`
      until the row count changes. With no live traffic, scrolling never retries, and the only way out is a full
      reload. The Handoff documents this. Fine for now. Optional follow-up: re-arm when `message` is cleared, or show
      a "Retry" action in the message.
  - Implementer: Handoff records when automatic paging retries. A dedicated retry action remains a follow-up.
