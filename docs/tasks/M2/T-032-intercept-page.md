---
id: T-032
title: Intercept page, top-bar intercept toggle and sidebar badge
milestone: M2
role: frontend
status: todo
depends_on: [T-030, T-104, T-034]
owner: ""
branch: task/T-032-intercept-page
paths:
  - web/src/pages/intercept/**
  - web/src/lib/stores/intercept.svelte.ts
  - web/src/lib/stores/intercept.svelte.test.ts
  - web/src/lib/components/TopBar.svelte
  - web/src/lib/components/TopBar.test.ts
  - web/src/lib/components/Sidebar.svelte
  - web/src/lib/components/Sidebar.test.ts
  - web/src/lib/commands/builtin.ts
spec:
  - docs/01-feature-spec.md#F-INT-1 (incl. M2 contract)
  - docs/02-ui-layout.md §1 (top bar, sidebar badge), §3.4, §4 (shortcuts)
---

## Goal
Users can switch Forwarding/Queuing from the top bar, see the held count on the sidebar, and forward, edit or drop held
requests and responses on the Intercept page (against the T-030 mocks until T-109 lands).

## Scope
- In: `intercept.svelte.ts` store (settings + queue, kept live by both subscriptions, shared by top bar, sidebar, page);
  top-bar toggle (Forwarding ▶ / Queuing ‖) with dropdown for Requests / Responses; global command `intercept.toggle`
  (⌘/Ctrl+I, already bound in `shortcuts.ts`); sidebar badge; page per docs/02 §3.4: kind tabs, queue table (id, host,
  method, path, time held), `RawEditor` (T-104) for the selected item, Forward (⌘⏎) / Drop (⌘⌫) / Forward all / Drop all /
  Undo, condition bar (`QueryBar` HTTPQL + `ScopeSelector` bound to settings, drop status code field), Send to Replay (via
  the helper from T-034) and Copy as cURL.
- Out: WebSocket tab (T-053), add to scope, workflows.

## Acceptance criteria
- [ ] Toggle reflects `interceptSettings.queuing` and flips it; dropdown toggles `requests`/`responses`; ⌘/Ctrl+I toggles
      (shortcut test); a change from another client (mock subscription) updates the button.
- [ ] Sidebar badge equals the number of held items, updates live, hidden at 0, has an accessible label.
- [ ] Queue keeps FIFO order, appends new items without stealing selection, and the time-held column ticks.
- [ ] Forward sends `raw` only when the editor bytes differ from the held bytes (else `raw: null`); Undo restores the held
      bytes; after Forward/Drop the next item is selected.
- [ ] Bad condition → inline error from `BAD_QUERY` span; drop status outside 100–599 shows the server's `BAD_INPUT`.
- [ ] Empty state when nothing is held explains Forwarding/Queuing in our own words.
- [ ] Component tests against the mock exchange; `make check` green.

## Notes from lead
- Re-read docs/00 §4 before writing UI copy.
- Forward/Drop shortcuts are page-scoped; ⌘⌫ must stay a text edit inside the editor (existing shortcuts test covers the rule).

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
