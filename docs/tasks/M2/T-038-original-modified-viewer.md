---
id: T-038
title: Original/Modified viewer toggle and edited markers in HTTP History
milestone: M2
role: frontend
status: in_progress
depends_on: [T-030]
owner: "DeepSeek V4.1 Flash (opencode)"
branch: task/T-038-original-modified-viewer
paths:
  - web/src/lib/components/MessageViewer/**
  - web/src/pages/http-history/HttpHistoryPage.svelte
  - web/src/pages/http-history/HttpHistoryPage.test.ts
  - web/src/pages/http-history/detail.ts
  - web/src/pages/http-history/detail.test.ts
  - web/src/pages/http-history/columns.ts
  - web/src/pages/http-history/columns.test.ts
  - web/src/pages/http-history/http-history.graphql
spec:
  - docs/01-feature-spec.md#F-HIST-4 (incl. M2 contract)
  - docs/02-ui-layout.md §2 (MessageViewer toolbar), §3.1
---

## Goal
Both records of an edited pair are marked in history, and the viewer switches both panes between Original and Modified.

## Scope
- In: `MessageViewer` optional toolbar snippet prop (additive, no behaviour change for existing callers); the history
  detail fetches `editedFrom`/`editedBy` ids; an Original / Modified segmented switch shown when either exists, starting
  on the clicked row, switching request and response panes together; Edited column + row icon with tooltip on both
  records of a pair.
- Out: Intercept/Replay pages, `http-history/commands.ts` (T-107).

## Acceptance criteria
- [ ] Rows with `editedFrom` or `editedBy` show the edited marker (icon + tooltip, not colour only); others do not.
- [ ] Selecting either record of the seeded mock pair shows the switch; switching loads the other record into both panes.
- [ ] The switch is keyboard operable and announces the current state (ARIA).
- [ ] Existing `MessageViewer` tests pass unchanged.
- [ ] Component tests; `make check` green.

## Notes from lead
- Runs in parallel with T-107, which only edits `http-history/commands.ts`.

## Plan (implementer, ≤15 lines)
1. `MessageViewer`: optional `toolbar` snippet prop, rendered at the left of the toolbar row (additive).
2. `http-history.graphql`: add `editedFrom { id }` / `editedBy { id }` to `HistoryMessage`; rerun codegen.
3. `detail.ts`: `HistoryDetail` carries `editedFromId` / `editedById`; tests.
4. Page: `pair` derive (original id, modified id) from the clicked record; segmented Original/Modified switch in the
   request viewer's toolbar, bound to the record the user picked (falls back to the clicked record); the sibling
   record is fetched with `HistoryRequestDocument` and fed to both panes; response preview URL follows the shown id.
5. `columns.ts`: Edited column marker for both halves (`editedFrom` or `editedBy`) with the tooltip text; `EDITED`
   sort key stays `editedFrom`.
6. Row-cell tooltip needs a `TrafficTable` hook (see Out-of-scope change needed) — raised with the lead.
7. Tests: detail ids, column markers + sort key, page switch (both directions, keyboard, ARIA), existing viewer tests.

## Out-of-scope change needed (if any)
- `web/src/lib/components/TrafficTable/{types.ts,TableView.svelte}`: `TrafficColumn.value` is text-only, so a row cell
  cannot carry an icon + tooltip (`title`). AC 1 asks for "icon + tooltip". Smallest fix: an optional
  `hint?: (row: Row) => string | null` on `TrafficColumn`, rendered as `title` on the gridcell (native tooltip, same as
  T-104). Widen `paths` or confirm the plain-text marker.

## Handoff (implementer)
- What changed:
- How to verify:
- Known limitations:
- Follow-ups (proposed tasks):

## Review (reviewer)
- Verdict: approve | changes_requested
- Findings:
