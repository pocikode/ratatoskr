---
id: T-034
title: Replay page shell - session sidebar, collections, tab strip
milestone: M2
role: frontend
status: changes_requested
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
- [ ] Tree renders collections/sessions in server order; search filters by name, host or path; Default collection has no
      delete action.
- [ ] Drag a session to another collection or position → `moveReplaySession` with the right index; failure reverts the UI.
- [ ] Rename, duplicate (`(copy)`), delete (with confirm) for sessions and collections work against the mock.
- [ ] Tabs: open on click, close variants, rename on double-click, colour tag; reload restores open tabs and the active
      tab for the same project only; deep link `#/replay/session/<id>` opens that tab.
- [ ] URL dialog builds `GET <path> HTTP/1.1` + `Host` (port omitted when default) and creates the session.
- [ ] Keyboard: tree and tabs reachable and operable without a mouse (ARIA tree/tablist roles).
- [ ] Component tests against the mock; `make check` green.

## Notes from lead
- Docs/02 §2 lists `SessionSidebar` and `TabStrip` as shared components; keep Replay-specific GraphQL out of them (props + callbacks).

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

## Handoff (implementer)
- What changed: `web/src/pages/replay/` (ReplayPage → per-project ReplayShell, `ReplayState`, commands, URL dialog, delete/new-collection dialogs, `openInReplay`, `replay.graphql`, placeholder `session/SessionEditor.svelte`); shared `SessionSidebar` (ARIA tree, search, native drag & drop, inline rename) and `TabStrip` (tablist, rename, colour dot, overflow menu); regenerated `web/src/lib/api/generated/**` via codegen. Context menus, palette and `tab.next/prev/close` shortcuts all run through the registry (`replay.*`). A visually hidden `<h1>Replay</h1>` keeps App.test's heading check.
- How to verify: `make check`; `cd web && bun run dev` then open `/?mock=replay#/replay`. Tests: `src/pages/replay/ReplayPage.test.ts`, `url.test.ts`.
- Known limitations: no tab drag-reorder, no collection reorder UI (docs mention tab drag reorder; not in this task's AC); no Playwright E2E (the page is only user-visible against the mock until T-033 lands); move failure revert is covered by the refetch in `#run`, not by a dedicated test; `openInReplay` is not yet wired into History/Intercept (T-107/T-032); `ReplayContext` carries the clicked row as `ctx.replay` via a cast because `lib/commands` is outside this task's paths.
- Follow-ups (proposed tasks): wire `openInReplay` + `history.sendToReplay` in T-107; tab drag reorder; Automate reuses both shared components.

## Review (reviewer)
What this change does: Adds the Replay collections sidebar, session tabs, URL dialog and project-scoped tab persistence. Shared components use callbacks, while Replay state owns GraphQL operations; the editor remains a placeholder.

- Verdict: changes_requested
- Reviewed against `main...HEAD`, F-REP-1 including the M2 contract, docs/02 §§2/3.7/4 and frontend conventions. Assumed load: one local operator with a project containing multiple collections and sessions.
- Verification: `make check` passed (Go fix/vet/lint/race tests, Svelte check, ESLint, formatting, 44 frontend test files / 340 tests). Read connected commands, context menus, router, project store, mock mutations and generated operations. Bun probes confirmed finding 1's wrong rename target and finding 4's encoding exception; traced filtered-drop indices against the mock's splice semantics for finding 2.

### Must fix

- [ ] [major] web/src/pages/replay/commands.ts:72 — **1. Sidebar rename targets a tab.**
  - **What this is:** The sidebar menu passes a session target without `from`; tab menus pass `from: 'tab'`.
  - **Problem:** Both become `from: 'tab'`. Right-clicking a closed session and choosing Rename creates no input anywhere; an open session edits its tab instead of its sidebar row.
  - **Fix:** Preserve the sidebar target's missing `from`; default to `'tab'` only when there is no explicit menu target. Add a component test renaming a closed session through its sidebar menu.
  - **If we skip it:** A required sidebar action silently does nothing.

- [ ] [major] web/src/lib/components/SessionSidebar/SessionSidebar.svelte:100 — **2. Search changes the saved drop position.**
  - **What this is:** The sidebar computes a move index from `g.items`, supplied from `replay.filtered` in ReplayShell.
  - **Problem:** For server order `[hidden, match-b, match-a]`, search hides `hidden`; dropping `match-a` before `match-b` sends index 0 instead of 1. The saved order becomes `[match-a, hidden, match-b]` instead of `[hidden, match-a, match-b]`. Dropping on a group header also uses the filtered length.
  - **Fix:** Translate the destination item (or append intent) into an index in the full destination collection before calling the mutation. Test a filtered move with a hidden predecessor.
  - **If we skip it:** Dragging search results unexpectedly reorders hidden sessions too.

- [ ] [major] web/src/pages/replay/replay.svelte.ts:220 — **3. Failed moves do not reliably revert.**
  - **What this is:** `move` changes the tree immediately; `#run` relies on a subsequent query to restore it.
  - **Problem:** When the connection drops, both the mutation and the refetch fail. `load` returns without changing collections, so the uncommitted move remains visible indefinitely. This violates the explicit failure-revert acceptance criterion.
  - **Fix:** Keep the previous collections and restore them when the move mutation fails, without depending on a successful query. Test both mutation and refetch failing.
  - **If we skip it:** The tree claims a move was saved when the server never accepted it.

- [ ] [major] web/src/pages/replay/replay.svelte.ts:179 — **4. Accepted Unicode paths crash creation.**
  - **What this is:** The URL builder accepts non-ASCII path text; creation calls `btoa` directly on the resulting JavaScript string.
  - **Problem:** A path of `/東京` passes validation but throws `InvalidCharacterError` before any mutation. The submit handler does not catch it, so the dialog displays no error. Latin-1 text is also encoded as Latin-1 rather than UTF-8.
  - **Fix:** Convert the request text to explicit bytes before base64 encoding (reuse `encodeText` in `web/src/lib/editor/bytes.ts`), or validate unsupported input and return an error. Test a path outside Latin-1 through creation.
  - **If we skip it:** Pasted international paths fail without useful feedback.

- [ ] [major] web/src/lib/components/SessionSidebar/SessionSidebar.svelte:81 — **5. Moving sessions requires a mouse.**
  - **What this is:** Keyboard handlers support selection, expansion and rename; only native drag events invoke `onmove`.
  - **Problem:** A keyboard-only user cannot move a session to another collection or reorder it. Neither the row menu nor registered commands provide an alternative, despite the acceptance criterion requiring the tree to be operable without a mouse.
  - **Fix:** Add a minimal keyboard-accessible move/reorder action using the existing callback and command/menu patterns, and test it without pointer events.
  - **If we skip it:** Part of the required tree functionality remains inaccessible.

- [ ] [major] docs/tasks/M2/T-034-replay-shell.md:10 — **6. Generated output falls outside the declared paths.**
  - **What this is:** The branch also changes `web/src/lib/api/generated/gql.ts` and `graphql.ts`.
  - **Problem:** Neither matches this task's allowed paths, and the out-of-scope section is empty. The handoff says these were regenerated; there is no evidence of manual editing, but AGENTS.md hard rule 1 still requires the scope to include them.
  - **Fix:** Have the lead explicitly include the required generated outputs in the task's allowed paths and document the scope adjustment.
  - **If we skip it:** The branch violates the repository's mandatory scope rule.

### Should fix

- [ ] [minor] web/src/pages/replay/ReplayPage.test.ts:117 — **7. Several acceptance behaviors have no test.**
  - **What this is:** The sole move test calls the array helper directly; the page tests cover only part of the required behavior.
  - **Problem:** Wrong mutation indices or failed rollback can pass every existing test. Collection create/rename/delete, close-other/right tabs, colour changes and keyboard operation also lack coverage; the handoff acknowledges no E2E coverage.
  - **Fix:** Add focused mock component tests for the missing acceptance behaviors, including actual move mutation arguments and failure restoration; resolve the required E2E scope with the lead rather than silently deferring it.
  - **If we skip it:** Green checks will continue to miss failures in the task's main interactions.

Verdict: fix 1–6 first; add the missing acceptance coverage in 7 before re-review.

Not checked: browser E2E, real Replay backend integration and load benchmarks; no source files changed during review. Integration tests were not required because proxy/store/rawhttp did not change.
