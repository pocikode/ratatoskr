---
id: T-104
title: RawEditor - byte-preserving editable HTTP editor with {{var}} highlighting
milestone: M2
role: frontend
status: in_review
depends_on: []
owner: "claude-sonnet-5.5"
branch: task/T-104-raw-editor
paths:
  - web/src/lib/editor/**
  - web/package.json   # add @codemirror/search only (approved family @codemirror/*, docs/06)
  - web/bun.lock
spec:
  - docs/01-feature-spec.md#F-REP-2 (editor bytes rule), #F-UI-6
  - docs/02-ui-layout.md §2, §3.4, §3.7
  - docs/06-frontend-conventions.md (editor/ folder, runes, CodeMirror via $effect)
---

## Goal
One editable CodeMirror component for raw HTTP messages, shared by Intercept (T-032) and Replay (T-107), that never
changes bytes the user did not edit.

## Scope
- In: `web/src/lib/editor/RawEditor.svelte` with props `value: Uint8Array`, `onchange(bytes: Uint8Array)`,
  `readonly?`, `variables?: Record<string, {value: string; secret: boolean}>`; HTTP language (start line, header names,
  values, body); `{{var}}` decorations (known vs unknown) with hover showing the value (secret masked as `••••`);
  Ctrl/⌘+F search (`@codemirror/search`), wrap toggle; bytes ↔ text helpers in `web/src/lib/editor/bytes.ts`.
- Out: Pretty/Hex/Table modes (the response side keeps `MessageViewer`), convert workflows on selection (M5), large-message
  virtualization (T-085).

## Acceptance criteria
- [ ] Round-trip unit tests: bytes with CRLF, lone LF, lone CR, a trailing `\r\n\r\n` and an empty body come back
      byte-identical when nothing is edited.
- [ ] Enter inserts `\r\n` (test on the emitted bytes).
- [ ] Input that is not valid UTF-8 renders read-only with a visible notice and emits nothing.
- [ ] `{{name}}` and `{{ name }}` are decorated; known/unknown use different classes; hover text shows the value or the mask.
- [ ] Component test: typing emits `onchange` with the new bytes; `readonly` blocks edits.
- [ ] `bun run check` / unit tests green (`make check`).

## Notes from lead
- CodeMirror normalises line breaks by default. Keep the document lossless (e.g. a custom `lineSeparator` plus a
  transaction filter, or map CR/LF explicitly) and prove it with the round-trip tests above; do not ship a "close enough" mapping.
- Runs in parallel with every other M2 task (own folder only).

## Plan (implementer, ≤15 lines)
- Doc = UTF-8 text with CR/LF kept as characters (`lineSeparator "\n"`, CR not flagged as special char) so decode→encode is identity; Enter inserts CRLF.
- Invalid UTF-8: lossy display, read-only, notice, no emit. External `value` pushed in with an annotation so it is not echoed.
- `{{var}}` via ViewPlugin decorations + `title` hover; HTTP StreamLanguage; search via @codemirror/search; Wrap button.

## Out-of-scope change needed (if any)

## Handoff (implementer)
- What changed: `web/src/lib/editor/{bytes.ts,http.ts,RawEditor.svelte}` + tests; added `@codemirror/search`.
- How to verify: `make check`; `bunx vitest run src/lib/editor`.
- Known limitations: hover uses native `title` (not a styled tooltip); wrap toggle is a button, not a registry command (no consumer page yet); cursor can sit between CR and LF; no Playwright spec (component not mounted in a page until T-032/T-107).
- Review round 2: all 5 findings fixed (history reset on prop replace, CRLF-aware Backspace/Delete + Shift-Enter, getClientRects shim, blankLine body phase, mixed-ending component test). `make check` green.
- Review round 3: CRLF-aware cursor (transaction filter keeps caret out of CR|LF); new optional `identity` prop resets doc+history on message change (consumers T-032/T-107 should pass the message id); Mod-Enter removed from editor keymap; lexer test uses `\n` separator. Also fixed editor being rebuilt on prop updates (`new EditorView` now untracked), which had made earlier undo tests vacuous.
- Follow-ups (proposed tasks):

## Review (reviewer)
- Reviewed `6643ecd` against `main`. Assumed one operator editing a message in the browser, with the documented 5 MB editor budget.
- Verdict: changes_requested
- Findings:
  1. [x] [major] web/src/lib/editor/RawEditor.svelte:126 — Replacing the `value` prop records an undo event. After loading a second message in the same editor, Ctrl/⌘+Z restores and emits the first message under the current selection. Reset editor history when genuinely replacing the document and test that undo after a prop change cannot emit the old bytes.
  2. [x] [major] web/src/lib/editor/RawEditor.svelte:91 — The default editing keys see CRLF as two separate characters. Backspace at the start of the next line deletes only LF and leaves a hidden CR. Shift+Enter also falls through to CodeMirror and inserts a bare LF. Treat CRLF as one editing boundary, bind Shift+Enter to the CRLF command, and assert emitted bytes for both cases.
  3. [x] [major] web/src/lib/editor/RawEditor.test.ts:21 — CodeMirror measurement can run while the test editor is mounted, and jsdom lacks the range geometry API it calls. Both the full web run and a focused rerun reported `TypeError: textRange(...).getClientRects is not a function` from this file. Provide the missing range geometry in the component test setup, as in `web/src/pages/filters/FiltersPage.test.ts:8`.
  4. [x] [minor] web/src/lib/editor/http.ts:27 — For LF-only messages CodeMirror invokes `blankLine` at the separator instead of calling `token`. A body line like `name: body` then receives header highlighting. Add a `blankLine` handler that moves the lexer into the body and test LF and CRLF against the same message.
  5. [x] [minor] web/src/lib/editor/bytes.test.ts:16 — The round-trip tests only pass data through TextDecoder and TextEncoder. Removing the editor's explicit LF separator could normalize lone CR and still pass those tests. Add a component test that edits one character in a message with mixed endings and checks that the remaining bytes are identical.
- Verification: `make check` passed Go lint, race tests, and frontend static checks after retrying with cache access. Vitest failed with the editor geometry error above and `Error: Test timed out in 5000ms.` in `SitemapPage.test.ts:33` and `sitemap.svelte.test.ts:132`. The targeted rerun passed all 17 assertions and reproduced the editor geometry error.
- Browser E2E and 5 MB interaction timings were outside the checks run for this review.

### Re-review of `78e42bb`

- Reviewed the current branch against `main`, including the follow-up that landed during review. Assumed one operator editing messages up to 5 MB.
- Verdict: changes_requested
- The follow-up fixes blocked undo after loading different bytes and cleared the jsdom geometry error. The mixed-ending component test passed. Chromium confirmed that both LF and CRLF blank lines end header highlighting.
- [x] [major] web/src/lib/editor/RawEditor.svelte:78 — Chromium's End key lands between CR and LF. From the first line of `a\r\nb`, End then Delete emits `a\rb`, and End then typing `x` emits `a\rx\nb`. Keep the cursor out of the middle of CRLF, make deletion consume it as one boundary, and test editing from the visual end of the line.
- [x] [major] web/src/lib/editor/RawEditor.svelte:152 — A byte-identical replacement keeps the previous message's undo stack. In Chromium, editing `a` to `ax`, loading a different message with bytes `ax`, then undoing emits `a` for the newly selected message. Tie the editor's lifetime or history reset to message identity and test both same-byte selection changes and ordinary parent echoes.
- [x] [major] web/src/lib/editor/RawEditor.svelte:123 — CodeMirror's default Mod+Enter binding inserts a blank line and consumes the app's Send and Forward shortcut. Chromium on macOS emitted changed bytes and the window received a prevented event, which `ShortcutManager` ignores. Filter the Mod+Enter binding out of the editor keymap.
- [x] [minor] web/src/lib/editor/http.test.ts:7 — The lexer test uses CodeMirror's default newline handling, so both cases reach the parser as LF. Set `EditorState.lineSeparator.of('\n')`, as RawEditor does, to exercise the CRLF path with its retained CR.
- Verification: The final `make check` passed, including all 248 web tests. The earlier run stopped at Prettier's `[warn] src/lib/editor/RawEditor.svelte`, and a separate web test run reported `Error: Test timed out in 5000ms.` at `src/pages/sitemap/sitemap.svelte.test.ts:132`. The format fix and the final gate cleared both failures.
- Browser checks mounted the component in Chromium. The app's Playwright suite and 5 MB interaction timing were outside this review.
