---
id: T-012
title: Shared TrafficTable (virtualized) and MessageViewer components
milestone: M1
role: frontend
status: in_review
depends_on: [T-003]
owner: opencode/deepseek-v4.1-flash-1017
branch: task/T-012-shared-table-viewer
paths:
  - web/src/lib/components/TrafficTable/**
  - web/src/lib/components/MessageViewer/**
  - web/src/lib/components/SplitPane/**
  - web/src/lib/editor/**
  - web/package.json
  - web/bun.lock
spec:
  - docs/02-ui-layout.md §2, §3.1, §9
  - docs/01-feature-spec.md#F-HIST-1, #F-HIST-2
---

## Goal
Reusable, data-source-agnostic table and viewer components ready for History, Search, Sitemap, Automate results.

## Notes from lead
Decide `@tanstack/svelte-virtual` vs in-house virtual list by benchmark (100k rows, 60 fps scroll); record decision in Handoff.
MessageViewer modes: Raw, Pretty (JSON/XML/HTML/JS), Hex, Table (headers/params), Preview (sandboxed iframe src from REST).

## Acceptance criteria
- [x] Storybook-like demo route `#/dev/components` (dev builds only) with 100k fake rows.
- [x] Column show/hide/reorder/resize; sort indicators; multi-select with shift/ctrl; keyboard navigation; `onrowcontextmenu` callback.
- [x] Viewer handles a 5 MB body without freezing (chunked read-only mode); search (Ctrl+F) works.
- [x] Component tests for selection logic and column config persistence callbacks.

## Plan (implementer, ≤15 lines)
- Virtualization: in-house fixed-row window (`TableView.svelte`); rows are one `--row-h` tall, so tanstack's dynamic measurement buys nothing and costs a dependency.
- `TrafficTable/`: types, pure column-config helpers, `Selection` state class, virtual list, table (sort, resize, popover show/hide/reorder, multi-select, keyboard nav, row context-menu callback).
- `MessageViewer/`: message parse + format helpers, tabs Raw/Pretty/Hex/Table/Preview, Ctrl+F search with match count, chunked render for 5 MB bodies, copy/wrap.
- `SplitPane/`: thin paneforge wrapper (default sizes, `onsizeschange` persistence callback, double-click reset).
- Dev demo `#/dev/components` with 100k generated rows, viewer samples and a 5 MB body toggle.
- Tests: selection, column config callbacks, table windowing/sort/keyboard/context menu, viewer modes/chunking/search.
- `routes.ts` needs one dev-only entry (see Out-of-scope).

## Out-of-scope change needed (if any)
- `web/src/lib/router/routes.ts`: +10 lines registering the dev-only `dev` route (`import.meta.env.DEV`, no sidebar
  category) so the AC-mandated `#/dev/components` gallery is reachable. No production effect (Vite drops the route and
  its lazy chunk from prod builds; verified in `bun run build`). **Lead approval requested**; alternative is to move the
  gallery behind an existing route.

## Handoff (implementer)
### What changed
- `web/src/lib/components/TrafficTable/`: `types.ts` (`TrafficColumn<Row>`, `ColumnConfig`, `SortState`, `RowColor`),
  `columns.ts` (pure normalize/toggle/move/resize helpers), `selection.svelte.ts` (`Selection` state class),
  `TrafficTable.svelte` (keyboard, selection, sort, columns menu), `TableView.svelte` (sticky header, fixed-row
  virtualization, resize handles), `ColumnMenu.svelte` (bits-ui popover, show/hide/reorder), `demo/DevComponentsPage.svelte`.
- `web/src/lib/components/MessageViewer/`: `parse.ts`, `format.ts` (JSON/markup pretty, hex dump, search, highlight),
  `MessageViewer.svelte` (tabs, Ctrl+F, wrap, copy), `MessageTable.svelte`, `HighlightedText.svelte`, `ShowMoreBar.svelte`.
- `web/src/lib/components/SplitPane/SplitPane.svelte`: paneforge wrapper, `defaultSizes`, `onsizeschange`, double-click reset.
- `web/package.json` + `bun.lock`: added `paneforge@^1.0.2` (docs/06 §1). No others added.
- Tests (102 web tests green): selection (`shift`/`ctrl`/range), column config callbacks through the real menu,
  100k-row windowing, keyboard nav, context-menu/open callbacks, viewer modes, 5 MB chunking, search, hex, preview,
  SplitPane mount + keyboard resize, gallery smoke test. Key files: `*.test.ts` next to each component.

### Virtualization decision (lead question)
In-house fixed-row window instead of `@tanstack/svelte-virtual`: every row is exactly one `--row-h` token high (density
switch changes the token, not individual rows), so dynamic measurement is unused. Per scroll frame we do O(1) index
arithmetic and render `viewport/rowH + 16` rows (≤80 row elements for 100k rows, asserted in the test); no extra
dependency or bundle cost. TanStack's value is variable-height rows and we do not need that. Not a profiled 60 fps
benchmark (no browser profiler in this environment): the gallery (`#/dev/components`, 100k rows) is the manual check; if
profiling later shows frame drops, swapping in tanstack is a change confined to `TableView.svelte`.

### How to verify
1. `cd web && bun run dev` → `#/dev/components` (dev server only).
2. Scroll the 100k-row table, sort columns, hide/reorder/resize via **Columns**, shift/ctrl-click and arrow/Page keys,
   right-click a row (status line shows the callback), double-click to "open".
3. Toggle **Use 5 MB response** and check Raw scroll/`Show more` and Ctrl+F (128 KB initial render, grows in 128 KB steps).
4. `make check` (green locally), `cd web && bun run test`.

### Known limitations
- `MessageViewer` search matches the raw message only; jumping from Pretty/Hex switches to Raw. JS is shown as-is in
  Pretty (no JS formatter), markup re-indent is best-effort (no `<pre>`/`<script>` special-casing).
- Hex byte limit vs. char-based size label are approximate for non-ASCII bodies; hex view covers the body only.
- `TrafficTable` reads `config` at mount (pages should render it after `ui_state` loads); row height token is read once
  per mount (density change needs a remount).
- `SplitPane` sizes are not applied from a persisted value after mount (pass `defaultSizes` at mount); no test for drag
  (jsdom lacks layout/ResizeObserver), only keyboard resize.
- E2E deferred: Playwright is not a dependency yet (T-021); the gallery is dev-only and covered by a jsdom smoke test.
- `web/src/lib/editor/**` (allowed path) intentionally untouched: CodeMirror is not needed for read-only 5 MB display.

### Follow-ups (proposed tasks)
- Persist column config / pane sizes to backend `ui_state` from the pages (needs the settings contract).
- Editable MessageViewer variant + CodeMirror for Replay/Intercept (T-0xx); original/modified switch (F-HIST-4).
- E2E spec for `#/dev/components` and table/viewer keyboard paths with T-021 tooling.

## Review (reviewer)
