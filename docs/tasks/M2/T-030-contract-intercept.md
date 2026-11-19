---
id: T-030
title: Contract - intercept queue and settings, HttpRequest.editedBy
milestone: M2
role: lead
status: in_review
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
- [x] Every type, field and argument documented with the F-INT-1 contract rules, incl. error codes: `NOT_FOUND` (unknown or
      already released item, unknown `scopeId`), `BAD_INPUT` (unparsable `raw` with the parse error; `dropStatusCode`
      outside 100–599), `BAD_QUERY` with span (bad `query`). States that `queuing` is not persisted.
- [x] `make gen` clean (`git diff --exit-code` after a second run); stubs compile; `make check` green.
- [x] Mock: settings round-trip; forward/drop remove items and emit `REMOVED`; queuing → false releases all; a mock
      timer adds an item so the badge can be seen live (opt-in flag, off in tests). Vitest covers these.
- [x] Mock history: `editedBy`/`editedFrom` resolve both ways for the seeded pair.

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
  - `intercept.graphqls` (new): `InterceptItemKind`, `InterceptSettings`, `InterceptSettingsInput`, `InterceptItem`,
    `InterceptQueueChangeKind`/`InterceptQueueChange`; queries `interceptSettings`, `interceptQueue`; mutations
    `updateInterceptSettings`, `forward/dropInterceptItem`, `forward/dropAllInterceptItems`; subscriptions
    `interceptQueueChanged`, `interceptSettingsChanged`. Every type, field and argument documented, with error codes.
  - `history.graphqls`: `HttpRequest.editedBy`; F-HIST-4 pair model on `HttpRequest`; `editedFrom`, `INTERCEPT`,
    `isDropped` (a dropped response keeps the server's response) and `response` (null while held) docs aligned.
  - `gqlgen.yml`: `editedBy` resolver; `InterceptSettingsInput.scopeId` is `graphql.Omittable[*string]` so the
    backend can tell `scopeId: null` (clear) from omitted (keep). The other input fields treat null as omitted (documented).
  - Resolvers: `stubs_intercept.go` (`type InterceptService any` placeholder, lint wants `any` over `interface{}`;
    all intercept fields return `ErrNotImplemented`), `stubs_history_edited.go`, `Deps.Intercept`.
  - Mocks: `mocks/intercept.ts` merged into the exchange; new `MockExchangeOptions.interceptLiveIntervalMs` (default 0)
    holds a request on a timer. The history mock gets `editedBy` (both sides of every seeded pair, cleared on delete)
    and `addRecord`/`updateRecord`, so held items are real history records and forward/drop emit `requestUpdated`.
- How to verify: `make gen` twice → `git diff --exit-code` clean; `make check`; `cd web && bunx --bun vitest run src/lib/api/mocks`.
- Known limitations:
  - Intercept mock starts **queuing with requests and responses on** (unlike the real defaults) so the seeded items
    make sense. It seeds lazily on the first intercept operation, so history-only clients and tests keep their row counts.
  - Mock shortcuts (commented): `raw` validation checks only the start line and the header terminator; `query`
    validation flags only an unterminated string; a forward with edits stores no edited record.
  - Calling `editedBy` against the real server returns `NOT_IMPLEMENTED` until T-100. Frontend queries that select it
    fail there until then.
  - `make check` passed; on the first run `src/pages/sitemap/*` failed once under heavy parallel load (158 s run),
    and it passed on both reruns. That test is not touched here.
- Follow-ups (proposed tasks): none. T-031 replaces `stubs_intercept.go`; T-100 replaces `stubs_history_edited.go`.

## Review (reviewer)
- Verdict: approve | changes_requested
- Findings:
