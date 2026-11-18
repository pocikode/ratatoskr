---
id: T-012
title: Shared TrafficTable (virtualized) and MessageViewer components
milestone: M1
role: frontend
status: changes_requested
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
  - web/src/lib/router/routes.ts  # dev-only gallery route block only (lead, 2026-11-18)
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
- [ ] Storybook-like demo route `#/dev/components` (dev builds only) with 100k fake rows. — component and test done;
  the route registration is **blocked** (see Out-of-scope).
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

## Rework plan (review round 1)
- `MessageTable`: unkeyed header loop (duplicate headers crashed Svelte); params parsed from the first 128 KB and
  rendered up to 500 with a "more" note; table tab only mounted while selected.
- `TrafficTable`: ignore key events from header/resize buttons; ArrowUp/Down from no active row land on row 0;
  Ctrl/Cmd+A prevents the browser text selection.
- `TableView`: resolve `color(row)` once and only style the stripe for a real colour.
- `columns`: tolerate a stored config without `widths`, de-duplicate `hidden`; `parse`: split the query at the first `?`.
- `format`: highlight the window of matches around `current`; hex-encode only the shown prefix.
- Fix the SplitPane resize test (paneforge fires on mount) and add tests for every fix above.

## Out-of-scope change needed (if any)
- `web/src/lib/router/routes.ts`: +10 lines registering the dev-only `dev` route (`import.meta.env.DEV`, no sidebar
  category) so the AC-mandated `#/dev/components` gallery is reachable. No production effect (Vite drops the route and
  its lazy chunk from prod builds; re-verified with `bun run build`: 0 matches for "Component gallery" in `dist/`).
  **Blocked (review round 1).** The change was committed without the path being in `paths`, so it has been reverted.

  **Question for the lead:** add `web/src/lib/router/routes.ts` to this task's `paths` (the 10-line dev-only block is
  ready to restore), or route the gallery another way (e.g. behind an existing page) — or drop the AC. Until then
  `DevComponentsPage.svelte` + its smoke test exist but nothing links to them.

  **Lead decision (2026-11-18): approved.** `web/src/lib/router/routes.ts` is added to `paths`, limited to the
  reverted 10-line `if (import.meta.env.DEV) { routes.push(...) }` block at the end of the file. Do not change existing
  routes or `CATEGORIES`. The `bun run build` check (no gallery string in `dist/`) stays required and goes in the
  Handoff. No other open task owns this file (T-003 is done). Restore the block, tick the gallery AC, run `make check`,
  then set `in_review`.

## Handoff (implementer)
### Review round 1 — responses
- Blocker `routes.ts`: reverted (`git checkout main -- web/src/lib/router/routes.ts`), not self-approved. Task set to
  `blocked` with the question in Out-of-scope. The gallery component + test stay, nothing links to them.
- Blocker `MessageTable` duplicate keys: fixed — header rows key by index; test with two identical `Set-Cookie` lines.
- Major Table tab: mounted only while selected (`{#if mode === 'table'}`), params parsed from the first 128 KB of the
  body, max 500 rows + "N more params not shown". Test: 5 MB urlencoded body renders < 600 `<tr>`.
- Major key bubbling: `if (e.target !== e.currentTarget) return;` in the grid handler; test: Enter on a header button
  does not call `onrowopen` or select.
- Major row stripe: `{@const stripe = color?.(row)}`, only a real colour sets `border-left-color`.
- Minors fixed: ArrowUp/Down from no active row land on row 0; Ctrl/Cmd+A prevents text selection; `normalizeConfig`
  tolerates a missing `widths` map and de-duplicates `hidden`; search highlights the window around `current`
  (200 marks) so jumping past the cap still lands; `queryParams` splits at the first `?` only; `hexDump` encodes just
  the shown prefix (`body.slice(0, maxBytes + 1)`); `SplitPane` test clears the mount layout and asserts the keyboard
  step moves the first pane past 55 %; `selection?.has` evaluated once per row.
- `web/src/lib/components/MessageViewer/MessageTable.svelte` header loop keeps a key (eslint `require-each-key`) —
  index key, which is stable for a list that never reorders.

### What changed
- `web/src/lib/components/TrafficTable/`: `types.ts` (`TrafficColumn<Row>`, `ColumnConfig`, `SortState`, `RowColor`),
  `columns.ts` (pure normalize/toggle/move/resize helpers), `selection.svelte.ts` (`Selection` state class),
  `TrafficTable.svelte` (keyboard, selection, sort, columns menu), `TableView.svelte` (sticky header, fixed-row
  virtualization, resize handles), `ColumnMenu.svelte` (bits-ui popover, show/hide/reorder), `demo/DevComponentsPage.svelte`.
- `web/src/lib/components/MessageViewer/`: `parse.ts`, `format.ts` (JSON/markup pretty, hex dump, search, highlight),
  `MessageViewer.svelte` (tabs, Ctrl+F, wrap, copy), `MessageTable.svelte`, `HighlightedText.svelte`, `ShowMoreBar.svelte`.
- `web/src/lib/components/SplitPane/SplitPane.svelte`: paneforge wrapper, `defaultSizes`, `onsizeschange`, double-click reset.
- `web/package.json` + `bun.lock`: added `paneforge@^1.0.2` (docs/06 §1). No others added.
- Tests (110 web tests green): selection (`shift`/`ctrl`/range), column config callbacks through the real menu,
  100k-row windowing, keyboard nav (incl. no-active-row start and header-button keys), context-menu/open callbacks,
  duplicate headers, viewer modes, 5 MB chunking, sampled 5 MB form params, search, hex, preview, SplitPane mount +
  keyboard resize, gallery smoke test. Key files: `*.test.ts` next to each component.

### Virtualization decision (lead question)
In-house fixed-row window instead of `@tanstack/svelte-virtual`: every row is exactly one `--row-h` token high (density
switch changes the token, not individual rows), so dynamic measurement is unused. Per scroll frame we do O(1) index
arithmetic and render `viewport/rowH + 16` rows (≤80 row elements for 100k rows, asserted in the test); no extra
dependency or bundle cost. TanStack's value is variable-height rows and we do not need that. Not a profiled 60 fps
benchmark (no browser profiler in this environment): the gallery (100k rows) is the manual check; if profiling later
shows frame drops, swapping in tanstack is a change confined to `TableView.svelte`.

### How to verify
1. `make check` (green), `cd web && bun run test`.
2. `bun run dev`: the gallery needs the dev route, which is **not** registered right now (blocked). Until the lead
   approves `routes.ts`, mount `DevComponentsPage.svelte` from a scratch route locally to walk the table and viewers.
3. To wire it after approval, re-add the 10-line block that was reverted (see Out-of-scope) — prod builds drop it
   (`bun run build`: no "Component gallery" string in `dist/`).
4. Once reachable: scroll the 100k-row table, sort, hide/reorder/resize via **Columns**, shift/ctrl-click and
   arrow/Page keys, right-click a row, double-click to "open"; toggle **Use 5 MB response** for Raw `Show more`,
   Ctrl+F, and the Table tab on the big form body.

### Known limitations
- The gallery has no route until the lead resolves the Out-of-scope question; `DevComponentsPage` is covered only by
  its jsdom smoke test.
- `MessageViewer` search matches the raw message only; jumping from Pretty/Hex switches to Raw. JS is shown as-is in
  Pretty (no JS formatter), markup re-indent is best-effort (no `<pre>`/`<script>` special-casing).
- Table mode samples: params come from the first 128 KB of the body, max 500 rows, and the note says so. Query params
  of a request line are never truncated (start lines are short).
- Hex view is approximate for non-UTF-8 bytes (the body is a JS string by the time it reaches the viewer; the
  byte limit vs. the 128 KB char-based size label are also approximate) and covers the body only.
- `TrafficTable` reads `config` at mount (pages should render it after `ui_state` loads); row height token is read once
  per mount (density change needs a remount).
- `SplitPane` sizes are not applied from a persisted value after mount (pass `defaultSizes` at mount); no test for drag
  (jsdom lacks layout/ResizeObserver), only keyboard resize.
- E2E deferred: Playwright is not a dependency yet (T-021).
- `web/src/lib/editor/**` (allowed path) intentionally untouched: CodeMirror is not needed for read-only 5 MB display.

### Follow-ups (proposed tasks)
- **Lead:** resolve the `routes.ts` path question (blocker 1); the gallery AC stays unchecked until then.
- Persist column config / pane sizes to backend `ui_state` from the pages (needs the settings contract).
- Editable MessageViewer variant + CodeMirror for Replay/Intercept (T-0xx); original/modified switch (F-HIST-4).
- Hex view over `Uint8Array` once the History data contract carries raw bytes.
- E2E spec for the gallery and table/viewer keyboard paths with T-021 tooling.

## Review (reviewer)
Reviewer: claude-code (reviewer agent) · 2026-11-18 · branch `task/T-012-shared-table-viewer` @ 57efcde

`make check`: green (Go tests, svelte-check 0/0, eslint, prettier, vitest 18 files / 102 tests).
`make test-int`: not run (no proxy/store/rawhttp changes).
Scope: every file is inside `paths` **except** `web/src/lib/router/routes.ts` (see first finding).
Load assumed: one user, up to 100k rows in the table, 5 MB bodies in the viewer (AC + docs/02 §9).

### Findings
- [ ] [blocker] web/src/lib/router/routes.ts:163-171 — Out-of-scope file changed and committed. Hard rule 1 says to write the request and stop. The note asks for lead approval, but the change was committed anyway. → Lead adds `web/src/lib/router/routes.ts` to `paths` (approval), or the implementer reverts it and sets `blocked`. The change itself is fine (dev only, 10 lines).
- [ ] [blocker] web/src/lib/components/MessageViewer/MessageTable.svelte:20 — `{#each parsed.headers as h (h.name + h.value)}`: two identical header lines (e.g. `Set-Cookie: a=1` twice, `Vary: Accept-Encoding` twice, duplicate `Content-Length` in smuggling tests) give duplicate keys. Svelte 5 throws `each_key_duplicate` in dev **and** prod (`svelte/.../blocks/each.js:357-363`). bits-ui renders every `Tabs.Content` (hidden ones too), so the whole viewer crashes, even on the Raw tab. → Key by index `(h, i) (i)` or drop the key. Add a test with a duplicate header.
- [ ] [major] web/src/lib/components/MessageViewer/MessageViewer.svelte:228 + MessageTable.svelte:8 — The Table tab is always mounted, and the param table has no limit. A 5 MB `application/x-www-form-urlencoded` body (e.g. `a=1&` repeated) parses to about 1.25M params and renders 1.25M `<tr>` when the viewer opens. This breaks the AC "5 MB body without freezing". → Wrap it in `{#if mode === 'table'}<MessageTable …/>{/if}` and render the first N params (e.g. 1000) plus an "N more" line. Add a test with a big form body.
- [ ] [major] web/src/lib/components/TrafficTable/TrafficTable.svelte:85-118 — The grid `onkeydown` also gets keys from the buttons inside the grid (header sort buttons, resize handles, "Columns" trigger). Enter on a sort button sorts **and** calls `onrowopen(rows[0])`. Space on one toggles row selection and calls `preventDefault`. → Start the handler with `if (e.target !== e.currentTarget) return;`. Add a test: Enter on a header button does not call `onrowopen`.
- [ ] [major] web/src/lib/components/TrafficTable/TableView.svelte:199 — `color ? \`var(--row-${color(row)})\` : undefined` checks that the callback exists, not what it returns. If `color(row)` returns `null`, the value is `var(--row-null)`. That is invalid at computed-value time, so the colour falls back to `currentcolor`. Every uncoloured row gets a 3px text-coloured stripe (visible in the gallery, where most rows return null). → `{@const c = color?.(row)}` … `style:border-left-color={c ? \`var(--row-${c})\` : undefined}`.
- [ ] [minor] web/src/lib/components/TrafficTable/TrafficTable.svelte:87 — With no active row, `from` is 0, so the first ArrowDown selects row 1 and skips row 0. → If `active < 0`, ArrowDown/ArrowUp go to index 0.
- [ ] [minor] web/src/lib/components/TrafficTable/TrafficTable.svelte:105 — Ctrl/Cmd+A selects all rows but does not call `preventDefault`, so the browser also selects all page text. → Call `e.preventDefault()` in that branch.
- [ ] [minor] web/src/lib/components/TrafficTable/columns.ts:23-25 — Stored `ui_state` is outside data. A config without `widths` throws (`config?.widths[c.id]`). A config with a repeated hidden id (`['host','host']`) breaks `visibleCount` and `toggleColumn`, so the menu can disable the wrong checkbox. → `config?.widths?.[c.id]` and `[...new Set(config?.hidden ?? [])]`. Add one test.
- [ ] [minor] web/src/lib/components/MessageViewer/format.ts:86,114 — Offsets are kept for up to 1000 matches, but only 200 are highlighted. After 200 Enter presses the current match has no `<mark data-current>`, so `jump` scrolls nowhere while the counter still moves. → Pick the 200 highlighted matches around `current` instead of the first 200, or use the same cap for both.
- [ ] [minor] web/src/lib/components/MessageViewer/parse.ts:53 — `target.split('?')[1]` cuts the query at a second `?`. With `/a?x=1?y=2`, the value of `x` shows as `1` and `?y=2` is lost. → `target.slice(target.indexOf('?') + 1)` when `indexOf('?') >= 0`.
- [ ] [minor] web/src/lib/components/SplitPane/SplitPane.test.ts:36 — paneforge already calls `onLayoutChange` on mount (`paneforge.svelte.js:97`). So `toHaveBeenCalled()` passes even if keyboard resize does nothing. → Call `onsizeschange.mockClear()` before the key press, then assert the new first size is larger than 55.
- [ ] [minor] web/src/lib/components/MessageViewer/format.ts:63 — `hexDump` encodes the whole 5 MB body on every "Show more" to show 8 KB more. Also, since the body is a JS string, any non-UTF-8 byte shows as `ef bf bd`. For a security tool this hex view shows wrong bytes (the limitation is noted in the handoff). → Encode only `body.slice(0, maxBytes)` now. Add a follow-up task so Hex takes the body as `Uint8Array` once the History data contract exists.
- [ ] [nit] web/src/lib/components/TrafficTable/TableView.svelte:190-197 — `selection?.has(key)` is evaluated 3 times per row. → `{@const selected = selection?.has(key) ?? false}`.

### AC coverage
- Demo route with 100k rows: yes (`DevComponentsPage.test.ts`). It depends on the out-of-scope route (blocker 1).
- Column show/hide/reorder/resize, sort, multi-select, keyboard, context menu: covered, except mouse/keyboard resize (no test for `onwidth`/commit) and the key-bubbling bug.
- 5 MB without freezing + Ctrl+F: the Raw path is covered; the Table path is not (major above).
- Selection and column-config callback tests: yes.
- Virtualization decision recorded in the Handoff: yes. It is reasoned, not benchmarked; acceptable for now, and the gallery is the manual check.

### Verdict
**changes_requested**. Fix the 2 blockers and 3 majors; the minors may go to follow-ups if noted in the Handoff.
