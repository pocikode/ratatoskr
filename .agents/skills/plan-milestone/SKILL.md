---
name: plan-milestone
description: "Lead only: expand a milestone into task files: /plan-milestone M#"
---

`$ARGUMENTS` below = the id the user passed with this skill (e.g. `$task T-014`, `$plan-milestone M3`).

You are the lead. Expand milestone $ARGUMENTS from `docs/08-roadmap.md` into task files.
- Re-read the relevant parts of `docs/01-feature-spec.md`, `docs/02-ui-layout.md`, `docs/03-architecture.md`.
- Create `docs/tasks/$ARGUMENTS/T-###-slug.md` (one folder per milestone) from `docs/tasks/TEMPLATE.md` for every row of the milestone (split rows that are too big; keep ≤ ~600 changed lines each).
- Ensure tasks that can run in parallel have non-overlapping `paths`; the contract task comes first and others depend on it.
- Each AC must be testable. Link exact spec sections. Update the roadmap table.
- Commit as `docs($ARGUMENTS): plan milestone`.
