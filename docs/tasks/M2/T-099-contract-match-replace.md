---
id: T-099
title: Contract - Match & Replace collections, rules, tester, import/export
milestone: M2
role: lead
status: in_progress
depends_on: [T-098]
owner: "claude-code (opus-5.5)"
branch: task/T-099-contract-match-replace
paths:
  - internal/api/graphql/schema/matchreplace.graphqls
  - internal/api/graphql/resolvers/stubs_matchreplace.go
  - internal/api/graphql/resolvers/resolver.go
  - internal/api/graphql/generated/**
  - web/src/lib/api/generated/**
  - web/src/lib/api/mocks/**
  - gqlgen.yml
  - docs/tasks/M2/T-099-contract-match-replace.md
spec:
  - docs/01-feature-spec.md#F-MR-1 (incl. "Contract (lead, 2026-11-19, M2)")
  - docs/03-architecture.md §6
---

## Goal
GraphQL contract for Match & Replace so T-035/T-102 (backend) and T-036/T-108 (frontend) can start.

## Scope
- In: enums `MatchReplaceSource {PROXY, INTERCEPT, REPLAY, AUTOMATE, WORKFLOW, PLUGIN}`, `MatchReplaceSection
  {REQUEST_FIRST_LINE, REQUEST_METHOD, REQUEST_PATH, REQUEST_QUERY, REQUEST_HEADER, REQUEST_BODY, RESPONSE_FIRST_LINE,
  RESPONSE_STATUS_CODE, RESPONSE_HEADER, RESPONSE_BODY}`, `MatchReplaceOperation {UPDATE_RAW, UPDATE, UPDATE_VALUE, ADD,
  REMOVE}`, `MatchReplaceMatcherKind {FULL, STRING, REGEX, NAME}`, `MatchReplaceMessageKind {REQUEST, RESPONSE}`.
- Types: `MatchReplaceRule implements Node {id, name, enabled, collection, condition: String!, sources, section, operation,
  matcher: MatchReplaceMatcher {kind, value: String!}, term: String!}`; `MatchReplaceCollection implements Node {id, name,
  rules}`; `MatchReplaceRuleInput`; `MatchReplaceTestResult {output: Bytes!, conditionMatched: Boolean!, changed: Boolean!}`.
- Query: `matchReplaceCollections` (ordered, rules ordered), `matchReplaceRule(id)`, `exportMatchReplace: String!`.
- Mutation: `createMatchReplaceCollection(name)`, `renameMatchReplaceCollection`, `deleteMatchReplaceCollection(id): ID!`
  (deletes its rules), `reorderMatchReplaceCollections(ids)`, `createMatchReplaceRule(collectionId, input)`,
  `updateMatchReplaceRule(id, input)`, `setMatchReplaceRuleEnabled(id, enabled)`, `moveMatchReplaceRule(id, collectionId,
  index)`, `deleteMatchReplaceRule(id): ID!`, `importMatchReplace(json: String!): Int!` (collections added),
  `testMatchReplace(rule: MatchReplaceRuleInput!, kind: MatchReplaceMessageKind!, raw: Bytes!): MatchReplaceTestResult!`.
- The valid section × operation table and which matchers each operation accepts are written in the schema docs:
  first line / path / body → `UPDATE_RAW`; method / status code → `UPDATE`; query / header → `UPDATE_RAW`,
  `UPDATE_VALUE`, `ADD`, `REMOVE`. `NAME` only with `UPDATE_VALUE`/`REMOVE`; `ADD` ignores the matcher.
- In: `resolvers.Deps.MatchReplace` placeholder, stubs, `make gen`, mock (2 collections, 4 rules on reserved hosts; tester
  mock does STRING/FULL header and body replacement only, with a `shortcut:` comment).
- Out: Workflow replacer (T-066), WebSocket sections (T-053), subscriptions.

## Acceptance criteria
- [ ] Every type/field/argument documented with the F-MR-1 contract rules and errors: `BAD_INPUT` (invalid section ×
      operation, matcher not allowed, invalid regex with the RE2 message, empty sources, names 1–200, term > 1 MiB,
      > 1000 rules, bad import JSON/version, reorder ids not the exact set), `BAD_QUERY` with span (condition), `NOT_FOUND`.
- [ ] Export/import JSON format documented (`{"version":1,"collections":[…]}`, all or nothing, appends).
- [ ] `make gen` clean; stubs compile; `make check` green.
- [ ] Mock covers CRUD, enable, move, import/export round-trip and the tester (vitest).

## Notes from lead
- `condition` is `String!` (empty = always) to match how `InterceptSettings.query` is typed.
- RE2 limits (no look-around, no backrefs) must be stated on `MatchReplaceMatcherKind.REGEX` (F-MR-1 "document this").

## Plan (implementer, ≤15 lines)
1. `matchreplace.graphqls`: enums, rule/collection/matcher types, input, test result, queries, mutations; every
   rule from F-MR-1 contract (section × operation table, matchers, errors, JSON format) in descriptions.
2. gqlgen.yml: `MatchReplaceCollection.rules` and `MatchReplaceRule.collection` lazy resolvers (like Replay).
3. `stubs_matchreplace.go` returning `ErrNotImplemented`; `Deps.MatchReplace` placeholder; `make gen`.
4. Mock `mocks/matchreplace.ts` (2 collections, 4 rules, reserved hosts) wired in `exchange.ts`; validation + tester
   (STRING/FULL on header/body only, `shortcut:` comment); vitest for CRUD, enable, move, import/export, tester.
5. `make check`, Handoff, `in_review`.

## Out-of-scope change needed (if any)

## Handoff (implementer)
- What changed:
- How to verify:
- Known limitations:
- Follow-ups (proposed tasks):

## Review (reviewer)
- Verdict: approve | changes_requested
- Findings:
