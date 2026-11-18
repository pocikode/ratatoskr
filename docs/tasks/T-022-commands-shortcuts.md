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

### How to verify
- `cd web && bun run test` (136 tests) and `make check` (green). `cd web && bun run build` re-checked: no `dist` changes
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
