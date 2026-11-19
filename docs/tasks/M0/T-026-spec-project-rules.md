---
id: T-026
title: "Spec: copy the project rules of project.graphqls into docs/01 F-WSP-1"
milestone: M0
role: lead
status: done
depends_on: [T-007]
owner: "claude-code/opus (lead)"
branch: task/T-026-spec-project-rules
paths:
  - docs/01-feature-spec.md
  - internal/api/graphql/schema/project.graphqls
  - docs/tasks/T-026-spec-project-rules.md
spec:
  - docs/01-feature-spec.md#F-WSP-1
  - internal/api/graphql/schema/project.graphqls
---

## Goal
The project contract of T-007 (`project.graphqls`) decides behaviour that F-WSP-1 does not state yet. Make docs/01 the
source of truth before T-006 implements it (T-007 review, minor finding). **Must be done before T-006 starts.**

## Scope
- In: F-WSP-1 text and ACs; keeping schema descriptions consistent with it (descriptions only, no type changes).
- Out: any resolver/service code (T-006), UI (T-009).

## Acceptance criteria
- [x] F-WSP-1 states: renaming a temporary project saves it (it stops being temporary); a temporary project cannot be
      the default (`BAD_INPUT`); duplicating without a name uses "<source name> (copy)"; the project list is ordered by
      most recently opened first; names are trimmed and must not be empty.
- [x] Each rule has an AC line T-006 can test.
- [x] `project.graphqls` descriptions match the spec wording (or link to F-WSP-1); `make gen` leaves no diff besides
      regenerated descriptions.
- [x] `docs/00-overview.md §4` re-read; no third-party product wording.

## Notes from lead
- Operator decided (T-007 review round 1) to track this as its own task. The operator should add T-026 to T-006's
  `depends_on`.

## Plan (implementer, ≤15 lines)

## Out-of-scope change needed (if any)

## Handoff (implementer)
- What changed: F-WSP-1 gained a Rules list and an AC line. It was written after T-006 shipped, so it records
  what `internal/project` and `project.graphqls` already do (including the 200-character and control-character name
  limits from `cleanName`). The schema descriptions already matched, so the schema and generated code are unchanged.
- How to verify: compare docs/01 F-WSP-1 with `project.graphqls` and `internal/project/project.go`; T-006 tests cover
  each rule (`TestCreateRenameValidation`, `TestRenameSavesTemporary`, `TestListOrderAndSetDefault`, `TestDuplicate`,
  `TestDeleteSelected`).
- Known limitations: done after T-006 rather than before it.
- Follow-ups: none.

## Review (reviewer)
