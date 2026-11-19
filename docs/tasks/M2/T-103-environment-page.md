---
id: T-103
title: Environment page and status-bar environment switcher
milestone: M2
role: frontend
status: in_progress
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
- [ ] Global listed first and cannot be renamed/deleted (no actions shown).
- [ ] Grid edits are saved together; `BAD_INPUT` for a variable highlights that row with the message.
- [ ] Secret values render masked until revealed; the reveal is per row and resets on navigation.
- [ ] Status bar shows the selected environment and switches it (mock assertion on `selectEnvironment`), including "none".
- [ ] Store test: `variables` resolves selected over Global and updates after selection change.
- [ ] Component tests; `make check` green.

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
- What changed:
- How to verify:
- Known limitations:
- Follow-ups (proposed tasks):

## Review (reviewer)
- Verdict: approve | changes_requested
- Findings:
