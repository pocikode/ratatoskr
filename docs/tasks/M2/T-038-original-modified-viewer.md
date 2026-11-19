---
id: T-038
title: Original/Modified viewer toggle and edited markers in HTTP History
milestone: M2
role: frontend
status: done
depends_on: [T-030]
owner: "DeepSeek V4.1 Flash (opencode)"
branch: task/T-038-original-modified-viewer
paths:
  - web/src/lib/components/MessageViewer/**
  - web/src/lib/components/TrafficTable/types.ts
  - web/src/lib/components/TrafficTable/TableView.svelte
  - web/src/lib/components/TrafficTable/TrafficTable.test.ts
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
- [x] Rows with `editedFrom` or `editedBy` show the edited marker (icon + tooltip, not colour only); others do not.
- [x] Selecting either record of the seeded mock pair shows the switch; switching loads the other record into both panes.
- [x] The switch is keyboard operable and announces the current state (ARIA).
- [x] Existing `MessageViewer` tests pass unchanged.
- [x] Component tests; `make check` green.

## Notes from lead
- Runs in parallel with T-107, which only edits `http-history/commands.ts`.
- AC 1 tooltip = native `title` + `aria-label` on the gridcell via the optional `TrafficColumn.hint`; no custom tooltip
  component.

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
- Resolved by lead 2026-11-19: `TrafficTable` paths widened per option (a) — `TrafficColumn.hint` (native `title` +
  `aria-label` on the gridcell).

## Handoff (implementer)
- What changed:
  - `MessageViewer`: additive `toolbar?: Snippet` prop, rendered in the toolbar row after the view-mode tabs.
  - `TrafficTable`: `TrafficColumn.hint?` — the gridcell renders it as native `title` and as `aria-label`.
  - `HistoryRow`/`HistoryMessage` fragments now select `editedBy { id }`; `HistoryDetail` exposes `editedFromId` /
    `editedById`.
  - Edited column: `✎` on both halves of a pair, tooltip saying which half it is; `EDITED` sort key still `editedFrom`.
  - History page: an Original / Modified switch (`role=group`, `aria-pressed` buttons) from either record, loading the
    sibling via `HistoryRequestDocument`; both panes plus the preview URL follow the shown record. The switch flips only
    once the other record's bytes have arrived, so the pressed state never disagrees with the panes.
- Review fixes (2026-11-19):
  - A selection change clears `chosen` / `sibling` and bumps `siblingToken` (an `$effect` on `Selection.anchor`), so a
    reselected row starts on its own record and a slow sibling fetch for the previous row is dropped, error message
    included. Regression test: "starts a reselected row on its own record after switching elsewhere" (fails without the
    fix).
- How to verify: `cd web && bun run test`; or `bun run dev`, open `/?mock=history` (or `localStorage['history.mock']=1`),
  and pick a row whose Edited column shows `✎` (ids 24/25 with the 500-row mock). `make check` green.
- Known limitations: the sibling is fetched on click, so the swap waits for one round trip; the switch is a pair of
  `aria-pressed` buttons (Tab + Enter/Space) rather than a radiogroup with arrow-key semantics; the tooltip is a native
  `title` (no styled popover, ~1 s hover delay); no Playwright spec — `web/tests/e2e/**` is outside this task's paths.
- Follow-ups (proposed tasks): E2E spec for the pair marker + switch on the seeded mock; preload both records with the
  detail query if the switch latency is noticeable on large bodies.

## Review (reviewer)
- Verdict: approve
- Checks: diff is inside `paths` (generated files come from `make gen`). `make check` green (Go cached; web check clean, 304 unit tests pass).
- Round 1 findings (stale `chosen`, uncancelled `showRecord`) are fixed in 9096b4f: an `$effect` on `selection.anchor` clears `chosen`/`sibling` and bumps `siblingToken`; regression test added. Verified the token check also drops the stale error message.
- Findings:
- [x] [major] HttpHistoryPage.svelte — `chosen` never cleared on reselect → fixed in 9096b4f.
- [x] [minor] HttpHistoryPage.svelte — stale `showRecord` result after selection change → fixed in 9096b4f.
- [x] [nit] switch uses `aria-pressed` buttons; accepted (Handoff).
- [ ] [nit] HttpHistoryPage.svelte (switch) — if the sibling fetch is in flight there is no pending indicator; acceptable, noted as a follow-up in Handoff.
