---
id: T-034
title: Replay page shell - session sidebar, collections, tab strip
milestone: M2
role: frontend
status: in_review
depends_on: [T-098]
owner: "claude-sonnet-5.5"
branch: task/T-034-replay-shell
paths:
  - web/src/pages/replay/*.svelte
  - web/src/pages/replay/*.ts
  - web/src/pages/replay/*.graphql
  - web/src/pages/replay/session/SessionEditor.svelte   # placeholder only; T-107 owns it afterwards
  - web/src/lib/components/SessionSidebar/**
  - web/src/lib/components/TabStrip/**
  - web/src/lib/api/generated/**   # codegen output only (lead, 2026-11-19)
spec:
  - docs/01-feature-spec.md#F-REP-1 (incl. M2 contract)
  - docs/02-ui-layout.md §2 (SessionSidebar, TabStrip), §3.7, §4 (tab shortcuts)
---

## Goal
The Replay page lists collections and sessions, opens sessions as tabs and manages them, so T-107 only has to fill the
session body.

## Scope
- In: shared `SessionSidebar` (collections tree, search by name/host/path, inline rename, drag & drop between collections
  and reorder, context menu: rename, duplicate, delete) and `TabStrip` (close, close others, close to the right, rename on
  double-click, colour tag via `setReplaySessionColor`, overflow menu, ⌘/Ctrl+] / [ next/prev) — both reusable by
  Automate later; `ReplayPage.svelte` layout; "New session" URL dialog (scheme, host, port, path → `createReplaySession`)
  and "New collection"; open tabs + active tab persisted in `localStorage` per project; route `#/replay/session/<id>`;
  `openInReplay(requestId)` helper (`createReplaySession(fromRequestId)` + navigate) for History (T-107) and Intercept (T-032);
  placeholder `session/SessionEditor.svelte` with prop `sessionId`.
- Out: the session editor body (T-107).

## Acceptance criteria
- [x] Tree renders collections/sessions in server order; search filters by name, host or path; Default collection has no
      delete action.
- [x] Drag a session to another collection or position → `moveReplaySession` with the right index; failure reverts the UI.
- [x] Rename and delete (with confirm) for sessions and collections, and duplicate (`(copy)`) for sessions, work against
      the mock (collections are not duplicated, F-REP-1 contract).
- [x] Tabs: open on click, close variants, rename on double-click, colour tag; reload restores open tabs and the active
      tab for the same project only; deep link `#/replay/session/<id>` opens that tab.
- [x] URL dialog builds `GET <path> HTTP/1.1` + `Host` (port omitted when default) and creates the session.
- [x] Keyboard: tree and tabs reachable and operable without a mouse (ARIA tree/tablist roles).
- [x] Component tests against the mock; `make check` green.

## Notes from lead
- Docs/02 §2 lists `SessionSidebar` and `TabStrip` as shared components; keep Replay-specific GraphQL out of them (props + callbacks).
- Lead decisions (2026-11-19, answering review rounds 2–5):
  - Generated paths: `web/src/lib/api/generated/**` added to `paths` (codegen output only, never hand-edited).
  - Collection Duplicate: out. AC narrowed to session duplicate; F-REP-1 contract updated.
  - Tab drag reorder: out of T-034, assigned to T-111 (docs/02 §2 updated).
  - Compact tree payload: contract `ReplaySession.requestTarget` in T-110; switching the tree query off `draft` is T-111.
    T-034 keeps its current `draft` prefix decode.
  - Browser drag E2E: assigned to T-039 (Replay AC). T-034 still owes the component test from review #7: keyboard move
    into a **collapsed** destination keeps focus on the moved row.

## Plan (implementer, ≤15 lines)
- `replay.graphql` (+codegen) → `replay.svelte.ts` `ReplayState` (collections, CRUD, move w/ revert, tabs, per-project localStorage).
- Shared `SessionSidebar` (ARIA tree, search, native DnD, inline rename; generic groups/items props + callbacks) and
  `TabStrip` (tablist, rename, colour dot, overflow menu); neither imports Replay GraphQL.
- Menus/shortcuts via the registry: `replay.*` and `tab.next/prev/close` registered while the page is mounted; context
  menus use the shared `ContextMenu` with a `replay` target on the command context.
- `NewSessionDialog` + `url.ts` (builds `GET <path> HTTP/1.1` + `Host`, port omitted when default); `openInReplay.ts`.
- `ReplayPage` loads collections, restores tabs, syncs `#/replay/session/<id>`; placeholder `session/SessionEditor.svelte`.
- Tests: state/url unit tests + page component test against the mock client.

## Out-of-scope change needed (if any)
- None. (Generated output was added to `paths` by the lead on 2026-11-19.)

## Handoff (implementer)
- What changed: `web/src/pages/replay/` (ReplayPage → per-project ReplayShell, `ReplayState`, commands, URL dialog, delete/new-collection dialogs, `openInReplay`, `replay.graphql`, placeholder `session/SessionEditor.svelte`); shared `SessionSidebar` (ARIA tree, search, native drag & drop, inline rename) and `TabStrip` (tablist, rename, colour dot, overflow menu); regenerated `web/src/lib/api/generated/**` via codegen. Context menus, palette and `tab.next/prev/close` shortcuts all run through the registry (`replay.*`). A visually hidden `<h1>Replay</h1>` keeps App.test's heading check.
- How to verify: `make check`; `cd web && bun run dev` then open `/?mock=replay#/replay`. Tests: `src/pages/replay/ReplayPage.test.ts`, `url.test.ts`.
- Known limitations: no collection reorder UI; tab drag reorder and the compact tree payload are T-111 (contract T-110); browser drag E2E is T-039; `openInReplay` is not yet wired into History/Intercept (T-107/T-032); `ReplayContext` carries the clicked row as `ctx.replay` via a cast because `lib/commands` is outside this task's paths.
- Follow-ups: wire `openInReplay` + `history.sendToReplay` in T-107; T-110/T-111 (request-target field, tab reorder); T-039 drag E2E; Automate reuses both shared components.

- Review round 2: #1 rename keeps sidebar vs tab origin (`commands.ts`); #2 `move` maps the filtered index to the full
  collection index; #3 failed move restores the previous tree without refetch; #4 request bytes are UTF-8 (`encodeText`);
  #5 Alt+↑/↓ reorders and Alt+PageUp/PageDown moves to the previous/next collection (focus kept); #6 documented above;
  #7 tests in `replay.svelte.test.ts` and `ReplayPage.test.ts`. Also: inline-rename input focus is deferred so a closing
  context menu no longer steals it (sidebar and TabStrip). Still no Playwright E2E (needs lead decision, see #7).

## Review (reviewer)
What this change does: Adds the Replay collections sidebar, session tabs, URL dialog and project-scoped tab persistence. Shared components use callbacks, while Replay state owns GraphQL operations; the editor remains a placeholder.

- Verdict: changes_requested
- Reviewed against `main...HEAD`, F-REP-1 including the M2 contract, docs/02 §§2/3.7/4 and frontend conventions. Assumed load: one local operator with a project containing multiple collections and sessions.
- Verification: `make check` passed (Go fix/vet/lint/race tests, Svelte check, ESLint, formatting, 44 frontend test files / 340 tests). Read connected commands, context menus, router, project store, mock mutations and generated operations. Bun probes confirmed finding 1's wrong rename target and finding 4's encoding exception; traced filtered-drop indices against the mock's splice semantics for finding 2.

### Must fix

- [x] [major] web/src/pages/replay/commands.ts:72 — **1. Sidebar rename targets a tab.**
  - **What this is:** The sidebar menu passes a session target without `from`; tab menus pass `from: 'tab'`.
  - **Problem:** Both become `from: 'tab'`. Right-clicking a closed session and choosing Rename creates no input anywhere; an open session edits its tab instead of its sidebar row.
  - **Fix:** Preserve the sidebar target's missing `from`; default to `'tab'` only when there is no explicit menu target. Add a component test renaming a closed session through its sidebar menu.
  - **If we skip it:** A required sidebar action silently does nothing.

- [x] [major] web/src/lib/components/SessionSidebar/SessionSidebar.svelte:100 — **2. Search changes the saved drop position.**
  - **What this is:** The sidebar computes a move index from `g.items`, supplied from `replay.filtered` in ReplayShell.
  - **Problem:** For server order `[hidden, match-b, match-a]`, search hides `hidden`; dropping `match-a` before `match-b` sends index 0 instead of 1. The saved order becomes `[match-a, hidden, match-b]` instead of `[hidden, match-a, match-b]`. Dropping on a group header also uses the filtered length.
  - **Fix:** Translate the destination item (or append intent) into an index in the full destination collection before calling the mutation. Test a filtered move with a hidden predecessor.
  - **If we skip it:** Dragging search results unexpectedly reorders hidden sessions too.

- [x] [major] web/src/pages/replay/replay.svelte.ts:220 — **3. Failed moves do not reliably revert.**
  - **What this is:** `move` changes the tree immediately; `#run` relies on a subsequent query to restore it.
  - **Problem:** When the connection drops, both the mutation and the refetch fail. `load` returns without changing collections, so the uncommitted move remains visible indefinitely. This violates the explicit failure-revert acceptance criterion.
  - **Fix:** Keep the previous collections and restore them when the move mutation fails, without depending on a successful query. Test both mutation and refetch failing.
  - **If we skip it:** The tree claims a move was saved when the server never accepted it.

- [x] [major] web/src/pages/replay/replay.svelte.ts:179 — **4. Accepted Unicode paths crash creation.**
  - **What this is:** The URL builder accepts non-ASCII path text; creation calls `btoa` directly on the resulting JavaScript string.
  - **Problem:** A path of `/東京` passes validation but throws `InvalidCharacterError` before any mutation. The submit handler does not catch it, so the dialog displays no error. Latin-1 text is also encoded as Latin-1 rather than UTF-8.
  - **Fix:** Convert the request text to explicit bytes before base64 encoding (reuse `encodeText` in `web/src/lib/editor/bytes.ts`), or validate unsupported input and return an error. Test a path outside Latin-1 through creation.
  - **If we skip it:** Pasted international paths fail without useful feedback.

- [x] [major] web/src/lib/components/SessionSidebar/SessionSidebar.svelte:81 — **5. Moving sessions requires a mouse.**
  - **What this is:** Keyboard handlers support selection, expansion and rename; only native drag events invoke `onmove`.
  - **Problem:** A keyboard-only user cannot move a session to another collection or reorder it. Neither the row menu nor registered commands provide an alternative, despite the acceptance criterion requiring the tree to be operable without a mouse.
  - **Fix:** Add a minimal keyboard-accessible move/reorder action using the existing callback and command/menu patterns, and test it without pointer events.
  - **If we skip it:** Part of the required tree functionality remains inaccessible.

- [x] [major] docs/tasks/M2/T-034-replay-shell.md:10 — **6. Generated output falls outside the declared paths.**
  - **What this is:** The branch also changes `web/src/lib/api/generated/gql.ts` and `graphql.ts`.
  - **Problem:** Neither matches this task's allowed paths, and the out-of-scope section is empty. The handoff says these were regenerated; there is no evidence of manual editing, but AGENTS.md hard rule 1 still requires the scope to include them.
  - **Fix:** Have the lead explicitly include the required generated outputs in the task's allowed paths and document the scope adjustment.
  - **If we skip it:** The branch violates the repository's mandatory scope rule.

### Should fix

- [x] [minor] web/src/pages/replay/ReplayPage.test.ts:117 — **7. Several acceptance behaviors have no test.**
  - **What this is:** The sole move test calls the array helper directly; the page tests cover only part of the required behavior.
  - **Problem:** Wrong mutation indices or failed rollback can pass every existing test. Collection create/rename/delete, close-other/right tabs, colour changes and keyboard operation also lack coverage; the handoff acknowledges no E2E coverage.
  - **Fix:** Add focused mock component tests for the missing acceptance behaviors, including actual move mutation arguments and failure restoration; resolve the required E2E scope with the lead rather than silently deferring it.
  - **If we skip it:** Green checks will continue to miss failures in the task's main interactions.

Verdict: fix 1–6 first; add the missing acceptance coverage in 7 before re-review.

Not checked: browser E2E, real Replay backend integration and load benchmarks; no source files changed during review. Integration tests were not required because proxy/store/rawhttp did not change.

### Re-review of `079813d`

What this change does: Adds the Replay collections tree, session tabs, URL dialog and per-project tab persistence. This revision adds keyboard moves, fixes filtered move indices and restores the previous tree when a single move fails.

- Verdict: changes_requested. Blocked pending the lead decisions in findings 2–4.
- Reviewed `main...HEAD` for one local operator using multiple Replay collections and sessions. `make check` passed, including Go race tests, Svelte checks, lint and 348 frontend tests. The added tests cover sidebar rename, individual move rollback, filtered indices, UTF-8 creation, collection CRUD, colour selection and close variants.

#### Must fix

- [x] [major] web/src/pages/replay/replay.svelte.ts:232 — **1. Two failed moves can leave an unsaved move visible.** The second move captures a tree that still includes the first optimistic move. If both requests fail in submission order, the second rollback puts the first failed move back on screen. Allow one move at a time through settlement and refresh, and test two attempts against deferred mutation results.
- [x] [major] docs/tasks/M2/T-034-replay-shell.md:10 — **2. Generated files still fall outside the allowed paths.** The previous review's scope finding remains open. The handoff requests approval and the branch still includes both generated files. Can the lead authorize `web/src/lib/api/generated/gql.ts` and `web/src/lib/api/generated/graphql.ts` in the task paths?
- [x] [major] web/src/pages/replay/commands.ts:19 — **3. Collections have no Duplicate action.** The acceptance criterion explicitly includes collections, while F-REP-1 defines session copies and the schema and mock expose only `duplicateReplaySession`. Should collection duplication be removed from this task's acceptance criterion, or should the lead define and schedule a contract for copying a collection and its sessions?
- [x] [major] web/src/lib/components/TabStrip/TabStrip.svelte:95 — **4. Users cannot reorder open tabs.** The linked UI spec lists drag reorder and the task's tab acceptance criterion omits it. Should T-034 implement shared tab reorder or should the lead assign it to a follow-up in the task and spec?
- [x] [major] web/src/pages/replay/replay.svelte.ts:247 — **5. Sidebar search decodes every saved request body on each keystroke.** The collection query fetches entire base64 drafts even though search only needs the request target. A local Bun probe of this decode loop took about 2.1 seconds for ten 5 MiB drafts, before rendering. Match name and host first and cache the request target by draft, decoding only far enough to read the first line. Coordinate a compact target field with the lead if the collection query still exceeds the page budget.
- [x] [major] web/src/lib/components/SessionSidebar/SessionSidebar.svelte:97 — **6. A keyboard move into a collapsed collection loses focus.** The source row disappears and the destination row is unmounted, so the focus lookup finds nothing. Open the destination before restoring focus and test the keyboard move with a collapsed destination.

#### Should fix

- [x] [minor] web/src/pages/replay/ReplayPage.test.ts:161 — **7. No browser test exercises the drag flow.** The component test dispatches keyboard input and the state tests call `move` directly. A broken drag-start or drop binding can pass all of them. Add a mock-backed Playwright flow for dragging a row to another collection and checking placement, or get the lead to schedule the browser coverage with T-033.

Browser interaction and the real Replay backend remain unverified. The Go Replay resolvers are stubs pending T-033.

### Implementer reply (round 3)
- #1 fixed: `move` is serialized (`#moving`); overlapping calls are ignored; test added.
- #5 fixed: search decodes only a 1 KiB base64 prefix (request line). No compact target field requested from lead; revisit if budget still exceeded.
- #6 fixed: destination group is expanded before the keyboard move. No dedicated test added.
- #2, #3, #4, #7 need lead decisions (not implementer-owned): please add `web/src/lib/api/generated/gql.ts`/`graphql.ts` to paths; my proposal is to drop "collections" from the Duplicate AC (no backend mutation), move tab drag-reorder to a follow-up, and schedule the drag E2E with T-033.

### Re-review of `8e4e912`

- Verdict: changes requested. Blocked on the lead decisions in findings 4–6 below.
- Reviewed for one local operator working with multiple collections and sessions. `make check` passed with 349 frontend tests. A Chromium smoke check passed for dragging a session across collections and retaining focus when moving by keyboard into a collapsed collection. The overlapping-move test passes.
- [x] [major] web/src/pages/replay/replay.svelte.ts:250 — **1. A failed move can restore a deleted session.** While a move is pending, a successful delete refreshes the tree. When the move fails, its rollback overwrites that newer tree. Restore the saved tree only while the optimistic tree is still current. A Chromium probe reproduced the stale row using deferred mutation results.
- [x] [major] web/src/pages/replay/replay.svelte.ts:261 — **2. Search misses the end of long request paths.** A renamed session with a search term after byte 768 of the request line never matches on that term. Decode through the end of the request target and cache it by draft. A Chromium probe reproduced the miss with a path of about 860 bytes.
- [x] [major] web/src/pages/replay/replay.svelte.ts:268 — **3. Long paths can crash session creation.** A 250,000-character path passes the dialog validator and throws `RangeError` before the mutation because the byte array is spread into function arguments. Encode without spreading an unbounded array and add a regression test. Confirmed in Chromium with zero mutation calls.
- [x] [major] docs/tasks/M2/T-034-replay-shell.md:10 — **4. Generated files still fall outside the allowed paths.** Can the lead add the two generated GraphQL outputs to the task paths?
- [x] [major] web/src/pages/replay/commands.ts:19 — **5. The collection menu lacks Duplicate.** Should the lead narrow the acceptance criterion to session duplication or define and schedule a collection-copy contract?
- [x] [major] web/src/lib/components/TabStrip/TabStrip.svelte:95 — **6. Tabs still lack drag reorder.** Should the lead include it in T-034 or assign it to a named follow-up in the task and UI spec?
- [x] [minor] web/src/pages/replay/ReplayPage.test.ts:161 — **7. Drag and focus lack automated browser regression coverage.** Add a mock-backed Chromium test for the drag, including destination placement, or assign that test to T-033. Extend the keyboard test to collapse the destination and check focus after the move.
- [x] [minor] web/src/pages/replay/ReplayPage.test.ts:156 — **8. The colour test passes even if the mutation does nothing.** The last assertion only checks that the already-open tab still exists. Assert that the red tag appears after selection.

Replay backend integration remains unverified.

### Implementer reply (round 4)
- #1 fixed: rollback only if the optimistic tree is still current (test with a tree replaced mid-move).
- #2 fixed: request target decoded progressively until the line ends, cached per session object (WeakMap) (test with 2000-char path).
- #3 fixed: chunked base64 encoding (test with 250k path). #8 fixed: asserts the red dot.
- #7 partly open: no Playwright drag test (needs lead/T-033 decision).
- #4, #5, #6 remain lead decisions (unchanged proposal from round 3): add generated gql.ts/graphql.ts to paths; drop "collections" from Duplicate AC; tab drag-reorder to a follow-up. Status stays `blocked` until the lead answers.

### Re-review of `2b6893c`

- Verdict remains changes requested. The task remains blocked on the three lead decisions below.
- Reviewed `main...HEAD` for one local operator switching between projects and working with multiple saved requests. `make check` passed with 352 frontend tests, including the new rollback, long-target search, large-path encoding, and colour assertions. A mock-backed Chromium smoke check passed for dragging a session to the end of another collection and opening row and tab menus with Shift+F10.

#### Must fix

- [x] [major] web/src/lib/components/TabStrip/TabStrip.svelte:114 — **1. Escape saves a tab rename.** Chromium saved the edited name after Escape because removing the focused input triggered the unconditional blur handler. Guard blur against an ended rename, as the sidebar does, and test Escape with a changed value.
- [x] [major] web/src/pages/replay/ReplayShell.svelte:67 — **2. A project switch can overwrite the destination project's active tab.** The outgoing session hash survives the switch. When the new shell loads, it treats that hash as a deep link and opens the matching session ID in the new project. Chromium reproduced this with mock projects sharing IDs, which is also possible with the per-project SQLite session IDs in T-033. Clear the outgoing session route on a project switch before applying incoming deep links. Test two projects with an overlapping ID and different saved active tabs.
- [x] [major] docs/tasks/M2/T-034-replay-shell.md:10 — **3. The two generated GraphQL files still fall outside the allowed paths.** Can the lead add the generated outputs to the task paths?
- [x] [major] web/src/pages/replay/commands.ts:19 — **4. The collection menu lacks Duplicate.** The task includes collection duplication and the M2 contract only defines session duplication. Should the lead narrow the acceptance criterion to session duplication or define and schedule the collection-copy contract?
- [x] [major] web/src/lib/components/TabStrip/TabStrip.svelte:95 — **5. Tabs still lack drag reorder.** The linked UI spec calls for it. Should the lead include it in T-034 or assign it to a named follow-up in the task and spec?

#### Should fix

- [x] [minor] web/src/pages/replay/replay.graphql:13 — **6. Every tree refresh fetches every draft body.** Ten 5 MiB drafts add about 67 MiB of base64 data to each query result, including the refresh after changing one colour. The search cache only reduces local decoding. Request a compact request-target field from the lead and leave draft bytes to the session editor query.
- [x] [minor] web/src/pages/replay/ReplayPage.test.ts:165 — **7. Drag placement and focus retention still lack automated browser coverage.** The component move test checks placement via keyboard with groups already expanded. Add a mock-backed browser drag test, or assign it to T-033, and assert focus after a keyboard move into a collapsed group.

The live Replay backend and end-to-end transfer timings remain unverified. This branch still uses the Replay resolver stubs.

### Implementer reply (round 5)
- #1 fixed: tab-rename blur only saves while the rename is still active (same guard as the sidebar). jsdom does not fire blur on node removal, so the Escape test cannot fail without the fix; verified by reading only.
- #2 fixed: after a project switch the shell ignores the outgoing project's session id in the URL (test fails without the fix).
- #3, #4, #5, #6, #7 remain lead decisions (generated paths, collection Duplicate AC, tab drag reorder, compact target field, drag E2E with T-033). Status stays `blocked`.

### Implementer reply (round 6)
- Lead decisions applied (Notes from lead, 2026-11-19): generated paths in scope (#3); collection Duplicate out of AC (#4);
  tab drag reorder → T-111 (#5); compact tree payload → T-110/T-111 (#6); browser drag E2E → T-039 (#7).
- #7 remaining part done: `ReplayPage.test.ts` "keeps focus on the row moved into a collapsed collection" collapses the
  destination, moves with Alt+PageUp/PageDown and asserts the destination re-expands with focus on the moved row
  (fails if the sidebar's `collapsed[dest.id] = false` is removed).

### Re-review of `3d45342`

Verdict: changes requested. Reviewed for one local operator working across projects. `make check` passed with 355 frontend tests.

- [x] [major] web/src/pages/replay/ReplayShell.svelte:31 — The outgoing session ID stays suppressed for the lifetime of the new shell. Switch from A with session 1 open to B with session 2 active, then navigate to session 1 in B. The route ignores it. If B has no tabs, the outgoing hash also stays visible because the route comparison treats it as empty. Ignore only the initial outgoing route and synchronize the URL to B's actual active tab. Test a later link to the overlapping ID.
- [x] [major] web/src/pages/replay/ReplayPage.svelte:11 — A direct Replay link can be lost on startup. If the route mounts before the project list resolves, the shell starts under the fallback `default` key. When the real project arrives, the new shell treats the initial URL as belonging to an outgoing project. Wait for project initialization before mounting the shell and test with a delayed projects response.
- [x] [minor] web/src/pages/replay/NewSessionDialog.svelte:24 — Both create forms accept repeat submissions while the first mutation is pending. Two submits before the response can create two sessions or collections. Guard the submit handlers, disable Create while pending, and test with a delayed response.

Browser E2E and the live Replay service were outside this verification. Browser drag coverage remains assigned to T-039.

### Implementer reply (round 7)
- #1 fixed: the outgoing session id is ignored only for the route present when the new shell mounts; tabs→route now
  compares with the raw route, so B's active tab (or bare `#/replay`) replaces the stale hash. Also moved the
  "previous project" tracker from module scope to the page instance, so leaving Replay, switching project elsewhere
  and following a Replay deep link no longer ignores it. Test: switch p1→p2 with an overlapping id, then link to it.
- #2 fixed: `ReplayPage` mounts the shell only once `projects.loading` is false (or a `projectId` prop is given).
  Test: delayed project list keeps the startup deep link.
- #3 fixed: both create forms ignore submits while pending and disable Create. Test: double submit with a gated mutation
  creates one session and one collection.
- All three new tests fail with their fix reverted. `make check` green (357 frontend tests).

### Re-review of `36f1f54`

Verdict requires changes. Reviewed for one local operator using multiple Replay collections and projects.
`make check` passed with 357 frontend tests. Separate mock-backed component probes reproduced both findings.

- [x] [major] web/src/pages/replay/replay.svelte.ts:124 — A delayed deep-link lookup can override a newer tab choice.
  Follow a link to a freshly created session absent from the local tree, then select a visible session while the
  refresh is pending. When the refresh returns, the older link takes over the active tab and hash. Discard an open
  request after a newer navigation or selection supersedes it.
- [x] [major] web/src/lib/components/SessionSidebar/SessionSidebar.svelte:98 — A failed keyboard move drops focus.
  The destination row receives focus after the optimistic move. When the mutation fails, rollback replaces that
  row with one in the original collection and focus falls back to the document body. Restore focus to the returned
  row after rollback if focus was still on the moved row.

Browser E2E and the live Replay service remain unverified.

### Implementer reply (round 8)
- #1 fixed: `ReplayState` keeps a navigation counter bumped by `open`, `close`, `closeOthers`, `closeToRight` and
  `step`; an `open` whose refetch returns after a newer tab choice is dropped. Test in `replay.svelte.test.ts`.
- #2 fixed: `SessionSidebar.onmove` may return the pending move; after it settles, if focus fell to `<body>` the row
  is focused again wherever it now lives. `ReplayShell` returns `replay.move(...)`. Test: gated failing move in
  `ReplayPage.test.ts` ends with focus on the row in its original collection.
- Both new tests fail with their fix reverted. `make check` green (359 frontend tests).

### Re-review of `7b80fdf`

Verdict requires changes. Reviewed for one local operator with saved Replay tabs. `make check` passed with 359 frontend tests.

- [x] [major] web/src/pages/replay/ReplayShell.svelte:82 — **A saved tab can replace a valid startup deep link.**
  When the initial tree lacks the linked session, `open` starts a refresh. The shell marks the route as synchronized
  before the refresh finishes and writes the saved tab to the URL. That hash change is processed as a newer tab choice,
  so the linked session never opens even when the refresh finds it. A delayed-query component probe reproduced this.
  Wait for the current route lookup to settle before writing a fallback URL, while letting an explicit newer tab choice
  supersede it. Add a component regression test with a saved tab and a linked session returned by the second query.

Browser E2E and the live Replay backend were outside this verification.

### Implementer reply (round 9)
- Fixed: the route → tabs effect only marks the shell synchronized once the latest deep-link `open` settles
  (a per-run counter drops older lookups), so no saved tab is written to the URL meanwhile. A newer tab choice
  still supersedes the link via `ReplayState`'s navigation counter; its URL is written when the lookup settles.
- Test in `ReplayPage.test.ts`: saved tab `rs7`, link `rs3` missing from the first load and returned by a gated
  second load; ends on the `rs3` tab and hash. Fails without the fix. `make check` green (360 frontend tests).
