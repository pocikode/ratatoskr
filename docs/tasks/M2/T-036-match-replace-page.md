---
id: T-036
title: Match & Replace page - collections, rule list, rule editor, import/export
milestone: M2
role: frontend
status: in_review
depends_on: [T-099]
owner: claude-sonnet-5.5
branch: task/T-036-match-replace-page
paths:
  - web/src/pages/match-replace/*.svelte
  - web/src/pages/match-replace/*.ts
  - web/src/pages/match-replace/*.graphql
  - web/src/pages/match-replace/tester/TesterPanel.svelte   # placeholder only; T-108 owns it afterwards
spec:
  - docs/01-feature-spec.md#F-MR-1 (incl. M2 contract)
  - docs/02-ui-layout.md §3.6
---

## Goal
Users manage Match & Replace collections and rules: create, edit, enable, reorder, move, delete, import and export.

## Scope
- In: three-column layout per docs/02 §3.6 (tester column renders the `TesterPanel` placeholder with prop
  `rule: MatchReplaceRuleInput | null` = the editor's current draft); collections tree with rules (enable checkbox,
  drag to reorder and across collections → `moveMatchReplaceRule`, collection reorder); rule editor form: name, condition
  (`QueryBar` HTTPQL), sources checkboxes, section select → only valid operations, operation → only allowed matchers
  (`ADD` hides the matcher), matcher value, term; save/cancel/delete; RE2 hint next to Regex; import (file picker →
  `importMatchReplace`) and export (download `exportMatchReplace` as `.json`).
- Out: tester behaviour (T-108), Workflow replacer, WebSocket sections.

## Acceptance criteria
- [ ] Section/operation/matcher dependencies follow the T-099 table (unit test over the table).
- [ ] Create, edit, enable toggle, delete rule and collection against the mock; validation errors (`BAD_INPUT`,
      `BAD_QUERY` span) appear on the right field.
- [ ] Drag reorder and move across collections call `moveMatchReplaceRule` with the right index; failure reverts.
- [ ] Export downloads a `.json` file with the mock's export text; import shows the number added or the error.
- [ ] Unsaved edits prompt before switching rules.
- [ ] Component tests; `make check` green.

## Notes from lead
- Reuse `SplitPane`, `QueryBar`, `ContextMenu`; no new dependencies.

## Plan (implementer, ≤15 lines)
1. `matchreplace.graphql` + `bun run codegen` (T-099 schema already on main).
2. `rules.ts`: section/operation/matcher tables (mirror matchreplace.graphqls) + unit test.
3. `matchreplace.svelte.ts` (`MatchReplaceState`): collections, draft + dirty, field errors (BAD_INPUT message → field, BAD_QUERY span → QueryBar), CRUD, enable toggle, move/reorder (optimistic, reload on failure), import/export.
4. Components: `MatchReplacePage` (two nested `SplitPane`s, 30/40/30), `RuleList`, `RuleEditor`, `tester/TesterPanel` placeholder (prop `rule`).
5. Drag/drop via HTML5 DnD plus ↑/↓ buttons for keyboard; right-click menu via `ContextMenu` + registered `mr.*` commands.
6. Tests: rules table, state/page against `createMockClient`. Playwright spec if the mock route is reachable.
Shortcuts: `window.prompt/confirm` for rename, new collection and unsaved-edit guard.

## Out-of-scope change needed (if any)

## Handoff (implementer)
- What changed: Match & Replace page (`web/src/pages/match-replace/`): `RuleList` (collections tree, enable checkbox, HTML5 drag + ↑/↓ buttons, context menu via `mr.*` registry commands, import/export), `RuleEditor` (section→operation→matcher dependencies from `rules.ts`, `ADD` hides matcher, RE2 hint, `QueryBar` condition), `MatchReplaceState` (field errors: `BAD_INPUT` message→field, `BAD_QUERY` span→QueryBar; optimistic enable/move with revert), `tester/TesterPanel` placeholder (prop `rule`). Operations in `matchreplace.graphql`; generated client regenerated (`bun run codegen`).
- How to verify: `make check`; `cd web && bun run dev` then open `/?mock=match-replace` (dev mock). Tests: `rules.test.ts`, `MatchReplacePage.test.ts`.
- Known limitations: `window.prompt/confirm` for new/rename collection, delete collection and the unsaved-edit guard (marked `shortcut:`). Field attribution of `BAD_INPUT` is by message keywords; unmatched messages show at the form bottom. No Playwright spec (page only reachable through the dev mock; no E2E fixture for it yet). Collection drag drop-targets the header only.
- Follow-ups (proposed tasks): dialog component replacing prompt/confirm; E2E spec once T-102 backend is on main; structured `field` in `BAD_INPUT` extensions would remove message matching.

## Review (reviewer)
- Verdict: approve | changes_requested
- Findings:
