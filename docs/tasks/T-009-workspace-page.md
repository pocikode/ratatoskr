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
  - web/src/lib/stores/project.svelte.ts
spec:
  - docs/01-feature-spec.md#F-WSP-1
  - docs/02-ui-layout.md §1, §3.12
---

## Goal
Users manage projects on the Workspace page (Projects tab; Backups tab placeholder) and switch projects from the top bar; all page stores reset on `projectChanged`.

## Acceptance criteria
- [ ] Create/rename/duplicate/delete/set default/select work against the real API (component tests with mocked client + one Playwright smoke).
- [ ] Switcher is keyboard accessible, searchable, shows the temp project distinctly.
- [ ] Deleting requires typed confirmation of the project name.

## Plan (implementer, ≤15 lines)
- `pages/workspace/project.graphql` + codegen; `ProjectStore` (`project.svelte.ts`): list, mutations, `projectChanged` subscription, `epoch` + `onReset()` for stores to reset on SELECTED.
- `ProjectSwitcher` (bits-ui Combobox) in TopBar; registers `project.create` / `project.select.<id>` palette commands.
- `WorkspacePage` (Projects/Backups tabs, table, row actions) + `ProjectDialog` (create/rename/typed-name delete).
- Component tests with a fake urql client.

## Out-of-scope change needed (if any)
Applied minimally because the task is unfinishable without them: `TopBar.svelte` mounts `<ProjectSwitcher />`; `App.test.ts` line 91 scoped its `combobox` query to the palette (the switcher is now a second combobox). Generated files regenerated via `bun run codegen`. Lead: please confirm or widen `paths`.

## Handoff (implementer)
- **Changed:** project store, switcher, Workspace page/dialog, GraphQL ops, tests (see Plan).
- **Verify:** `make check` green; `cd web && bun run dev` against `make run`, use the top-bar switcher and the Workspace page.
- **Limitations:** (1) **No Playwright smoke**: `@playwright/test` and `web/tests/e2e` do not exist in the repo yet; needs E2E infra first. (2) Row actions (rename/duplicate/delete/default) call the store directly, not the command registry (commands take no per-row context yet); only `project.create` and per-project `project.select.*` are commands. (3) Page stores don't yet subscribe to `projects.onReset()`/`epoch`: no project-scoped stores exist yet; later tasks must hook in. (4) Backups tab is a placeholder. (5) Dialog `$effect`-free; switcher input shows the project name and filters only while open.
- **Follow-ups:** E2E harness task; context-aware commands for row actions.

## Review (reviewer)
