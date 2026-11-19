---
id: T-038
title: Original/Modified viewer toggle and edited markers in HTTP History
milestone: M2
role: frontend
status: todo
depends_on: [T-030]
owner: ""
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

## Out-of-scope change needed (if any)

## Handoff (implementer)
- What changed:
- How to verify:
- Known limitations:
- Follow-ups (proposed tasks):

## Review (reviewer)
- Verdict: approve | changes_requested
- Findings:
