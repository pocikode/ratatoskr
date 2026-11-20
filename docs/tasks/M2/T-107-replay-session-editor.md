---
id: T-107
title: Replay session editor - connection bar, request editor, send/cancel, entry history, response; Send to Replay from History
milestone: M2
role: frontend
status: in_review
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
- [x] From History, ⌘/Ctrl+R and the context-menu item create a session from the selected row and open its tab.
- [x] Component tests against the mock; `make check` green.

## Notes from lead
- `historyCommands` must not need new deps from `HttpHistoryPage.svelte` (owned by T-038): import `openInReplay` directly.

- Lead answer (2026-11-20): T-038 is done and nothing else owns it, so `paths` now include `HttpHistoryPage.svelte`
  and `HttpHistoryPage.test.ts`. Add only the `'history.sendToReplay'` entry to `MENU_COMMANDS` plus a test that the
  context menu lists it; no other changes there.

## Plan (implementer, ≤15 lines)
1. `session/session.graphql`: `ReplaySessionDetail` (connection+options+draft+entries with `...ReplayEntryMessage`), `UpdateReplaySession`, `SendReplaySession`, `CancelReplaySend`; `bun run codegen`.
2. `session/session.svelte.ts`: `SessionEditorState` — load, `shown` bytes, 1 s throttle save (`updateReplaySession`, `flush()` on send/entry switch/destroy), `saveSettings`, `send`/`cancel`, entries newest-first, `activeEntryId`.
3. `session/ConnectionBar.svelte`: scheme/host/port/SNI, Apply M&R, env selector (shared store `select`), ⚙ options panel (redirects, Content-Length, timeout).
4. `session/EntryNav.svelte`: `‹ n/N ›` + entry dropdown (error-marked entries).
5. `session/SessionEditor.svelte`: bar + Send/Cancel + `SplitPane` (`RawEditor` with env `variables`, `MessageViewer` response, status/size/roundtrip, error banner); registers `replay.send`.
6. `http-history/commands.ts`: `history.sendToReplay` via `openInReplay(row.id, state.client)`, first selected row.
7. Tests: `session/SessionEditor.test.ts` (autosave timing with fake timers, send/cancel, entry nav, env hover + selector), `http-history/commands.test.ts` (command + ⌘R creates a session and routes).

## Out-of-scope change needed (if any)
- None. The History context-menu id landed with the lead's widened `paths` (`HttpHistoryPage.svelte`).

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
  - `http-history/HttpHistoryPage.svelte`: `'history.sendToReplay'` added as the first `MENU_COMMANDS` id, so the
    row context menu offers it (lead widened `paths` for this one line).
- How to verify:
  - `make check` (green here: 439 web unit tests, Go suite + lint).
  - `cd web && bun run dev`, open `#/replay` → a session: edit the request, wait ≥1 s, reload the tab — the edit is
    there; Send shows `Sending…`, then the response; Cancel on a slow target yields a `cancelled` entry banner.
  - History: select a row, press Ctrl/⌘+R or pick "Send to Replay" from its context menu → a Replay tab opens on a
    copy of it.
- Known limitations:
  - Only the newest 100 entries are loaded (`entries(first: 100)`); older ones need paging (not in M2's AC).
  - Entry navigation shows the entry's request *as stored*; a request body past the 4 MiB display cap makes the
    editor read-only for that entry (notice shown) rather than silently dropping bytes.
  - `?mock=replay` dev mode: the shell swaps in the mock client for its own state, but `SessionEditor` (mounted by
    `ReplayShell`, not in `paths`) has no client prop to receive it, so the editor's own queries go to the shared
    client. Component tests pass the mock explicitly.
  - No Playwright spec: `web/tests/e2e/**` is outside `paths` (M2 E2E is T-039).
  - Closing a tab mid-send leaves the send running server-side; the entry is not shown until the session is
    reopened.
- Follow-ups (proposed tasks): pass the shell's client into `SessionEditor` (or a shared `replayClient` context) so
  `?mock=replay` covers the editor; entry paging / "load older" once sessions grow past 100 sends; E2E in T-039 for
  send, cancel and Send-to-Replay.

## Review (reviewer)
- Verdict: changes_requested
- Reviewer: sonnet (round 1, 5ca11b8; written by lead, reviewer has no edit tool)
- Findings:
  - [ ] [major] web/src/pages/replay/session/session.svelte.ts:189 — `send()` passes `this.connection` / `this.options`
    from the `ReplaySessionDetail` result as `ReplayConnectionInput` / `ReplayOptionsInput`. urql's cacheExchange
    (client.ts:53) adds `__typename` to every selection set, so a real server rejects the input
    (`Field "__typename" is not defined by type ReplayConnectionInput`). Mocks build objects without `__typename`, so no
    test covers it. → Build inputs explicitly (`{host, port, tls, sni}` and the four option fields); add a test whose
    mocked session includes `__typename` and assert the mutation variables do not.
  - [ ] [minor] web/src/pages/replay/session/session.svelte.ts:175 — `...(input.draft && …)` skips the local mirror when
    the draft is `""`, so `session.draft` goes stale after clearing the editor. → `input.draft !== undefined`.
  - [ ] [minor] web/src/pages/replay/session/session.svelte.ts:230 — `dispose()` runs an async `flush()` on unmount; a
    failed final save only sets `error` on a gone state object (silent). → acceptable for M2: note it as a limitation,
    or retry once.
  - [ ] [nit] web/src/pages/replay/session/SessionEditor.svelte:57 — `size()` has B and KB only; add MB or reuse the
    History formatter.
- Checked, no findings: scope inside paths (widened HttpHistoryPage one-line change; codegen output authorized); every
  AC has tests; truncated entries open read-only with Send disabled; no hard-rule violations.
- Verification: `make check` green (0 lint issues, Go tests, svelte-check 0 errors, prettier, 439 vitest tests).
  `make test-int` not run (no Go proxy/store/rawhttp changes). Not run against a real server.
