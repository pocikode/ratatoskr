---
id: T-030
title: Contract - intercept queue and settings, HttpRequest.editedBy
milestone: M2
role: lead
status: in_progress
depends_on: []
owner: "claude-code (opus-5.5)"
branch: task/T-030-contract-intercept
paths:
  - internal/api/graphql/schema/intercept.graphqls
  - internal/api/graphql/schema/history.graphqls
  - internal/api/graphql/resolvers/stubs_intercept.go
  - internal/api/graphql/resolvers/stubs_history_edited.go
  - internal/api/graphql/resolvers/resolver.go
  - internal/api/graphql/generated/**
  - web/src/lib/api/generated/**
  - web/src/lib/api/mocks/**
  - gqlgen.yml
  - docs/tasks/M2/T-030-contract-intercept.md
spec:
  - docs/01-feature-spec.md#F-INT-1 (incl. "Contract (lead, 2026-11-19, M2)")
  - docs/01-feature-spec.md#F-HIST-4
  - docs/03-architecture.md §4, §6
---

## Goal
GraphQL contract for Intercept (settings, queue, forward/drop, live updates) and the `editedBy` link of edited pairs, with
stubs and mocks, so T-031/T-100 (backend) and T-032/T-038 (frontend) can start in parallel.

## Scope
- In (`intercept.graphqls`): `InterceptItemKind {REQUEST, RESPONSE}`; `InterceptSettings {queuing, requests, responses,
  query: String!, scopeId: ID, dropStatusCode: Int!}`; `InterceptSettingsInput` (every field optional, `scopeId: null`
  clears); `InterceptItem implements Node {id, kind, request: HttpRequest!, raw: Bytes!, heldAt: Time!}` (`raw` = the held
  request or response bytes); `InterceptQueueChange {kind: ADDED|REMOVED, item: InterceptItem!}`.
  Query `interceptSettings`, `interceptQueue(kind: InterceptItemKind): [InterceptItem!]!` (FIFO).
  Mutation `updateInterceptSettings(input)`, `forwardInterceptItem(id, raw: Bytes, updateContentLength: Boolean! = true): ID!`,
  `dropInterceptItem(id): ID!`, `forwardAllInterceptItems(kind): Int!`, `dropAllInterceptItems(kind): Int!` (null kind = both).
  Subscription `interceptQueueChanged`, `interceptSettingsChanged`.
- In (`history.graphqls`): `HttpRequest.editedBy: HttpRequest` (field resolver in `gqlgen.yml`), docs of `editedFrom`,
  `HttpRequest`, `RequestSource.INTERCEPT` aligned with the F-HIST-4 pair model.
- In: `resolvers.Deps.Intercept InterceptService` (placeholder `interface{}` declared in `stubs_intercept.go`; nil →
  `NOT_IMPLEMENTED`); stubs return `apperr.ErrNotImplemented`; `make gen`; in-memory intercept mock merged into the mock
  exchange (seed: 2 requests + 1 response held, reserved hosts only) and one edited pair in the history mock.
- Out: WebSocket items (T-053), "add to scope" / workflow actions, any backend logic.

## Acceptance criteria
- [ ] Every type, field and argument documented with the F-INT-1 contract rules, incl. error codes: `NOT_FOUND` (unknown or
      already released item, unknown `scopeId`), `BAD_INPUT` (unparsable `raw` with the parse error; `dropStatusCode`
      outside 100–599), `BAD_QUERY` with span (bad `query`). States that `queuing` is not persisted.
- [ ] `make gen` clean (`git diff --exit-code` after a second run); stubs compile; `make check` green.
- [ ] Mock: settings round-trip; forward/drop remove items and emit `REMOVED`; queuing → false releases all; a mock
      timer adds an item so the badge can be seen live (opt-in flag, off in tests). Vitest covers these.
- [ ] Mock history: `editedBy`/`editedFrom` resolve both ways for the seeded pair.

## Notes from lead
- Same layout as `scope.graphqls`: extend `Query`/`Mutation`/`Subscription`, no new root files.
- Contract tasks T-030 → T-098 → T-099 run one after another (they share generated files and `mocks/exchange.ts`).
- Backend tasks later move the placeholder interface into their own resolver file and delete the stub file.

## Plan (implementer, ≤15 lines)
1. `intercept.graphqls`: types, input, queue change, Query/Mutation/Subscription extensions; docs carry the F-INT-1 rules and error codes.
2. `history.graphqls`: `editedBy`, pair-model docs on `HttpRequest`, `editedFrom`, `INTERCEPT`, `isDropped`, `response`.
3. `gqlgen.yml`: `editedBy` field resolver; `InterceptSettingsInput.scopeId` omittable (null clears vs omitted keeps).
4. `stubs_intercept.go` (placeholder `InterceptService`, all fields `ErrNotImplemented`), `stubs_history_edited.go`, `Deps.Intercept`.
5. `make gen` twice, diff clean.
6. Mocks: `intercept.ts` (settings, FIFO queue, forward/drop/all, release on toggle off, subscriptions, opt-in timer)
   merged into the exchange; `editedBy` in the history mock. Vitest `intercept.test.ts`.
7. `make check`, handoff.

## Out-of-scope change needed (if any)

## Handoff (implementer)
- What changed:
- How to verify:
- Known limitations:
- Follow-ups (proposed tasks):

## Review (reviewer)
- Verdict: approve | changes_requested
- Findings:
