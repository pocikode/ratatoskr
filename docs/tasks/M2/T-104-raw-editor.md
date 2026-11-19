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
- [ ] Line-ending rule (F-REP-2, added at the lead escalation): in `a\r\nb`, `a\nb` and `a\rb`, End, Shift+End,
      Backspace, Shift+Backspace, Delete, Alt+Up/Down and Mod+Backspace emit bytes in which every untouched line break
      keeps its original bytes and every new one is CRLF. Undo after deleting an LF or CR break restores the exact bytes.
- [ ] Mod+Enter reaches a `window` listener with `defaultPrevented === false` and emits nothing.

## Notes from lead
- CodeMirror normalises line breaks by default. Keep the document lossless (e.g. a custom `lineSeparator` plus a
  transaction filter, or map CR/LF explicitly) and prove it with the round-trip tests above; do not ship a "close enough" mapping.
- Runs in parallel with every other M2 task (own folder only).
- **Escalation decision (lead, 2026-11-19, after review round 3).** Three rounds patched one CodeMirror command at a
  time (End, Backspace, Shift+Backspace, line move…) because CR lives inside the line as an ordinary character. Every
  default command, mouse path and platform key will keep finding the same hole. Change the representation:
  - Drop `lineSeparator.of('\n')`, `crlfCursor`, `eraser` and the CR special-char handling. Use CodeMirror's default
    splitting (`/\r\n?|\n/`), so every break is one `\n` in the document and stock commands are correct by
    construction.
  - Keep the original terminator out of band in a `StateField<RangeSet>` in `web/src/lib/editor/lineEndings.ts`.
    Place a marker only for non-CRLF breaks (`lf`/`cr`) at `line.to`. Map it through changes and drop markers inside
    changed ranges, filtered to the touched lines to keep keystrokes O(change), not O(lines). A break without a marker
    is CRLF, which gives Enter, paste and line commands CRLF for free.
  - Undo: register `invertedEffects` (from `@codemirror/commands`) that re-add the markers a transaction removed, with a
    `map` on the effect. Encode = walk lines + markers → bytes. Decode builds markers with the same regex.
  - Enter/Shift+Enter: bind `insertNewline` (no auto-indent, which would add or strip whitespace bytes). Keep the
    Mod-Enter filter.
  - Unit-test `lineEndings.ts` without a view (round-trip, edits, undo). Component tests then only cover wiring and
    the AC key list above. Moving the logic out also resolves the 250-line finding.
- Review budget: the lead authorises **one more review round (round 4)** on this redesign. If it still fails, the lead
  takes the task back.

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
- Round 4 (lead redesign): `lineEndings.ts` keeps the doc on default splitting (every break = `\n`) and stores lf/cr terminators out of band in a RangeSet (filtered to touched ranges, mapped through changes, `invertedEffects` restores them on undo). Removed `lineSeparator`, `crlfCursor`, `eraser`, CR special-char handling; Enter/Shift+Enter = `insertNewline`; Mod-Enter still filtered. Component is now 207 lines. Unit tests (no view) in `lineEndings.test.ts`; component tests cover the AC key list for CRLF/LF/CR and Mod+Enter reaching a window listener unprevented.
- Round 4 limitation: Alt+Up/Down test is loose (jsdom has no layout); lines moved by a line command lose their lone LF/CR and become CRLF (new break rule).
- Round 5 (findings of b22ec54): echo detection compares exact last loaded/emitted bytes before any parsing; read-only content has `tabindex=0`; Enter/Shift+Enter guarded by `readOnly`; line-move tested both directions at state level; Mod+Enter test records only the Ctrl+Enter keydown (exactly one, unprevented); new tests for invalid-lossy undo and read-only focus.
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

### Re-review of `f00c0d7`

- Verdict: `changes_requested`. This is the third review round. Escalate the remaining CRLF editing defects to the lead under `docs/07-agent-workflow.md` §7.
- Reviewed `main...f00c0d7` for one operator editing messages up to 5 MB. The product changes are within the task paths. The new tests cover identity changes, parent echoes, and the lexer’s retained-CR path. Chromium confirmed ordinary End followed by typing or Delete works from the start of a line and that both Ctrl+Enter and Cmd+Enter bubble without editing.
- [x] [major] web/src/lib/editor/RawEditor.svelte:104 — The cursor filter uses the old position to infer rightward motion. Starting at the beginning of `a\r\nb`, Right then End then `x` emits `a\r\nxb`. Right then Shift+End then `x` emits `axb`, deleting the separator. Give End and Shift+End an explicit CRLF-aware line-end target and test them with the caret already at the visual end.
- [x] [major] web/src/lib/editor/RawEditor.svelte:152 — Shift+Backspace bypasses the custom eraser. At the start of the second line in `a\r\nb`, it emits `a\rb`. Bind Shift+Backspace to the same eraser and assert the emitted bytes.
- [x] [major] web/src/lib/editor/RawEditor.svelte:157 — The default line-move command treats CR as part of the line and reinserts LF as the separator. Alt+Down on the first line of `a\r\nb` emits `b\na\r`. Drop the optional line-move bindings and test that the shortcut leaves the bytes intact.
- [x] [minor] web/src/lib/editor/RawEditor.test.ts:136 — The Mod+Enter test only checks for emitted bytes. An editor handler that calls `preventDefault()` without editing would pass, but the app would skip Send and Forward. Assert that the key reaches a window listener with `defaultPrevented` false.
- [x] [minor] web/src/lib/editor/RawEditor.svelte:101 — The component is 270 lines, above the 250-line limit in `docs/06-frontend-conventions.md` §3. Use the CRLF fixes to move the keyboard handling into a module beside the component.
- **Lead disposition (escalation):** findings 1–3 are symptoms of CR-in-line and are superseded by the
  representation change in *Notes from lead*. They stay open until round 4 verifies the listed keys against the new AC.
  Findings 4–5 stand as written.
- Verification: `make check` passed, including all 253 web tests. Chromium reproduced the three open editing defects. A local Chromium smoke check loaded two 5,000,000-byte fixtures, one multiline and one with a single-line body. Both emitted 5,000,001 bytes after typing one character.
- The app’s Playwright suite and cross-browser behavior were outside this review. The component has no route-level consumer on this branch.

### Re-review of `b22ec54`

- Verdict: `changes_requested`. This closes the lead-authorized fourth round. Return T-104 to the lead under the escalation decision above.
- Reviewed `main...b22ec54` for one operator editing a message up to 5 MB. Source and dependency changes are within the task paths. The branch also includes the task record and the lead's F-REP-2 amendment to `docs/01-feature-spec.md`, outside the listed frontend paths.
- [x] [major] web/src/lib/editor/RawEditor.svelte:119 — The prop effect treats different bytes as a parent echo when an invalid replacement has the same lossy display. In Chromium, replace `a` with `�`, load `[0xff]`, then load the valid UTF-8 bytes for `�` under the same optional identity. Undo then emits `a` for the replacement. Compare against the exact loaded or last-emitted bytes and reset history on a genuine replacement. Perform this check before parsing: five local Chromium edits of a 5 MB LF-delimited fixture took 211–233 ms each with a parent echo, compared with 69–74 ms without one. The echo currently reparses the bytes and re-encodes the document to decide it has nothing to do.
- [x] [major] web/src/lib/editor/RawEditor.svelte:87 — Read-only content cannot receive focus by click or Tab, so Cmd+F after clicking it does not open CodeMirror search in Chromium. Give read-only content a tab stop, and guard Enter and Shift+Enter with `state.readOnly`. With a tab stop added in the browser, `insertNewline` changed `ab` to `ab\n\n` on those two keys despite the lock, while the callback emitted nothing. Test both search and the unchanged document while locked.
- [x] [minor] web/src/lib/editor/RawEditor.test.ts:114 — The line-move test runs only Alt+Down and accepts zero emissions. It passes with either shortcut broken or removed and cannot detect changes to a neighboring break. Run both directions from a movable line in a three-line mixed-ending document, and assert the emitted sequence including the untouched break. CodeMirror's line-move commands can also be exercised without layout through the state tests.
- [x] [minor] web/src/lib/editor/RawEditor.test.ts:166 — The listener records all keydowns. The Ctrl keydown alone satisfies the assertion if the Enter keydown is swallowed without editing. Record only the modified Enter event and require exactly one visit with `defaultPrevented === false`.
- Verification: `make check` passed, including 279 web tests. A standalone Chromium run verified End, Shift+End, Backspace, Shift+Backspace, Delete, Mod+Backspace, both line-move directions, and undo/redo with CRLF, LF and CR. It also verified that Ctrl+Enter and Cmd+Enter reached `window` unprevented with no emitted bytes. The app's Playwright suite and other browser engines were outside the checks run. The component has no route-level caller on this branch.
