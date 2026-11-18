---
id: T-009
title: Workspace page (projects) and top-bar project switcher
milestone: M0
role: frontend
status: in_review
depends_on: [T-003, T-006, T-007]
owner: claude
branch: task/T-009-workspace-page
paths:
  - web/src/pages/workspace/**
  - web/src/lib/components/ProjectSwitcher*.svelte
  - web/src/lib/stores/project.svelte*.ts
  - web/src/lib/components/TopBar.svelte
  - web/src/App.test.ts
  - web/src/lib/commands/registry.ts
  - web/src/lib/api/generated/**   # regenerated via `bun run codegen` only
  - web/package.json
  - web/bun.lock
spec:
  - docs/01-feature-spec.md#F-WSP-1
  - docs/02-ui-layout.md §1, §3.12
---

## Goal
Users manage projects on the Workspace page (Projects tab; Backups tab placeholder) and switch projects from the top bar; all page stores reset on `projectChanged`.

## Acceptance criteria
- [x] Create/rename/duplicate/delete/set default/select work against the real API (WorkspacePage.test.ts, project.svelte.test.ts; component tests with mocked client). The Playwright smoke moved to T-021 (no E2E harness yet; lead decision 2026-11-18).
- [x] Switcher is keyboard (ProjectSwitcher.test.ts) accessible, searchable, shows the temp project distinctly.
- [x] Deleting requires typed confirmation of the project name (WorkspacePage.test.ts "deletes only after typing the exact name").

## Plan (implementer, ≤15 lines)
- `pages/workspace/project.graphql` + codegen; `ProjectStore` (`project.svelte.ts`): list, mutations, `projectChanged` subscription, `epoch` + `onReset()` for stores to reset on SELECTED.
- `ProjectSwitcher` (bits-ui Combobox) in TopBar; registers `project.create` / `project.select.<id>` palette commands.
- `WorkspacePage` (Projects/Backups tabs, table, row actions) + `ProjectDialog` (create/rename/typed-name delete).
- Component tests with a fake urql client.

## Out-of-scope change needed (if any)
Applied minimally because the task is unfinishable without them: `TopBar.svelte` mounts `<ProjectSwitcher />`; `App.test.ts` line 91 scoped its `combobox` query to the palette (the switcher is now a second combobox). Generated files regenerated via `bun run codegen`. Lead: please confirm or widen `paths`.

## Lead decisions (2026-11-18, answers review round 2)
- **Paths widened** (frontmatter) to cover TopBar, App.test, `registry.ts` (`CommandContext.project` only; the rest of `lib/commands/**` stays T-022's), generated code (codegen only), the store test, `package.json`/`bun.lock`. Resolves the scope blocker.
- **wonka approved** as a direct dependency (urql's own stream library, already in the lockfile). Listed in docs/06 §1. Implementer: `cd web && bun add wonka@^6`, commit `package.json` + `bun.lock`.
- **AC1 Playwright smoke** moved to T-021 (it owns `web/tests/e2e/**` and `playwright.config.ts`). AC1 amended above; T-021 gained a Workspace AC and depends on T-009.

## Handoff (implementer)
- **Changed:** project store, switcher, Workspace page/dialog, GraphQL ops, tests (see Plan).
- **Verify:** `make check` green; `cd web && bun run dev` against `make run`, use the top-bar switcher and the Workspace page.
- **Limitations:** (1) **No Playwright smoke**: `@playwright/test` and `web/tests/e2e` do not exist in the repo yet; needs E2E infra first. (2) Row actions (rename/duplicate/delete/default) call the store directly, not the command registry (commands take no per-row context yet); only `project.create` and per-project `project.select.*` are commands. (3) Page stores don't yet subscribe to `projects.onReset()`/`epoch`: no project-scoped stores exist yet; later tasks must hook in. (4) Backups tab is a placeholder. (5) Dialog `$effect`-free; switcher input shows the project name and filters only while open.
- **Follow-ups:** E2E harness task; context-aware commands for row actions.

## Rework (round 1 fixes)
- Store: full resubscribe on stream end (wonka `onEnd`) + refetch; refetch on ws reconnect; stale-refresh guard (`#seq`); `openDialog()` clears stale error; plain `Set`; new `project.svelte.test.ts`.
- Page: h1 "Workspace"; row actions via commands (`CommandContext.project`, `projectRowCommands`); inner `<div>` in actions cell; GB in `fmtSize`; tabs as objects.
- Switcher: "New project…" / "Manage projects" entries; visible "temporary" badge when closed; `role="alert"` (sr-only) for store errors. Tests added.
- Not done (lead decision): Playwright smoke (no E2E harness; needs follow-up task + AC1 amendment); `paths` widening for TopBar/App.test/generated + `registry.ts` (CommandContext.project) + `wonka` import (transitive dep of urql, not in package.json: add as direct dep or approve).

## Rework (round 2 fixes)
- Switcher: function `bind:value` (always returns to current project) so New/Manage work repeatedly; input `onclick` reopens a closed-but-focused list; `{value,label}` entries; test picks "New project…" twice.
- Store: resubscribe delayed 1 s (timer cleared on stop); test updated.
- Page: merged imports; test unregisters commands via `onTestFinished`.
- Still lead-owned: widen `paths`; add `wonka` as direct dep (`bun add wonka@^6`, docs/06); AC1 Playwright amendment.

## Review (reviewer)
**Round 3 — verdict: `changes_requested`** (reviewed the uncommitted working tree on top of e9543f5)

`make check`: **green** (svelte-check 0 errors, eslint, prettier, vitest 62/62). `make test-int` not run: no proxy/store/rawhttp changes.

Round-2 status: the switcher function `bind:value` fixes New/Manage repeat and select retry (tested). Resubscribe delay, test unregister and both nits are fixed. Lead decisions close the scope blocker and the AC1 major (ticked below). One round-2 item is still open: wonka.

- [ ] [major] web/package.json:dependencies — `wonka` is still missing, but `project.svelte.ts:3` and `project.svelte.test.ts:1` import it. The lead approved it and docs/06 now lists it, but the implementer has not run the install. Until then it only resolves through Bun hoisting `@urql/core`'s copy. → `cd web && bun add wonka@^6`, then commit `package.json` and `bun.lock`.
- [ ] [major] (process) None of the round-1/round-2 rework is committed. `git status` shows 10 modified files and the untracked `web/src/lib/stores/project.svelte.test.ts`. A merge of the branch as it stands would ship 45e10b8 without the fixes and without the new test. → Commit the code as `fix(T-009): review rounds 1-2` and the wonka install as `build(T-009): add wonka`. The lead commits their docs edits (docs/06, docs/08, T-021, the frontmatter) as `docs(T-009): lead decisions`.
- [ ] [minor] docs/tasks/T-009-workspace-page.md:Acceptance criteria — all three ACs are still unticked, although tests now cover them. AC1: WorkspacePage.test.ts plus project.svelte.test.ts. AC2: ProjectSwitcher.test.ts (keyboard, search, `temporary` badge). AC3: WorkspacePage.test.ts "deletes only after typing the exact name". → Tick them and name the test for each.
- [ ] [nit] web/src/lib/stores/project.svelte.test.ts:41-50 — the resubscribe test waits on real time, about 2.2 s (the 1 s retry plus `setTimeout(r, 1200)`). That is most of the suite's 3.3 s. → `vi.useFakeTimers()` and `vi.advanceTimersByTime(1000)`. Keep `vi.waitFor` only for the promise-based refetch.

Checked and fine: the reconnect refetch skips the first `connected` correctly. Both `ConnectionState.start` and `ProjectStore.start` attach their listeners synchronously at mount, before the lazy socket opens, and the `ping` subscription keeps the socket open, so the first event really is the first connect. wonka `onEnd` also fires on our own `unsubscribe()`, but `stopped` is set first, so `stop()` schedules no retry. The type-only import cycle between `registry.ts` and `project.svelte.ts` is fine (it is erased at build). `registry.run` enforces `when`, so row commands cannot run without `ctx.project`. Names are rendered as text only (no XSS).

**Round 2 — verdict: `changes_requested`** (reviewed the uncommitted rework on top of 70a7a0b; the implementer must commit it, e.g. `fix(T-009): review round 1`)

`make check`: **green**. `bun run test` passed 61/61 in 4 runs. The round-1 h1 flake is fixed. `make test-int` not run: no proxy/store/rawhttp changes.

Round-1 status: fixed: h1, store resubscribe and reconnect refetch, subscription tests, switcher create/manage, row commands, temporary badge, stale-refresh guard, dialog error reset, `<td>` wrapper, GB, plain `Set`, tab objects. **Still open (lead):** the out-of-`paths` blocker and the AC1 Playwright major. Both now also cover the new files listed below.

- [x] [blocker] (lead) Scope: round-1 out-of-`paths` blocker is still open. It now also covers `web/src/lib/commands/registry.ts` (`CommandContext.project`) and `web/src/lib/stores/project.svelte.test.ts`; the glob `project.svelte.ts` does not match the test file. → Lead: widen `paths` to `TopBar.svelte`, `App.test.ts`, `lib/api/generated/**`, `lib/commands/registry.ts`, `lib/stores/project.svelte*.ts`.
- [x] [major] (lead) AC1 Playwright smoke: still open, no change since round 1. → Lead: amend AC1 to point at the E2E-harness task id, or add the harness first.
- [x] [major] web/src/lib/components/ProjectSwitcher.svelte:32-38 — "New project…" and "Manage projects" only work every other time. `value` is passed one-way, so bits-ui keeps its own value (`'\0new'`) after the pick, and `store.current?.id` never changes to reset it. The next pick of the same entry runs `toggleItem` → `includesItem` true → value `''` (`allowDeselect` defaults to true). `onValueChange('')` then does nothing. Repro: open switcher → New project… → cancel dialog → open switcher → New project… → no dialog. A failed `store.select(B)` has the same problem: retrying B deselects. → Use a function binding so the value always comes back to the current project: `bind:value={() => store.current?.id ?? '', (id) => { …same branches… }}` and drop `value=`/`onValueChange`. Extend the `fixed entries` test to pick "New project…" twice (closing the dialog in between) and assert the dialog opens both times.
- [ ] [major] web/src/lib/stores/project.svelte.ts:3 — `wonka` is imported but is not in `web/package.json`. It only resolves because Bun hoists `@urql/core`'s dependency (bun.lock:396). Hard rule 8 / docs/06: deps only from the approved list. An isolated linker or a urql bump that changes wonka's major breaks the build silently. → Lead: approve wonka (it is urql's own stream library, so it adds no new code) and add it as a direct dependency with `bun add wonka@^6`, and list it in docs/06 next to `@urql/core`.
- [x] [minor] web/src/lib/stores/project.svelte.ts:71,82-86 — `relisten` resubscribes at once with no delay. If the server keeps rejecting the subscription (for example a resolver error, where urql emits the error and then ends), the store loops: subscribe, end, resubscribe, plus a `projects` refetch each round, at network speed, forever. → Delay the resubscribe: `onEnd(() => setTimeout(relisten, 1000))`, or back off up to ~30 s. `stopped` already guards cleanup. In the test, use fake timers or `vi.waitFor`.
- [x] [minor] web/src/pages/workspace/WorkspacePage.test.ts:32 — `registry.register(...)` is never undone. The global registry keeps a `project.create` bound to each test's store, so later test files see commands from an old store. → Keep the returned unregister and call it in `afterEach` (or `onTestFinished(registry.register(...))`).
- [x] [nit] web/src/pages/workspace/WorkspacePage.svelte:3-9 — two import statements from `$lib/stores/project.svelte`. → Merge `projectRowCommands` into the first import.
- [x] [nit] web/src/lib/components/ProjectSwitcher.svelte:83-85 — the `[[NEW,…],[MANAGE,…]]` tuple brings back the `value!` assertion that the tabs fix removed. → Use `{ value, label }` objects, as WorkspacePage does now.

Checked and fine: wonka `onEnd` fires once (it has its own `ended` flag), so `relisten`'s `sub?.unsubscribe()` on an ended source does not recurse, and `stop()` sets `stopped` before it unsubscribes. The `#seq` guard drops slower stale responses (tested). Row commands are hidden from the palette when there is no `ctx.project`. The delete flow still needs the typed name.

**Round 1 — verdict: `changes_requested`** (reviewed 45e10b8)

`make check`: 1st run **failed**: `src/App.test.ts:35 navigates to Workspace placeholder`, `findByRole('heading', { level: 1, name: 'Workspace' })` timed out. 2nd run and 6 isolated `bun run test` runs passed. The failure depends on timing (see the first blocker). `make test-int` not run: no proxy/store/rawhttp changes.

Scope: `TopBar.svelte`, `App.test.ts` and `web/src/lib/api/generated/**` are outside `paths`.

- [ ] [blocker] web/src/pages/workspace/WorkspacePage.svelte:34 — The page's h1 is "Projects". `App.test.ts:35` only passes because `RouterView` shows `PlaceholderPage` (h1 "Workspace") while the lazy chunk loads. When the chunk resolves first, the test fails, so `make check` is flaky. This is the failure seen above. → Render `<h1>Workspace</h1>` as the page title, as every page does, and demote "Projects" to an `h2` or drop it (the tab already says Projects).
- [ ] [blocker] web/src/lib/components/TopBar.svelte:5,27 · web/src/App.test.ts:91 · web/src/lib/api/generated/** — Files outside `paths` were edited before the lead approved them. AGENTS.md hard rule 1 and docs/07 §3 say "do not just do it". The edits themselves look right and are minimal. → Lead: widen `paths` to `web/src/lib/components/TopBar.svelte`, `web/src/App.test.ts` and `web/src/lib/api/generated/**` (regenerated, not hand-edited). This finding then resolves with no code change.
- [ ] [major] AC1 "one Playwright smoke" is not met. `@playwright/test` is not installed and `web/tests/e2e` does not exist. Do not tick the AC silently. The DoD also requires E2E for user-visible behaviour. → Lead: either move the smoke into the E2E-harness follow-up task explicitly (amend AC1 here and reference the new task id), or add the harness first. The implementer should have set `blocked` instead.
- [ ] [major] web/src/lib/stores/project.svelte.ts:49-61 — The store never resubscribes when `projectChanged` completes or errors. The schema (`project.graphqls`, `projectChanged`) says "the server ends this subscription if the client falls too far behind; clients must then resubscribe and refetch `projects`". After that, SELECTED events are lost: `epoch` and `onReset` never fire, so pages show stale rows, which breaks the F-WSP-1 AC "switching projects never shows stale rows". Also, after a WS reconnect (graphql-ws `retryAttempts: Infinity`), events missed during the gap are never refetched. → Pass a full observer (`{ next, error, complete }`), or check `r.error` and source completion, and on end/error resubscribe and `refresh()`. Also `refresh()` on `getApi().ws.on('connected')` after the first connect. Add a unit test that completes the fake subscription and asserts a resubscribe plus refetch.
- [ ] [major] web/src/lib/stores/project.svelte.ts:53-60 — The task Goal ("all page stores reset on `projectChanged`") has no test. No test drives the subscription: SELECTED bumping `epoch` and calling `onReset` callbacks, non-SELECTED kinds only refetching, and the unsubscribe cleanup. → Add `project.svelte.test.ts` with a fake client whose `subscription()` exposes the sink. Push `{ kind: 'SELECTED' }` and `{ kind: 'UPDATED' }`, then assert `epoch`, the callback calls and the refetch count.
- [ ] [major] web/src/lib/components/ProjectSwitcher.svelte:54-72 — docs/02 §1 specifies the switcher as "(search, create, manage)". Only search and select are implemented, so the dropdown has no way to create or manage projects. → Add two fixed entries after the list: "New project…" (runs `registry.run('project.create')`) and "Manage projects" (navigates to `workspace`). Cover both in `ProjectSwitcher.test.ts`.
- [ ] [minor] web/src/pages/workspace/WorkspacePage.svelte:65,78,87 — Row actions (Select, Duplicate, Set/Clear default, Rename, Delete) use ad-hoc `onclick` calls to the store, not registered commands. docs/06 §5 says "All actions registered in `lib/commands/registry.ts`". `CommandContext` is meant to be extended. → Add `project?: Project` (or `projectId`) to `CommandContext` and register `project.rename/duplicate/delete/setDefault/select` with `run(ctx)`. If that is deferred, create the follow-up task and reference its id here, not just "follow-ups".
- [ ] [minor] web/src/lib/components/ProjectSwitcher.svelte:39-43 — While the switcher is closed, the only sign that the current project is temporary is the amber icon colour. docs/02 §8: "no colour-only signals". → Show a text marker in the closed state, e.g. a visible "temporary" badge next to the input, or include "(temporary)" in `aria-label`/`title`. Assert it in the switcher test.
- [ ] [minor] web/src/lib/stores/project.svelte.ts:64-72 — `refresh()` calls race: a mutation's own refresh plus the refresh from the resulting `projectChanged` event. A slower, older response can overwrite a newer list. → Keep a request counter (`const n = ++this.#seq; … if (n !== this.#seq) return;`) and ignore stale responses.
- [ ] [minor] web/src/pages/workspace/ProjectDialog.svelte:50 · web/src/lib/stores/project.svelte.ts:71,79 — `store.error` is one shared field. A stale error from a failed row action or list refresh appears inside a newly opened create/rename/delete dialog. A failed switcher `select` is silent in the top bar. → Clear `store.error` when a dialog opens, e.g. in a small `openDialog()` setter, or keep a dialog-local error from `#run`'s result. Surface switcher failures via a toast once Toasts exist, or at minimum via `role="alert"` near the switcher.
- [ ] [minor] web/src/pages/workspace/WorkspacePage.svelte:59 — `class="flex …"` on a `<td>` changes the cell's display. Chromium can drop table/cell semantics and the column misaligns, against docs/02 §8 "ARIA roles on tables". → Put the buttons in an inner `<div class="flex flex-wrap gap-1">`.
- [ ] [nit] web/src/pages/workspace/WorkspacePage.svelte:8-15 — `fmtSize` stops at MB; a project DB can be GB (docs/02 §1 shows "1.2 GB"). → Add a GB branch.
- [ ] [nit] web/src/lib/stores/project.svelte.ts:34 — `#resets` is never read reactively. → Use a plain `Set`.
- [ ] [nit] web/src/pages/workspace/WorkspacePage.svelte:24 — `value={id!}` uses a non-null assertion because the tuple is inferred as `string[]`. → Declare the tabs `as const`, or as an array of `{ id, label }` objects.

Checked and fine: names rendered via text interpolation (no XSS); the delete confirm needs an exact name match and is tested; `$effect`s return their cleanup (start/unsubscribe, registry unregister); the Combobox `inputValue` and `onOpenChange` props exist in bits-ui 2.21; no Caido names or assets; the operation set matches `project.graphqls`.

## Rework (round 3 fixes)
- `bun add wonka@^6` (direct dep); ACs ticked with test names; store resubscribe test uses fake timers; rework committed.
