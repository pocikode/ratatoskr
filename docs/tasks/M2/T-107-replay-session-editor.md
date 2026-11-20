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
- [ ] Send calls `sendReplaySession` with the editor bytes, connection and options; button shows progress; Cancel calls
      `cancelReplaySend`; a cancelled/failed entry shows its error.
- [ ] Draft autosave: rapid typing produces ≤ 1 `updateReplaySession` per second; pending changes are saved before send
      and when switching tabs (fake timers test).
- [ ] Entry navigation loads that entry's request into the editor and its response into the viewer; editing afterwards
      keeps the edit as the draft.
- [ ] `{{var}}` hover shows the selected environment's value (secret masked); the env selector changes the project selection.
- [ ] From History, ⌘/Ctrl+R and the context-menu item create a session from the selected row and open its tab.
- [ ] Component tests against the mock; `make check` green.

## Notes from lead
- `historyCommands` must not need new deps from `HttpHistoryPage.svelte` (owned by T-038): import `openInReplay` directly.

## Plan (implementer, ≤15 lines)
1. `session/session.graphql`: `ReplaySessionDetail` (connection+options+draft+entries with `...ReplayEntryMessage`), `UpdateReplaySession`, `SendReplaySession`, `CancelReplaySend`; `bun run codegen`.
2. `session/session.svelte.ts`: `SessionEditorState` — load, `shown` bytes, 1 s throttle save (`updateReplaySession`, `flush()` on send/entry switch/destroy), `saveSettings`, `send`/`cancel`, entries newest-first, `activeEntryId`.
3. `session/ConnectionBar.svelte`: scheme/host/port/SNI, Apply M&R, env selector (shared store `select`), ⚙ options panel (redirects, Content-Length, timeout).
4. `session/EntryNav.svelte`: `‹ n/N ›` + entry dropdown (error-marked entries).
5. `session/SessionEditor.svelte`: bar + Send/Cancel + `SplitPane` (`RawEditor` with env `variables`, `MessageViewer` response, status/size/roundtrip, error banner); registers `replay.send`.
6. `http-history/commands.ts`: `history.sendToReplay` via `openInReplay(row.id, state.client)`, first selected row.
7. Tests: `session/SessionEditor.test.ts` (autosave timing with fake timers, send/cancel, entry nav, env hover + selector), `http-history/commands.test.ts` (command + ⌘R creates a session and routes).

## Out-of-scope change needed (if any)

## Handoff (implementer)
- What changed:
- How to verify:
- Known limitations:
- Follow-ups (proposed tasks):

## Review (reviewer)
- Verdict: approve | changes_requested
- Findings:
