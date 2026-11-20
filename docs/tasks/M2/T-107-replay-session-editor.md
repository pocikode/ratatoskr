---
id: T-107
title: Replay session editor - connection bar, request editor, send/cancel, entry history, response; Send to Replay from History
milestone: M2
role: frontend
status: in_progress
depends_on: [T-034, T-104, T-103]
owner: "DeepSeek V4.1 Flash (opencode)"
branch: task/T-107-replay-session-editor
paths:
  - web/src/pages/replay/session/**
  - web/src/pages/http-history/commands.ts
  - web/src/pages/http-history/commands.test.ts
  - web/src/pages/http-history/HttpHistoryPage.svelte
  - web/src/pages/http-history/HttpHistoryPage.test.ts
spec:
  - docs/01-feature-spec.md#F-REP-2 (incl. M2 contract)
  - docs/02-ui-layout.md §3.7, §4 (Send ⌘⏎, Send to Replay ⌘R)
---

## Goal
A Replay tab edits, sends and cancels a request, browses its sent entries and shows responses; History rows can be sent
to Replay with ⌘/Ctrl+R or the context menu.

## Scope
- In: `SessionEditor.svelte` body per docs/02 §3.7: connection bar (scheme/TLS, host, port, SNI override, Apply M&R
  toggle, environment selector from the T-103 store, options popover: follow redirects, update Content-Length, timeout);
  `RawEditor` (T-104) with `variables` from the environment store; Send (`replay.send`, ⌘/Ctrl+Enter) and Cancel; draft
  autosave via `updateReplaySession` (debounced, ≤ 1 call/s, flushed before send and on tab switch); entry navigation
  `‹ n/N ›` + history dropdown; response pane with `MessageViewer` + status, size, roundtrip; error entries shown as an
  error banner; `history.sendToReplay` command in `http-history/commands.ts` using `openInReplay` (T-034), registered for
  the context menu and ⌘/Ctrl+R.
- Out: HTTP/2 selector (T-081), convert workflows (M5), WS/Pipeline sessions.

## Acceptance criteria
- [x] Send calls `sendReplaySession` with the editor bytes, connection and options; button shows progress; Cancel calls
      `cancelReplaySend`; a cancelled/failed entry shows its error.
- [x] Draft autosave: rapid typing produces ≤ 1 `updateReplaySession` per second; pending changes are saved before send
      and when switching tabs (fake timers test).
- [x] Entry navigation loads that entry's request into the editor and its response into the viewer; editing afterwards
      keeps the edit as the draft.
- [x] `{{var}}` hover shows the selected environment's value (secret masked); the env selector changes the project selection.
- [ ] From History, ⌘/Ctrl+R and the context-menu item create a session from the selected row and open its tab.
      ⌘/Ctrl+R is done and tested; the **context-menu item** needs one id in `MENU_COMMANDS`
      (`HttpHistoryPage.svelte`, T-038 — outside `paths`; see "Out-of-scope change needed").
- [x] Component tests against the mock; `make check` green.

## Notes from lead
- `historyCommands` must not need new deps from `HttpHistoryPage.svelte` (owned by T-038): import `openInReplay` directly.

- Lead answer (2026-11-20): T-038 is done and nothing else owns it, so `paths` now include `HttpHistoryPage.svelte`
  and `HttpHistoryPage.test.ts`. Add only the `'history.sendToReplay'` entry to `MENU_COMMANDS` plus a test that the
  context menu lists it; no other changes there.

## Plan (implementer, ≤15 lines)
> Question for lead: the History context menu is built from `MENU_COMMANDS` in
> `HttpHistoryPage.svelte` (T-038, outside `paths`). Widen `paths` by that file (plus its test) so I can add the one
> `'history.sendToReplay'` id, or have T-038's owner add it. Everything else is implemented; `make check` is green.

1. `session/session.graphql`: `ReplaySessionDetail` (connection+options+draft+entries with `...ReplayEntryMessage`), `UpdateReplaySession`, `SendReplaySession`, `CancelReplaySend`; `bun run codegen`.
2. `session/session.svelte.ts`: `SessionEditorState` — load, `shown` bytes, 1 s throttle save (`updateReplaySession`, `flush()` on send/entry switch/destroy), `saveSettings`, `send`/`cancel`, entries newest-first, `activeEntryId`.
3. `session/ConnectionBar.svelte`: scheme/host/port/SNI, Apply M&R, env selector (shared store `select`), ⚙ options panel (redirects, Content-Length, timeout).
4. `session/EntryNav.svelte`: `‹ n/N ›` + entry dropdown (error-marked entries).
5. `session/SessionEditor.svelte`: bar + Send/Cancel + `SplitPane` (`RawEditor` with env `variables`, `MessageViewer` response, status/size/roundtrip, error banner); registers `replay.send`.
6. `http-history/commands.ts`: `history.sendToReplay` via `openInReplay(row.id, state.client)`, first selected row.
7. Tests: `session/SessionEditor.test.ts` (autosave timing with fake timers, send/cancel, entry nav, env hover + selector), `http-history/commands.test.ts` (command + ⌘R creates a session and routes).

## Out-of-scope change needed (if any)
- **Context-menu item for `history.sendToReplay`** — the History row context menu is built from
  `MENU_COMMANDS` in `web/src/pages/http-history/HttpHistoryPage.svelte` (owned by T-038, not in this task's
  `paths`), not from `http-history/commands.ts`. The command is registered and works from the palette and from
  Ctrl/⌘+R; it only needs one id added to that list:
  `'history.sendToReplay'` as the first entry of `MENU_COMMANDS` (after the import-free ids, i.e. before
  `'history.copy.url'`). No other change is needed — `openMenu` already renders every listed id through the registry.
- **Question for lead:** either widen this task's `paths` with `web/src/pages/http-history/HttpHistoryPage.svelte`
  (and `HttpHistoryPage.test.ts` if a test is wanted) so I can add that one line and a test, or have the owner of
  T-038 add it. Everything else of T-107 is implemented and `make check` is green.

## Handoff (implementer)
- What changed:
  - `session/session.graphql` (new): `ReplaySessionDetail` (connection, options, draft, `sending`, newest 100
    entries with `...ReplayEntryRow`), `UpdateReplaySessionDraft`, `SendReplaySession`, `CancelReplaySend`;
    `web/src/lib/api/generated/**` regenerated with `bun run codegen`.
  - `session/session.svelte.ts` (new): `SessionEditorState` — load, `shown` bytes (draft or the entry being
    viewed), `edit()` scheduling at most one `updateReplaySession` per second, `flush()` on send / entry switch /
    unmount, `saveConnection`/`saveOptions`, `send`/`cancel`, `viewed`/`position`, and `entryTruncated` (an entry
    request past the 4 MiB body cap opens read-only instead of losing bytes).
  - `session/ConnectionBar.svelte` (new): scheme, host, port, SNI override, Apply M&R, environment selector (the
    shared store's `select`), and a ⚙ options panel (follow redirects, update Content-Length, timeout). Fields
    commit on blur/Enter/change; an invalid port or timeout is not sent (the server validates again).
  - `session/EntryNav.svelte` (new): `‹ n/N ›` plus a dropdown of entries, newest first, failures marked.
  - `session/SessionEditor.svelte`: replaces the T-034 placeholder — connection bar, Send (⌘/Ctrl+⏎ via the
    `replay.send` command, `aria-busy` while in flight) and Cancel, `SplitPane` with `RawEditor` (env
    `variables`, identity per entry) and `MessageViewer` for the response plus `code · size · roundtrip`; a failed
    entry shows `role="alert"` "Send failed: <error>" instead of a response.
  - `http-history/commands.ts`: `history.sendToReplay` — `openInReplay(firstSelectedId, state.client)`, so the
    existing Ctrl/⌘+R binding now runs it; a failure lands in the page's message line.
- How to verify:
  - `make check` (green here: 438 web unit tests, Go suite + lint).
  - `cd web && bun run dev`, open `#/replay` → a session: edit the request, wait ≥1 s, reload the tab — the edit is
    there; Send shows `Sending…`, then the response; Cancel on a slow target yields a `cancelled` entry banner.
  - History: select a row, press Ctrl/⌘+R → a Replay tab opens on a copy of it.
- Known limitations:
  - The History **context-menu** item is missing: its id list lives in `HttpHistoryPage.svelte` (T-038), outside
    this task's `paths` — see "Out-of-scope change needed". Ctrl/⌘+R and the palette work.
  - Only the newest 100 entries are loaded (`entries(first: 100)`); older ones need paging (not in M2's AC).
  - Entry navigation shows the entry's request *as stored*; a request body past the 4 MiB display cap makes the
    editor read-only for that entry (notice shown) rather than silently dropping bytes.
  - `?mock=replay` dev mode: the shell swaps in the mock client for its own state, but `SessionEditor` (mounted by
    `ReplayShell`, not in `paths`) has no client prop to receive it, so the editor's own queries go to the shared
    client. Component tests pass the mock explicitly.
  - No Playwright spec: `web/tests/e2e/**` is outside `paths` (M2 E2E is T-039).
  - Closing a tab mid-send leaves the send running server-side; the entry is not shown until the session is
    reopened.
- Follow-ups (proposed tasks): add `'history.sendToReplay'` to `MENU_COMMANDS` (owner of T-038 or a widened
  path); pass the shell's client into `SessionEditor` (or a shared `replayClient` context) so `?mock=replay` covers
  the editor; entry paging / "load older" once sessions grow past 100 sends; E2E in T-039 for send, cancel and
  Send-to-Replay.

## Review (reviewer)
- Verdict: approve | changes_requested
- Findings:
