---
id: T-036
title: Match & Replace page - collections, rule list, rule editor, import/export
milestone: M2
role: frontend
status: done
depends_on: [T-099]
owner: claude-sonnet-5.5
branch: task/T-036-match-replace-page
paths:
  - web/src/pages/match-replace/*.svelte
  - web/src/pages/match-replace/*.ts
  - web/src/pages/match-replace/*.graphql
  - web/src/pages/match-replace/tester/TesterPanel.svelte   # placeholder only; T-108 owns it afterwards
  - web/src/lib/api/generated/**   # codegen output only (lead, 2026-11-20)
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
- Lead decisions (2026-11-20, review-limit escalation after round 3):
  - Generated paths: `web/src/lib/api/generated/**` added to `paths` (codegen output only, never hand-edited; same as T-034).
    Verified: `bun run codegen` on `6235548` produces no diff.
  - Browser coverage: not required here. The Match & Replace E2E belongs to T-039 (`web/tests/e2e/match-replace.spec.ts`,
    after T-109 wires T-102). No dev-mock E2E hook needed.
  - The `shortcut:` items (prompt/confirm, keyword-based `BAD_INPUT` field mapping) are accepted for M2.
  - One extra review round (4) is authorized, limited to the round 3 fixes in `6235548`. If round 4 finds only
    minor/nit items, approve and track them as follow-ups.

## Plan (implementer, ≤15 lines)
1. `matchreplace.graphql` + `bun run codegen` (T-099 schema already on main).
2. `rules.ts`: section/operation/matcher tables (mirror matchreplace.graphqls) + unit test.
3. `matchreplace.svelte.ts` (`MatchReplaceState`): collections, draft + dirty, field errors (BAD_INPUT message → field, BAD_QUERY span → QueryBar), CRUD, enable toggle, move/reorder (optimistic, reload on failure), import/export.
4. Components: `MatchReplacePage` (two nested `SplitPane`s, 30/40/30), `RuleList`, `RuleEditor`, `tester/TesterPanel` placeholder (prop `rule`).
5. Drag/drop via HTML5 DnD plus ↑/↓ buttons for keyboard; right-click menu via `ContextMenu` + registered `mr.*` commands.
6. Tests: rules table, state/page against `createMockClient`. Playwright spec if the mock route is reachable.
Shortcuts: `window.prompt/confirm` for rename, new collection and unsaved-edit guard.

## Out-of-scope change needed (if any)
- `web/src/lib/api/generated/graphql.ts` and `gql.ts` are `bun run codegen` output of `matchreplace.graphql`, never hand-edited. Lead: please add `web/src/lib/api/generated/**` (codegen output only) to this task's paths.
- Browser (Playwright) coverage needs a fixture route to the dev mock (`?mock=match-replace`) outside `paths`; recorded as a dependency for the lead (T-102 backend or an e2e mock hook).

## Handoff (implementer)
- What changed: Match & Replace page (`web/src/pages/match-replace/`): `RuleList` (collections tree, enable checkbox, HTML5 drag + ↑/↓ buttons, context menu via `mr.*` registry commands, import/export), `RuleEditor` (section→operation→matcher dependencies from `rules.ts`, `ADD` hides matcher, RE2 hint, `QueryBar` condition), `MatchReplaceState` (field errors: `BAD_INPUT` message→field, `BAD_QUERY` span→QueryBar; optimistic enable/move with revert), `tester/TesterPanel` placeholder (prop `rule`). Operations in `matchreplace.graphql`; generated client regenerated (`bun run codegen`).
- How to verify: `make check`; `cd web && bun run dev` then open `/?mock=match-replace` (dev mock). Tests: `rules.test.ts`, `MatchReplacePage.test.ts`.
- Known limitations: `window.prompt/confirm` for new/rename collection, delete collection and the unsaved-edit guard (marked `shortcut:`). Field attribution of `BAD_INPUT` is by message keywords; unmatched messages show at the form bottom. No Playwright spec (page only reachable through the dev mock; no E2E fixture for it yet). Collection drag drop-targets the header only.
- Follow-ups (proposed tasks): dialog component replacing prompt/confirm; E2E spec once T-102 backend is on main; structured `field` in `BAD_INPUT` extensions would remove message matching.

## Rework (changes_requested round 1)
- Project scope: `MatchReplacePage` is a `{#key project}` wrapper (Replay pattern) around `MatchReplaceShell`; test rerenders with another `projectId`.
- Slow save: outcome/errors apply only to the originating draft; edits made meanwhile are kept (id adopted, baseline = sent form); delayed-mutation test.
- Context menu: right-click sets `mr.menuRule` (row id) instead of selecting; delete acts on that id and `confirm`s naming the rule (editor/palette delete too). Test: declined switch, then menu delete.
- Enable rollback reverts only the failed rule; test with one late failure and one success.
- Move (variables + order) and export (filename + payload) tests strengthened.
- Generated paths and browser coverage raised under Out-of-scope for the lead.

## Rework (round 2)
- Save: `busy` spans mutation + refresh (second submit ignored); draft adopts the saved id before the refresh, so a failed/slow refresh keeps the rule; tests for slow+failed refresh and single create.
- Delete rule/collection: open draft checked after the refresh; test switching during it.
- Condition `BAD_INPUT` rendered under the QueryBar (test); move updates baseline collection id; export test compares the exact mock text.
- Generated paths: not mine to add (lead owns the task `paths`). Item left unticked: lead must authorize `web/src/lib/api/generated/**` (codegen output only).

## Review (reviewer)
- Verdict: changes_requested (reviewed `b16f38f`).
- Verification: `make check` passed, including 415 web tests. The diff changes no proxy, store or raw HTTP files, so the integration target was outside the review procedure. Live Match & Replace remains dependent on T-102.
- Findings:
  - [x] [major] web/src/pages/match-replace/MatchReplacePage.svelte:34 — The page loads on mount without tracking the active project. A project selection while this route stays mounted leaves the previous project's collections and draft visible → Scope the page state to the active project, following the Replay page pattern, and test a project change while the page is open.
  - [x] [major] web/src/pages/match-replace/matchreplace.svelte.ts:213 — A slow save reselects the saved rule after awaiting the mutation and reload. Users can switch to another rule and start editing it while the request is pending, then lose those edits when the old save finishes. A late failure also attaches errors to the current draft → Keep the originating draft stable during save or apply completion and errors only to the matching, unchanged draft. Test with a delayed response.
  - [x] [major] web/src/pages/match-replace/RuleList.svelte:161 — Right-clicking another rule while editing a dirty rule opens its context menu even when the user declines the discard prompt. Delete then uses the still-selected draft ID, so choosing it deletes the previously selected rule → Bind row actions to the row ID and give destructive actions a confirmation that names the rule. Test declining the switch before using the menu.
  - [x] [major] web/src/pages/match-replace/matchreplace.svelte.ts:235 — Rolling back an enable failure restores a snapshot of every collection. If a different toggle succeeds before the failure arrives, the snapshot also rolls back that successful toggle in the UI → Serialize overlapping list mutations or reconcile each mutation without undoing successful updates. Cover mixed success and failure with delayed mock responses.
  - [x] [major] web/src/lib/api/generated/graphql.ts:509 — This file and `web/src/lib/api/generated/gql.ts` are outside the task's declared paths. The handoff identifies both as codegen output → Have the lead authorize the generated paths in the task before resubmitting.
  - [x] [minor] web/src/pages/match-replace/MatchReplacePage.test.ts:123 — The successful drop test checks membership after dropping onto the only rule in the destination. An implementation that always appends would pass. The export test checks the version alone → Assert the move variables and resulting order, and verify the download filename and full mock payload.
  - [x] [minor] docs/tasks/M2/T-036-match-replace-page.md:65 — The handoff records no browser test for the user-visible page → Get the browser fixture path authorized and cover the workflow against the dev mock, or record the browser coverage dependency explicitly with the lead.

### Round 2

- Verdict: changes_requested (reviewed `b64cdc2`).
- Verification: `make check` passed, including 419 web tests. No proxy, store or raw HTTP files changed. Browser E2E was not run.
- The rework covers project switching, successful saves that finish after a rule switch, row-bound context-menu deletion, and rollback when a different rule's toggle succeeds. The browser fixture dependency is recorded in the handoff.
- [x] [major] web/src/lib/api/generated/graphql.ts:55 — This file and `web/src/lib/api/generated/gql.ts` still fall outside the declared paths. The handoff requests authorization → Have the lead add the generated paths before resubmitting.
- [x] [major] web/src/pages/match-replace/matchreplace.svelte.ts:210 — Save becomes available before the refresh completes and before a new draft adopts its saved ID. A second save during a slow refresh creates a duplicate. If the refresh fails, the successful create can disappear from the editor because selection uses the stale list → Reconcile the draft with the mutation result and guard duplicate submits through reconciliation. Test delayed and failed refreshes after a successful create.
- [x] [major] web/src/pages/match-replace/matchreplace.svelte.ts:240 — Delete records whether the open draft belongs to the deleted rule before awaiting a refresh. If the user opens and edits another rule during that refresh, the old delete clears the new draft. Collection deletion has the same race at line 330 → Check the current draft against the deleted entity at completion and cover a switch during the refresh.
- [x] [major] web/src/pages/match-replace/RuleEditor.svelte:50 — The form renders only the QueryBar's span error for Condition. A condition over 64 KiB returns `BAD_INPUT` and sets `mr.errors.condition`, which the form never renders → Show the condition's field error and cover this case alongside the existing `BAD_QUERY` test.
- [x] [minor] web/src/pages/match-replace/matchreplace.svelte.ts:304 — Moving the open rule updates its draft collection ID but leaves the old ID in the baseline. A clean draft then prompts the user to discard changes → Update the baseline collection ID while preserving any actual edits.
- [x] [minor] web/src/pages/match-replace/MatchReplacePage.test.ts:177 — Export assertions cover the filename, version, and collection names. Omitting rule fields could still pass those assertions and the subsequent import count check → Compare the downloaded text with the mock's export response.

### Round 3

- Verdict: changes_requested (reviewed `e4e23c5` against `69a3e46`).
- Verification: `make check` passed, including 422 web tests. `git diff --check main...HEAD` passed. Browser E2E was not run and remains tracked under Out-of-scope. The integration target was not run because the diff changes no proxy, store or raw HTTP files.
- The rework prevents a second create during refresh, renders condition validation, updates the draft baseline after a move, and checks the exact export payload.
- [x] [major] web/src/pages/match-replace/matchreplace.svelte.ts:237 — Updating an existing rule can succeed while the subsequent list refresh fails. The old rule still exists in the cached list, so the existence check passes and `select` silently replaces the saved values with the old values → Reconcile the list and clean draft from the mutation result. Test an existing-rule update followed by a failed refresh.
- [x] [major] web/src/pages/match-replace/MatchReplaceShell.svelte:88 — Dismissing a row menu leaves its ID in `mr.menuRule`. After selecting a different rule, running Delete from the command palette still targets the dismissed menu's rule → Pass the row ID in the menu's command context and use the active draft for palette invocations. Cover dismissing the row menu before running the palette command.
- [x] [major] web/src/lib/api/generated/graphql.ts:55 — Round 2's path finding remains open for this file and `web/src/lib/api/generated/gql.ts` → Have the lead authorize the generated paths for codegen output.
- [x] [minor] web/src/pages/match-replace/MatchReplacePage.test.ts:159 — The delete race test can assert the new draft before the deferred refresh has applied, allowing it to pass even if the old delete clears the draft afterward → First wait for the deleted row to disappear, then assert that the new draft remains.
- Rework round 3: update result is patched into the list before the refresh (failed refresh keeps saved values); `menuRule` state removed, row id travels in the menu's command context and palette runs use the open rule; delete-race test waits for the row to vanish first. Tests added for each. The generated-paths item stays unticked: lead authorization required.
- Lead escalation: Round 3 reached the review limit in docs/07 §7. The lead owns the pending decision on generated paths and coordination of the remaining fixes.
- Lead resolution (2026-11-20): generated paths authorized; browser coverage moved to T-039; round 4 review authorized (see Notes from lead).

### Round 4

- Verdict: approve (reviewed `53e2dd4` against `69a3e46`, limited to the fixes in `6235548` and the lead decisions).
- Verification: `make check` passed, including all 424 web tests. `git diff --check main...HEAD` passed. The diff is within the authorized paths plus the task record. Browser E2E is assigned to T-039. The integration target was skipped because the diff changes no proxy, store or raw HTTP files.
- The update mutation result replaces the cached rule before refresh. Row-menu deletion uses the row ID from the command context, and palette deletion uses the open draft. The delete-race test waits for the deleted row to disappear before checking the surviving draft. The lead authorized the generated paths for codegen output.
- [ ] [minor] web/src/pages/match-replace/MatchReplacePage.test.ts:317 — The regression test can observe the updated row as soon as the mutation patches the list, before the failing refresh finishes → As a follow-up, wait for the refresh error alert before asserting that the editor and list retain the saved name.
