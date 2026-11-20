---
id: T-032
title: Intercept page, top-bar intercept toggle and sidebar badge
milestone: M2
role: frontend
status: in_review
depends_on: [T-030, T-104, T-034]
owner: claude-code (sonnet)
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
- [ ] A held message over 16 MiB opens read-only with a short note that it can only be forwarded unchanged or dropped
      (F-INT-1); Forward then sends `raw: null`.
- [ ] Bad condition → inline error from `BAD_QUERY` span; drop status outside 100–599 shows the server's `BAD_INPUT`.
- [ ] Empty state when nothing is held explains Forwarding/Queuing in our own words.
- [ ] Component tests against the mock exchange; `make check` green.

## Notes from lead
- Re-read docs/00 §4 before writing UI copy.
- Forward/Drop shortcuts are page-scoped; ⌘⌫ must stay a text edit inside the editor (existing shortcuts test covers the rule).

## Plan (implementer, ≤15 lines)
- `intercept.graphql` + codegen; `intercept.svelte.ts`: ref-counted `start()` (settings/queue queries, both subscriptions, refetch on project reset and WS reconnect), actions (settings, forward, drop, all).
- Singleton `intercept` store; TopBar toggle + dropdown (Requests/Responses), Sidebar badge (`aria-label`), `intercept.toggle` in builtin.ts.
- Page: kind tabs, FIFO queue table with ticking time-held, RawEditor, Forward/Drop/Undo/all, condition bar (QueryBar + ScopeSelector + drop status), Send to Replay (openInReplay), Copy as cURL; page commands registered while mounted.
- Over 16 MiB held message: read-only + note, forward sends `raw: null`.
- Tests: store, TopBar, Sidebar, page against the mock exchange.

## Out-of-scope change needed (if any)

## Handoff (implementer)
- What changed: `intercept.graphql` (+ codegen output); `stores/intercept.svelte.ts` (ref-counted live store: settings, FIFO queue, both subscriptions, refetch on project reset/WS reconnect, forward/drop/all actions); TopBar toggle + Requests/Responses dropdown; Sidebar badge; `intercept.toggle`/`toggleRequests`/`toggleResponses` in `builtin.ts`; Intercept page (kind tabs, TrafficTable queue with ticking time-held, RawEditor, Forward/Drop/Undo/all, QueryBar + ScopeSelector + drop status, Send to Replay, Copy as cURL, page commands `intercept.forward/drop/forwardAll/dropAll/undo/replay/copy.curl`).
- How to verify: `make check`; run `bunx --bun vitest run src/pages/intercept src/lib/components/TopBar.test.ts src/lib/components/Sidebar.test.ts src/lib/stores/intercept.svelte.test.ts`.
- Known limitations: Copy as cURL is the History helper (method + URL only, no headers/body). Selection is single (shared `Selection` object kept on the current item). No E2E spec (backend T-109 not wired). Time-held column is not sortable.
- Follow-ups (proposed tasks): E2E for Intercept once T-109 lands; richer cURL with headers/body.

## Review (reviewer)
- Verdict: approve | changes_requested
- Findings:
