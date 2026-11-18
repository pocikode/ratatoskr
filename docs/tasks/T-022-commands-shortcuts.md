---
id: T-022
title: Command registry, shortcuts manager and context menu builder
milestone: M1
role: frontend
status: in_review
depends_on: [T-003]
owner: opencode/deepseek-v4.1-flash-1017
branch: task/T-022-commands-shortcuts
paths:
  - web/src/lib/commands/**
  - web/src/lib/components/CommandPalette/**
  - web/src/lib/components/ContextMenu/**
spec:
  - docs/02-ui-layout.md §4, §5
  - docs/01-feature-spec.md#F-UI-1, #F-UI-2
---

## Acceptance criteria
- [x] Commands registered with `when` predicates; palette, shortcuts and context menus read the same registry.
- [x] Default bindings from docs/02 §4; rebinding API with conflict detection; persistence via callback (backend wiring later).
- [x] Chorded "g then h" navigation works; shortcuts disabled inside text inputs unless marked global.

## Plan (implementer, ≤15 lines)
- Keep the existing registry (`when` already gates palette + `run`); add `commands/menu.ts: commandMenu(registry, ids, ctx)` so menus are a filtered, order-preserving view of the same registry.
- `components/ContextMenu/ContextMenu.svelte`: bits-ui wrapper, props `{ ids, context?, registry? }` + trigger snippet; items grouped by `group`, disabled/hidden by `when`, `onSelect` → `registry.run(id, ctx)`.
- Rewrite `commands/shortcuts.ts`: §4 default table as data (`{ key, mod?, page?, global?, command }`); one key may serve several page-scoped commands; page-specific binding wins over unscoped.
- Typing guard: non-mod bindings and the `g` chord are ignored in inputs/contenteditable; only `global: true` bindings fire there (palette, send, find, tab, intercept forward/drop).
- Rebinding API: `rebind(command, key, {mod})` keeps the command's page scope, returns the conflicting command id (null on success) and refuses the change; `conflictFor(binding)`; `reset()`.
- Persistence: `opts.persist(bindings)` callback after every change (backend wiring later); `opts.bindings` to seed from settings.
- Chord stays data-driven from `routes[].goto`; `ctx.page` forwarded to `registry.run` so `when` can scope commands.
- No palette/settings UI changes (Settings → Shortcuts is the settings task); no new deps.
- Tests: shortcut table/binding precedence/conflict/persist/typing guard; menu builder ordering + `when`; ContextMenu component renders items and runs a command with its context.

## Out-of-scope change needed (if any)

## Handoff (implementer)
### What changed
- `commands/shortcuts.ts`: the docs/02 §4 table as data — `{ key, mod?, page?, global?, command }`. All 15 rows are present,
  including the `g <letter>` chord (still derived from `routes[].goto`).
  - Page-scoped bindings (`replay.send`, `intercept.forward`, `intercept.drop`) let one key serve several contexts as
    F-UI-2 requires; a page-scoped hit beats an unscoped one, and a scoped key does nothing on other pages.
  - Typing guard: non-`global` bindings and the chord are ignored in `input`/`textarea`/`select`/contenteditable.
    `global: true` is set only where the action must work while editing (palette, find, send/forward/drop, tab next/prev/close).
  - Rebinding API: `rebind(command, key, { mod })` keeps the command's page/global scope, returns the conflicting command id
    and refuses the change (spec: "conflicts warned"), or `null` on success. `conflictFor(key, mod, page)` exposes the same
    check for a Settings → Shortcuts form. `reset()` restores the defaults.
  - Persistence via callback: `ShortcutOptions.persist(bindings)` fires after every rebind/reset with a copy; `bindings` can be
    seeded from stored settings. Nothing is read or written by this task (backend wiring later).
  - `registry.run(id, { page })` now receives the page, so command `when` predicates can scope themselves too.
- `commands/menu.ts`: `commandMenu(registry, ids, ctx)` — an ordered, deduplicated-by-nothing view of the registry for a
  context menu: unknown ids dropped, `when(ctx)` honoured. Menus therefore can never drift from palette/shortcuts.
- `components/ContextMenu/ContextMenu.svelte`: bits-ui `ContextMenu` wrapper. Props `ids`, `context`, `registry`
  (defaults to the shared one) and a `children` trigger snippet; renders items with icons, a separator whenever `group`
  changes, and runs the command with the same context on select.
- Tests: `shortcuts.test.ts` (default table, page precedence, typing guard, chord, rebind + conflict + persist + reset),
  `menu.test.ts` (order, unknown ids, `when`), `ContextMenu.test.ts` (right-click renders items from the registry and runs
  the picked command with its context; `when`-hidden commands absent).

### Round 2 — review fixes
- `shortcuts.ts`: a hit only counts when its command is registered and `when({ page })` passes (`#runnable`), so
  ⌘R/⌘F/⌘[ /`/`/Delete fall through to the browser default instead of being swallowed. Tests for both the unregistered and
  the `when`-rejected case assert `defaultPrevented === false`.
- `conflictFor` now reports the `g` chord: plain `g` conflicts with `nav.goto`, a goto letter with `nav.goto.<route id>`.
  `rebind` therefore refuses both (and still allows `⌘g`).
- `ContextMenu`: `disabled={items.length === 0}` on the trigger, so a menu with no runnable command leaves the native menu
  alone (test added; verified it fails without the fix).
- `menu.ts`: `when` checked per id instead of scanning `registry.available()`.

### Round 3 — review fix
- `intercept.drop` (⌘/Ctrl+Backspace) is no longer `global`: the row is "Intercept, item selected", so it stays out of the
  request editor, which keeps its delete-word/delete-to-line-start key. Drop still fires from the list (focus outside inputs).
- Test: with `intercept.drop` registered on page `intercept`, ⌘Backspace from an `input()` runs nothing and leaves
  `defaultPrevented === false`; the same key with focus outside the input runs it.

### How to verify
- `cd web && bun run test` (140 tests) and `make check` (green). `cd web && bun run build` re-checked: no `dist` changes
  tracked (build output is gitignored), bundle 274.9 kB / 84.7 kB gzip (budget 400 kB).
- Manual: `cd web && bun run dev` → ⌘K palette, `g h` / `g r`, `/` outside inputs, typing letters in a filter input does
  not navigate.

### Limitations / follow-ups
- No consumer of `ContextMenu` yet: the first is the History/Search row menu via `TrafficTable`'s existing
  `onrowcontextmenu` (page task, outside this task's paths). The component + builder are tested directly.
- Settings → Shortcuts UI (form over `conflictFor`/`rebind`/`reset`, seeded from settings) belongs to the Settings task; the
  manager holds `bindings` as a plain array, so that task can move it to `shortcuts.svelte.ts` if it needs reactive display.
- Bindings for commands other tasks own (`history.sendToReplay`, `replay.send`, `intercept.*`, `tab.*`, `querybar.focus`,
  `editor.find`, `selection.delete`) are registered as data only: `registry.run` is a no-op until those commands exist.
- `selection.delete` has two bindings (`Delete` and `Backspace`, macOS) — rebinding it collapses them to one key.
- `tab.close` on ⌘W is browser-reserved outside an installed PWA (docs/02 §4 documents the fallback); we `preventDefault`
  when the app has focus, which covers the in-app case only.
- No Playwright spec: `tests/e2e` and `@playwright/test` are not on this branch yet (T-021); the AC is covered by component
  tests. Re-add a shell E2E (palette + `g h`) once T-021 lands.

## Review (reviewer)
Reviewer: claude-opus-5-5 · 2026-11-18 · branch `task/T-022-commands-shortcuts` @ 41a5226

Scope: all changed files are inside `paths`. `make check` green (svelte-check 0/0, eslint, prettier, 140 vitest tests).
No Go/proxy/store changes, so `make test-int` was not needed. No new deps; no Caido material.

Findings:
- [x] [major] web/src/lib/commands/shortcuts.ts:75-81 — `handle` calls `e.preventDefault()` for any matching binding, even when its
  command is not registered or its `when` rejects the context (`registry.run` then returns false). Today only nav/palette/sidebar/theme
  are registered (`commands/builtin.ts`), so after merge ⌘/Ctrl+R no longer reloads, ⌘F (browser find, also inside inputs because
  `global`), ⌘[ / ⌘] (browser back/forward on macOS), `/` and Backspace/Delete are swallowed and do nothing. The same happens later on
  any page where a command's `when` is false (e.g. ⌘R outside History). → Keep only hits whose command is runnable before choosing:
  add `&& this.#runnable(b.command, page)` to the `filter` at :75, where `#runnable` = `const c = this.registry.get(id); return !!c && (c.when?.({ page }) ?? true)`.
  An unrunnable binding then falls through and the browser default stays. Add a test: ⌘R with no `history.sendToReplay`
  registered leaves `e.defaultPrevented === false`.
- [x] [minor] web/src/lib/commands/shortcuts.ts:100-104 — `conflictFor`/`rebind` ignore the `g` chord. `rebind('x', 'g')` is accepted
  and `g` then never starts the chord (the binding check at :75 runs first); `rebind('x', 'h')` is accepted and `g h` runs `x`
  instead of going to History. F-UI-2 says conflicts are warned. → In `conflictFor`, when `!mod` also report the chord: `key === 'g'`
  or `routes.some((r) => r.goto === k)` → return a pseudo binding `{ key: k, command: 'nav.goto.<id>' }` (or `'nav.goto'` for `g`). One test.
- [x] [minor] web/src/lib/components/ContextMenu/ContextMenu.svelte:28 — when every id is hidden by `when` or unregistered, right-click
  still opens an empty bordered box and blocks the native menu. → `<Menu.Trigger child={trigger} disabled={items.length === 0} />`
  (`disabled` exists on bits-ui `ContextMenuTriggerProps`, verified in node_modules types).
- [x] [nit] web/src/lib/commands/menu.ts:12-16 — `registry.available(ctx)` runs `when` for every registered command just to filter a
  few ids. → `return ids.flatMap((id) => { const c = registry.get(id); return c && (c.when?.(ctx) ?? true) ? [c] : []; });`

AC coverage: AC1 (shared registry: palette/shortcuts/`commandMenu` + ContextMenu tests) ✔ · AC2 (default table, rebind, conflict,
persist, reset) ✔ apart from the chord gap above · AC3 (chord, typing guard, `global`) ✔. Missing E2E is justified in Handoff (T-021).

Verdict: **changes_requested** — fix the major; the minors are cheap and should go in the same pass.

### Round 2
Reviewer: claude-opus-5-5 · 2026-11-18 · branch `task/T-022-commands-shortcuts` @ 87d55a7

Scope: all changed files still inside `paths`. `make check` green (svelte-check 0/0, eslint, prettier, 140 vitest tests).
All four round-1 findings are fixed and each has a test. `App.svelte` (`new ShortcutManager(registry)`) is the only caller and
still works with the new options signature.

Findings:
- [ ] [major] web/src/lib/commands/shortcuts.ts:25 — `intercept.drop` (Ctrl/⌘+Backspace) is `global: true`, so it fires inside the
  request editor on the Intercept page. Ctrl+Backspace is "delete previous word" on Win/Linux (⌘⌫ is "delete to line start" on macOS).
  A user editing an intercepted request who deletes a word drops the whole request and loses the edit. `when` only gets `{ page }`,
  so the intercept task can't stop this either, and `rebind` keeps the `global` flag. docs/02 §4 says "Drop (Intercept, *item selected*)".
  → Remove `global: true` from that row. Drop then still works from the list (focus outside the editor), and the editor keeps its normal
  key. Add one test: with `intercept.drop` registered and page `intercept`, ⌘Backspace from an `input()` target doesn't run it and
  leaves `defaultPrevented === false`.

AC coverage: AC1 ✔ · AC2 ✔ (chord conflicts now reported) · AC3 ✔ apart from the editor case above. Missing E2E is still justified (T-021).

Verdict: **changes_requested** — a one-line fix plus a test.
