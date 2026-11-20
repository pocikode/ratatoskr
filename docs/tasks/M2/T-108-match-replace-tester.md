---
id: T-108
title: Match & Replace tester panel with line diff
milestone: M2
role: frontend
status: todo
depends_on: [T-036]
owner: ""
branch: task/T-108-match-replace-tester
paths:
  - web/src/pages/match-replace/tester/**
  - web/src/lib/utils/diff.ts
  - web/src/lib/utils/diff.test.ts
spec:
  - docs/01-feature-spec.md#F-MR-1 (tester)
  - docs/02-ui-layout.md §3.6
---

## Goal
The tester column runs the draft rule against a request or response and shows before/after with changed lines highlighted.

## Scope
- In: `TesterPanel.svelte` (prop `rule` from T-036): input = paste or "pick request" (recent history rows via `requests`,
  newest first, 50) with Request/Response switch; calls `testMatchReplace` debounced (300 ms) on rule or input change;
  shows "condition matched / not matched" and "no change"; side-by-side or unified line diff; `diff.ts` — a small
  line-level LCS/Myers diff (no new dependency).
- Out: word-level diff, workflow replacer.

## Acceptance criteria
- [ ] `diff.ts` unit tests: identical, insert, delete, replace, empty inputs, CRLF lines.
- [ ] Changing the rule or input triggers one `testMatchReplace` call after 300 ms (fake timers).
- [ ] Picking a history row loads its request (or response) bytes.
- [ ] Changed lines are marked by more than colour (prefix `+`/`-` or icon) for accessibility.
- [ ] Inputs over 1 MiB show a notice and skip the diff (output still shown).
- [ ] Inputs over 12 MiB (pasted or picked) are not sent; a notice says the tester takes at most 12 MiB
      (F-MR-1 size budget, set by T-099 review).
- [ ] The draft is sent as `MatchReplaceTestRuleInput`: `name`, `enabled` and `sources` are dropped from the
      editor's `MatchReplaceRuleInput` (GraphQL rejects unknown input fields), so a long draft name cannot push the
      request past the 32 MiB API cap.
- [ ] A `BAD_INPUT` "tester output must be at most 16 MiB" result shows that message in place of the output
      (mock: 64 KiB of `b` with term of 300 bytes).
- [ ] Component tests against the mock; `make check` green.

## Notes from lead
- Cap the diff input (1 MiB, line count ≤ 20k) so O(n·d) stays responsive.

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
