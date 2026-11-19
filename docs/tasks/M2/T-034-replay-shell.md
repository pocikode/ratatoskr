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
- Verdict: approve | changes_requested
- Findings:
