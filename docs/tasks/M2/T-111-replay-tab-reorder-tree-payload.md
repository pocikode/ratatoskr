---
id: T-111
title: TabStrip drag reorder; Replay tree reads requestTarget instead of draft
milestone: M2
role: frontend
status: todo
depends_on: [T-034, T-110]
owner: ""
branch: task/T-111-replay-tab-reorder-tree-payload
paths:
  - web/src/lib/components/TabStrip/**
  - web/src/pages/replay/*.svelte
  - web/src/pages/replay/*.ts
  - web/src/pages/replay/*.graphql
  - web/src/lib/api/generated/**   # codegen output only
spec:
  - docs/01-feature-spec.md#F-REP-1 (M2 contract, T-034 escalation bullet)
  - docs/02-ui-layout.md §2 (TabStrip), §4
---

## Goal
Open tabs can be reordered by drag and by keyboard, and the Replay collections query no longer downloads draft bytes.

## Scope
- In: `TabStrip` drag reorder via an `onreorder(id, index)` callback (shared, no Replay GraphQL), keyboard equivalent
  (focused tab, Alt+←/→); Replay persists the new order with the open tabs in `localStorage`. Replace `draft` in the
  tree fragment with `requestTarget`; path search uses it; drop the draft prefix decode and its cache.
- Out: collection drag reorder; session editor (T-107).

## Acceptance criteria
- [ ] Dragging a tab before/after another reorders it; Alt+←/→ moves the focused tab and keeps focus; order survives reload
      for the same project only.
- [ ] `ReplayCollections` no longer selects `draft`; search by path still matches the end of a 2000-character target.
- [ ] Component tests against the mock; `make check` green.

## Notes from lead
- Keep keyboard parity with the sidebar's Alt+↑/↓ move.

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
