---
id: T-036
title: Match & Replace page - collections, rule list, rule editor, import/export
milestone: M2
role: frontend
status: todo
depends_on: [T-099]
owner: ""
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

## Out-of-scope change needed (if any)

## Handoff (implementer)
- What changed:
- How to verify:
- Known limitations:
- Follow-ups (proposed tasks):

## Review (reviewer)
- Verdict: approve | changes_requested
- Findings:
