---
id: T-098
title: Contract - Replay collections, sessions, entries, send; Environments
milestone: M2
role: lead
status: in_progress
depends_on: [T-030]
owner: "claude-code (opus-5.5)"
branch: task/T-098-contract-replay-environment
paths:
  - internal/api/graphql/schema/replay.graphqls
  - internal/api/graphql/schema/environment.graphqls
  - internal/api/graphql/resolvers/stubs_replay.go
  - internal/api/graphql/resolvers/stubs_replay_send.go
  - internal/api/graphql/resolvers/stubs_environment.go
  - internal/api/graphql/resolvers/resolver.go
  - internal/api/graphql/generated/**
  - web/src/lib/api/generated/**
  - web/src/lib/api/mocks/**
  - gqlgen.yml
  - docs/tasks/M2/T-098-contract-replay-environment.md
spec:
  - docs/01-feature-spec.md#F-REP-1, #F-REP-2 (incl. M2 contracts)
  - docs/01-feature-spec.md#F-ENV-1 (incl. M2 contract)
  - docs/03-architecture.md §6
---

## Goal
GraphQL contract for Replay (HTTP sessions only) and Environments, with stubs and mocks, so T-033/T-106/T-037 (backend)
and T-034/T-107/T-103 (frontend) can start in parallel.

## Scope
- In (`replay.graphqls`): `ReplaySessionKind {HTTP}`; `ReplayCollection implements Node {id, name, isDefault, sessions}`;
  `ReplayConnection {host, port, tls, sni}` + input; `ReplayOptions {followRedirects, updateContentLength, timeoutMs,
  applyMatchReplace}` + input (fields optional); `ReplaySession implements Node {id, kind, name, color: RequestColor,
  collection, connection, options, draft: Bytes!, sending: Boolean!, createdAt, entries(first, after): ReplayEntryConnection!}`
  (newest first, `first` 1..1000, default 100); `ReplayEntry implements Node {id, request: HttpRequest!, error: String, createdAt}`.
  Query `replayCollections` (collection order, sessions in position order), `replaySession(id)`.
  Mutation: `createReplayCollection(name)`, `renameReplayCollection(id, name)`, `deleteReplayCollection(id): ID!`,
  `reorderReplayCollections(ids)`, `createReplaySession(input {collectionId, fromRequestId, connection, raw, name})`,
  `updateReplaySession(id, input {name, connection, options, draft})`, `setReplaySessionColor(id, color)`,
  `duplicateReplaySession(id)`, `moveReplaySession(id, collectionId, index)`, `deleteReplaySession(id): ID!`,
  `sendReplaySession(id, input {raw, connection, options}): ReplayEntry!`, `cancelReplaySend(id): Boolean!`.
- In (`environment.graphqls`): `EnvironmentVariable {name, value, secret}` + input; `Environment implements Node {id, name,
  isGlobal, selected, variables}`. Query `environments` (Global first, then name order), `environment(id)`. Mutation
  `createEnvironment(name)`, `renameEnvironment(id, name)`, `deleteEnvironment(id): ID!`,
  `setEnvironmentVariables(id, variables): Environment!` (replaces the whole list), `selectEnvironment(id: ID): Environment`
  (null = none selected; returns the selection).
- In: `resolvers.Deps` fields `Replay`, `ReplaySend`, `Environment` (placeholder interfaces in the stub files; send/cancel
  stubs live in `stubs_replay_send.go` so T-033 and T-106 own separate files); `make gen`; mocks: replay (seed: Default +
  one more collection, 3 sessions, entries; `sendReplaySession` answers a canned response after ~300 ms and honours
  `cancelReplaySend`) and environments (Global + 2 project envs, one secret var), merged into the mock exchange.
- Out: WS/Pipeline sessions, HTTP/2, subscriptions (pages refetch after their own mutations).

## Acceptance criteria
- [ ] Every type/field/argument documented with the F-REP-1/F-REP-2/F-ENV-1 contract rules and error codes: `CONFLICT`
      (delete Default collection, send while sending, rename/delete Global, duplicate environment name), `BAD_INPUT` (empty or
      long names, timeout outside 1000–300000 ms, port outside 1–65535, truncated source request, invalid variable name,
      duplicate variable name, value > 1 MiB, reorder ids not the exact set), `NOT_FOUND` (unknown IDs).
- [ ] Send semantics documented on the field: draft saved first, blocking until done/timeout/cancel, interpolation → M&R →
      Content-Length order, fresh connection, failures as entries with `error`.
- [ ] `make gen` clean; stubs compile; `make check` green.
- [ ] Mocks cover CRUD, move, duplicate `(copy)`, send + cancel, env select/variables (vitest).

## Notes from lead
- Default collection: created lazily by the service, so `replayCollections` never returns an empty list.
- Keep `draft`/`raw` as `Bytes` (byte fidelity); the UI decodes for the editor (T-104).

## Plan (implementer, ≤15 lines)
1. `replay.graphqls`: kinds, collection/session/entry types, connection/options + inputs, entry connection (reuses
   `PageInfo`), queries, mutations; every field documents defaults, limits and error codes from F-REP-1/F-REP-2.
2. `environment.graphqls`: variable + input, environment, queries, mutations with F-ENV-1 rules/codes.
3. `gqlgen.yml`: lazy field resolvers for `ReplayCollection.sessions`, `ReplaySession.collection/entries`,
   `ReplayEntry.request` so list queries stay cheap; `omittable` fields where null ≠ omitted.
4. Stubs: `stubs_replay.go` (CRUD + field resolvers), `stubs_replay_send.go` (send/cancel), `stubs_environment.go`;
   `Deps.Replay`, `Deps.ReplaySend`, `Deps.Environment` placeholder interfaces. `make gen`.
5. Mocks: `replay.ts`, `environment.ts` (+ vitest), merged in `exchange.ts`. `make check`.

## Out-of-scope change needed (if any)

## Handoff (implementer)
- What changed:
- How to verify:
- Known limitations:
- Follow-ups (proposed tasks):

## Review (reviewer)
- Verdict: approve | changes_requested
- Findings:
