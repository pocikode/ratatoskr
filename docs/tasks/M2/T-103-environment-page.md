---
id: T-103
title: Environment page and status-bar environment switcher
milestone: M2
role: frontend
status: done
depends_on: [T-098]
owner: "opencode (deepseek-v4.1-flash-1017)"
branch: task/T-103-environment-page
paths:
  - web/src/pages/environment/**
  - web/src/lib/stores/environment.svelte.ts
  - web/src/lib/stores/environment.svelte.test.ts
  - web/src/lib/components/StatusBar.svelte
  - web/src/lib/components/StatusBar.test.ts
spec:
  - docs/01-feature-spec.md#F-ENV-1 (incl. M2 contract)
  - docs/02-ui-layout.md §1 (status bar), §3.12
---

## Goal
Users manage Global and project environments and their variables, and pick the selected environment from the status bar.
Replay (T-107) reads variables from the same store.

## Scope
- In: `environment.svelte.ts` store (list, selection, `variables` map = selected over Global, refetch after own mutations
  and on project switch); page per docs/02 §3.12: env list with Global pinned first, create/rename/delete (none for
  Global), variables grid (name, value, secret toggle, add/remove rows, save via `setEnvironmentVariables`), secret values
  masked with a per-row reveal; status bar item `env: <name>` (or "No environment") opening a menu to select one or none.
- Out: variables set by workflows/plugins (M5/M6).

## Acceptance criteria
- [x] Global listed first and cannot be renamed/deleted (no actions shown).
- [x] Grid edits are saved together; `BAD_INPUT` for a variable highlights that row with the message.
- [x] Secret values render masked until revealed; the reveal is per row and resets on navigation.
- [x] Status bar shows the selected environment and switches it (mock assertion on `selectEnvironment`), including "none".
- [x] Store test: `variables` resolves selected over Global and updates after selection change.
- [x] Component tests; `make check` green.

## Notes from lead
- Re-read docs/00 §4 before writing copy.

## Plan (implementer, ≤15 lines)
1. `pages/environment/environment.graphql`: Environments query + create/rename/delete/setVariables/select mutations.
2. `lib/stores/environment.svelte.ts`: `EnvironmentStore` (list, `selected`, `variables` map = selected over Global,
   mutations refetch, `start()` refetches on project switch via `projects.onReset`).
3. `pages/environment/EnvironmentPage.svelte`: env list (Global pinned, no actions), create/rename/delete, variables grid
   (name, value, secret, add/remove, Save), secret value masked with per-row reveal, BAD_INPUT row highlight.
4. `StatusBar.svelte`: `env: <name>` / "No environment" dropdown that selects one or none.
5. Tests: store (`variables` resolution + selection change), page component, status bar (mock `selectEnvironment`).
6. `bun run codegen`, `make check`.

## Out-of-scope change needed (if any)

## Handoff (implementer)
- What changed (round 2, review fixes): page state is now tied to the open environment — the rename form is
  `renamingId` + `renameName` and only renders for that environment; the grid resets only when `active.id` changes,
  so a status-bar switch no longer drops unsaved edits; `removeRow` clears the per-row reveal set (indexes shift);
  Delete is two-step with the environment name in the prompt; the mutation error sits in the grid footer; root uses
  `h-full` instead of `calc(100vh-140px)`. `BAD_INPUT` row parsing only accepts the phrases that name a variable.
  `StatusBar` shows a failed switch as an alert and keeps the old selection; it is the only caller of `store.start()`.
  `EnvironmentStore.refresh` keeps a request counter so a stale answer cannot overwrite a newer list.
- What changed (round 1): `EnvironmentStore` (`web/src/lib/stores/environment.svelte.ts`) with the list, `selected`,
  `variables` (selected over Global) and create/rename/delete/setVariables/select that refetch after each mutation and
  after a project switch (`projects.onReset`); operations in `pages/environment/environment.graphql` (codegen run);
  `EnvironmentPage.svelte` (env list with Global pinned and no actions, inline create/rename, delete, variables grid with
  per-row secret mask/reveal and `BAD_INPUT` row highlight); status bar `env: <name>` dropdown that selects an
  environment or none.
- How to verify: `cd web && bun run test` (store, page and status-bar tests) and `make check`. Without T-037 the real
  server answers `NOT_IMPLEMENTED`; the component tests run against `$lib/api/mocks` (T-098 contract).
- Known limitations: no E2E spec — the page needs the T-037 resolvers first. A `BAD_INPUT` row is found by the variable
  name the message quotes, so duplicate names highlight every row with that name, and a message that quotes no variable
  highlights none (the text still shows). `variables` is a plain `Map` rebuilt by `$derived`. The page no longer starts
  the store, so it relies on the status bar being mounted (it always is, in `App.svelte`).
- Follow-ups (proposed tasks): E2E for F-ENV-1 once T-037 lands; a dev `?mock=environment` hook if wanted before then.
  Question for the lead: can `BAD_INPUT` carry the offending row in `extensions` (name or index) so the UI does not parse
  the message? Also report to the lead: `TestUpstreamErrorIs502AndRecorded` (`internal/proxy/proxy_test.go:175`) failed
  once during review and did not recur on two full `make check` runs — looks flaky.

## Review (reviewer)
- Verdict: approve (round 2; round 1 was changes_requested)
- Checks: diff is inside `paths` (generated files are codegen output). Web `check` (svelte-check, eslint, prettier) and
  `bun run test` (339 tests) are green. `make check` failed once in `go test -race ./...`:
  `--- FAIL: TestUpstreamErrorIs502AndRecorded` (`internal/proxy/proxy_test.go:175`, entries = [...] with `Resp:<nil>`).
  This branch changes no Go file, and `go test -race -count=1 ./internal/proxy` passed on re-run, so it is a flaky proxy
  test, not this task. Please re-run `make check` before moving back to `in_review`, and report the flake to the lead.
- Findings (round 1, all addressed in `fix(T-103)`):
- [x] [major] web/src/pages/environment/EnvironmentPage.svelte:23,81,122,166 — `renaming` is not tied to an environment. Click Rename on A, then click B in the list: the rename form stays open for B with A's name, and "Save name" renames B to A's name (the server only refuses while A still exists). → store `renaming` as `{ id, name }` and show the form only when `renaming.id === active.id`, or set `renaming = null` in the list `onclick`.
  → done as `renamingId` + `renameName`; the form renders only for `renamingId === active.id` and the id-change reset closes it. Test: "keeps the rename form with the environment it was opened on".
- [x] [major] web/src/pages/environment/EnvironmentPage.svelte:19,36,50,206 — reveal state is keyed by row index. Reveal secret row 3, remove row 1: the secret that moves to index 2 shows in clear text, though the user never revealed it. AC "reveal is per row" is broken after a remove. → key `revealed` by a stable row id (add a client-side `key` to each row), or clear `revealed` in `removeRow`.
  → took the second option: `removeRow` clears `revealed` (indexes shift, so over-masking is the safe side). Test: "masks a secret again when a remove shifts its row" (fails without the clear).
- [x] [minor] web/src/pages/environment/EnvironmentPage.svelte:31-38 — the effect resets `rows` whenever `store.environments` changes. Edit a value, then pick another environment in the status bar (it refetches): the unsaved edits are dropped with no warning. → reset only when `active?.id` changes, or when the grid is not dirty.
  → resets only when `active.id` changes (`shownId`); rename/delete state resets there too. Test: "keeps unsaved edits when the list refetches".
- [x] [minor] web/src/pages/environment/EnvironmentPage.svelte:173 — Delete runs at once with no confirmation, and it deletes the environment with all its (sealed) secrets. → confirm first (bits-ui AlertDialog or a two-step button).
  → two-step button: Delete → "Delete “<name>” and its variables?" + Confirm delete / Cancel.
- [x] [minor] web/src/lib/components/StatusBar.svelte:62,70 — `void env.select(...)` drops the result. If the mutation fails, `store.error` is set but only the Environment page shows it; the status bar stays silent. → show the error (toast or title) or at least keep the old selection visibly.
  → `select()` records `env.error` in a status-bar `role="alert"` (truncated, full text in `title`); the store only refetches on success, so the old name stays. Test added.
- [x] [minor] web/src/lib/components/StatusBar.svelte:18 + EnvironmentPage.svelte:29 — both call `store.start()` on the same singleton. On the Environment page that is two refetches at mount and two `projects.onReset` subscriptions. → start the store once (status bar only) and drop it from the page, or make `start()` ref-counted.
  → took the first option: the status bar owns `start()`; the page test loads the store itself.
- [x] [minor] web/src/pages/environment/EnvironmentPage.svelte:76 — `errorName` is parsed from the first quoted string in the message. A `BAD_INPUT` that quotes nothing (invalid name, value too long) highlights no row, and a quoted name in another position is mis-parsed. The test only covers the duplicate message. → ask the lead whether `extensions` can carry the row (index/name); until then show the message next to the grid and add a test for the unquoted case.
  → the parse now only accepts the two phrases that name a variable (`variable name "…"`, `value of "…"`); anything else highlights no row. The message moved into the grid footer, next to Save. Test: "shows a BAD_INPUT that names no variable without highlighting a row". The `extensions` question is left to the lead (see Handoff follow-ups).
- [x] [nit] web/src/lib/stores/environment.svelte.ts:49 — two overlapping `refresh()` calls can resolve out of order, so an older list can overwrite a newer one. → keep a request counter and ignore stale results.
  → `#request` counter; test "ignores a refresh that answers after a newer one" (fails without it).
- [x] [nit] web/src/pages/environment/EnvironmentPage.svelte:97 — `h-[calc(100vh-140px)]` is a magic number tied to the shell; use the layout's flex height.
  → `h-full min-h-0` (the router renders pages in `main.h-full`).
- [x] Missing tests for the two majors (rename form after switching environment; reveal after removing a row). → added, and both were checked to fail without the fix.
- AC coverage otherwise OK: Global pinned and without actions, grid saved together, status bar select/none, store `variables` override.
- `make check` re-run on the branch: green (the `internal/proxy` flake did not recur).

### Round 2
- Checks: diff stays inside `paths` (plus codegen output). `make check` green on re-run (Go, svelte-check, eslint, prettier,
  345 web tests). The `internal/proxy` flake did not recur.
- All round-1 findings verified in code: rename form tied to `renamingId === active.id`; grid resets only on `active.id`
  change; `removeRow` clears `revealed`; two-step delete; status-bar alert on a failed switch; single `store.start()` owner;
  stricter `BAD_INPUT` row parse; `#request` counter in `refresh`.
- Findings:
- [ ] [nit] web/src/lib/stores/environment.svelte.ts:24 + StatusBar.svelte:22 — `error` is one shared field. A failed switch in the status bar also shows in the Environment page footer, and a failed save shows until the next successful refetch. → harmless now; scope the error per caller if it confuses users.
- [ ] [nit] web/src/lib/components/StatusBar.svelte:22 — the alert stays until the next switch; it is not cleared on project reset. → clear on `projects.onReset` if wanted.
- No blocker or major remain. AC coverage OK. The `extensions` question for `BAD_INPUT` stays with the lead.
