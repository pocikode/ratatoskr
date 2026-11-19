---
id: T-104
title: RawEditor - byte-preserving editable HTTP editor with {{var}} highlighting
milestone: M2
role: frontend
status: todo
depends_on: []
owner: ""
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

## Out-of-scope change needed (if any)

## Handoff (implementer)
- What changed:
- How to verify:
- Known limitations:
- Follow-ups (proposed tasks):

## Review (reviewer)
- Verdict: approve | changes_requested
- Findings:
