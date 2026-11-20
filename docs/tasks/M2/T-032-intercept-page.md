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

- Lead answer (2026-11-20, review round 1): finding 1 (generated `gql.ts`/`graphql.ts` outside paths) is resolved:
  regenerated output from `bun run codegen` for operations in `web/src/pages/intercept/intercept.graphql` is authorized;
  it must stay codegen output, never hand-edited. The reviewer's `golangci-lint` error came from its sandbox, not this
  branch. Address the remaining findings.

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
- Review round 2: all findings addressed (cancellable `store.query()`, draft pruning, Forward/Drop test).
- Review round 1: all findings addressed (see ticked items). Page split into `InterceptPage`, `ConditionBar`, `HeldEditor`, `commands.ts`; store gained `flip()`, `epoch`, event replay and revision guards.

## Review (reviewer)
- Verdict: changes_requested

### Round 1 findings

- [x] (resolved by lead answer; output is untouched codegen) [major] web/src/lib/api/generated/graphql.ts:55 — This file and `web/src/lib/api/generated/gql.ts` are outside the task's `paths`. The Handoff records codegen, which still changes files governed by the path restriction → Have the lead authorize both generated paths and keep the output generated.
- [x] (store logs queue events while a refresh is in flight and replays them on the snapshot; settings guarded by a revision counter; ordering tests in `intercept.svelte.test.ts`) [major] web/src/lib/stores/intercept.svelte.ts:152 — A queue query can return before the settings query, then an `ADDED` or `REMOVED` event can land before `Promise.all` assigns the earlier queue snapshot. This can hide a held item or leave a released item visible until the next refresh. Settings snapshots and mutation responses can likewise overwrite newer settings events → Reconcile queue events with in-flight snapshots, guard settings writes against newer events and project resets, and test out-of-order delivery.
- [x] (round 2 fixed via the urql finding: reset clears settings and disables toggles, and the old mutation result is guarded. The replacement query can still receive the old project's result. See the urql finding below.) [major] web/src/lib/stores/intercept.svelte.ts:130 — A project reset clears the queue but leaves the old settings active. The toggle commands read those settings to compute the next value, so switching from a queuing project to a forwarding project can make the first toggle send `queuing: false` again → Clear settings on reset, invalidate prior writes, and disable toggles until current-project settings arrive.
- [x] (round 2 fixed via the urql finding: `ConditionBar` clears and refetches presets on `store.epoch` and shows scope mutation errors. If the old presets request is still in flight, its result can still populate the new project's list. See the urql finding below.) [major] web/src/pages/intercept/InterceptPage.svelte:186 — Scope presets load only when the page mounts or the client changes. Switching projects while Intercept is open leaves the previous project's presets selectable, and the scope mutation discards the resulting error → Clear and refetch presets on project change and display scope update failures.
- [x] (drafts are kept per item ID and a release drops only its own; the shown item falls to the one now at its index, so selection moves only if the user stayed on it; tested) [major] web/src/pages/intercept/InterceptPage.svelte:69 — While Forward or Drop awaits the mutation, the user can select and edit another row. Completion clears that row's draft and can move selection back to the old item’s neighbor → Clear only the released item's draft and advance selection only if the user has stayed on that item. Test switching rows while a release is pending.
- [x] (page tests assert rendered order, next selection, live append, ticking time, pending release; TopBar covers both dropdown items) [major] web/src/pages/intercept/InterceptPage.test.ts:25 — The FIFO and release tests assert counts or store rows, leaving rendered order and the editor's selected item unverified. No component test delivers an `ADDED` event while another row is selected or advances the time-held clock. `TopBar.test.ts:60` clicks only Responses although both toggles are in the AC → Assert rendered FIFO and next selection, cover live append and ticking time, and exercise both dropdown actions.
- [x] (all queue columns `sortable: false`) [minor] web/src/pages/intercept/InterceptPage.svelte:156 — ID, Host, Method and Path show sort affordances, but the table leaves ordering to its caller and the page supplies no sort handler → Disable sorting on all queue columns to preserve FIFO.
- [x] (page is 231 lines; `ConditionBar`, `HeldEditor`, `commands.ts` extracted) [minor] web/src/pages/intercept/InterceptPage.svelte:1 — The 361-line component exceeds the 250-line limit in `docs/06-frontend-conventions.md` §3 → Extract the condition controls or editor pane within the task's page directory.

### Round 2 findings (d23adfa)

- [x] (`store.query()` subscribes to the urql operation and a project switch tears it down before the refetch, so nothing is shared; `ConditionBar` uses it too; real-urql overlap test in `intercept.svelte.test.ts`) [major] web/src/lib/stores/intercept.svelte.ts:167 — Project-reset queries reuse the previous project's urql operation keys. With an old HTTP query still pending, urql shares its result with the new `network-only` caller. The new caller has the latest sequence number, so it can restore old settings, re-enable the toggles, or show old held items. The same issue affects the preset refetch in `web/src/pages/intercept/ConditionBar.svelte:41`. A deferred-fetch check with the installed urql client issued one HTTP request for two concurrent queries and delivered the old payload to both callers → Cancel the old operation before refetching, or isolate in-flight operations by project. Cover the overlap with the real urql exchanges, since the manual store test supplies separate promises.
- [x] (an effect prunes drafts whose ID is no longer in the full queue; a reset empties the queue so it clears them too; tested) [major] web/src/pages/intercept/InterceptPage.svelte:58 — The new per-item draft map only deletes entries on Undo or a successful single-item release. Bulk actions, forwarding mode, remote removals, disconnects and project switches leave edited bytes in memory indefinitely while the page stays mounted. Repeatedly editing and releasing large messages this way can exhaust browser memory even when the live queue is empty → Prune drafts when their IDs leave the full queue, and clear them on project reset.
- [x] (`it.each` Forward/Drop asserts selected row and editor contents) [minor] web/src/pages/intercept/InterceptPage.test.ts:43 — The next-selection test name promises Forward and Drop, but the test only clicks Forward. The separate Drop test checks queue length alone → Exercise both actions and assert the selected row and editor contents after each.

### Round 1 verification

- `make check` stopped at Go lint:

  ```text
  golangci-lint run ./...
  level=error msg="Running error: context loading failed: no go files to analyze: running `go mod tidy` may solve the problem"
  make: *** [lint] Error 5
  ```

- `make -k web-check web-test` passed. Svelte reported 0 errors and 0 warnings, and all 444 tests across 54 files passed. `git diff --check main...HEAD` passed.
- Proxy, store backend and raw HTTP files are unchanged, so the reviewer procedure did not trigger `make test-int`. Live backend E2E remains unverified pending T-109.

### Round 2 verification

- `make check` passed with fresh writable temporary `GOCACHE` and `GOLANGCI_LINT_CACHE` directories and `GOFLAGS=-buildvcs=false`. Go lint reported 0 issues, the Go race suite passed, and all 457 frontend tests across 54 files passed. Svelte reported 0 errors and 0 warnings.
- `git diff --check main...HEAD` and `git diff --check` passed. Integration and live backend E2E were outside this review run for the same reasons as round 1.
