---
id: T-013
title: Contract - history queries, subscriptions, completions, colour/note mutations
milestone: M1
role: lead
status: in_review
depends_on: [T-007]
owner: "claude-code/opus-5.5"
branch: task/T-013-contract-history
paths:
  - internal/api/graphql/schema/history.graphqls
  - internal/api/graphql/schema/query.graphqls
  - internal/api/graphql/resolvers/stubs_history.go
  - internal/api/graphql/generated/**
  - web/src/lib/api/generated/**
  - web/src/lib/api/mocks/**
  - gqlgen.yml
  - web/codegen.ts
  - web/package.json
  - web/bun.lock
  - docs/04-query-language.md
  - docs/06-frontend-conventions.md
  - docs/tasks/T-013-contract-history.md
spec:
  - docs/01-feature-spec.md#F-HIST-1..4
  - docs/04-query-language.md §7
  - docs/03-architecture.md §6
---

## Goal
Schema for `requests(first, after, query, scopeId, sort)` connection, `request(id)`, `requestAdded(query, scopeId)` / `requestUpdated` subscriptions,
`queryCompletions`, `setRequestColor`, `setRequestNote`, `deleteRequests(query)`; stub resolvers; UI mocks with realistic fake data.

## Acceptance criteria
- [x] All fields documented; Bytes fields base64; large bodies fetched via separate field with `offset/limit`.
- [x] `make gen` clean; frontend can render mock history with no backend.

## Plan (implementer, ≤15 lines)
1. `history.graphqls`: `HttpRequest`/`HttpResponse` (Node), `head` bytes + `body(offset, limit, decode)` chunks, colour/source enums,
   `requests` connection (`first/after`, `last/before`, `query`, `scopeId`, `sort`) + `totalCount`, `request(id)`.
2. Subscriptions `requestAdded(query, scopeId)`, `requestUpdated(query, scopeId)` (with `matches`), `requestsDeleted` (ID batches).
3. `queryCompletions(language, text, cursor)` with replace range; spec it in docs/04 §7.
4. Mutations `setRequestColor(ids, color)`, `setRequestNote(id, note)`, `deleteRequests(ids | query, scopeId)`.
   `Query` is extended in `history.graphqls` (project.graphqls pattern), so no `query.graphqls`.
5. `gqlgen.yml`: force resolvers for `head`/`editedFrom` so the backend loads them lazily. Stubs in `stubs_history.go`.
6. Mocks: codegen also emits the schema SDL (`schema-ast`, same codegen family, added as a direct devDep and listed in docs/06);
   `mocks/` executes operations with graphql-js against that schema + seeded fake data, exposed as a urql exchange.

## Out-of-scope change needed (if any)

## Handoff (implementer)
- What changed:
  - `schema/history.graphqls`: `HttpRequest`/`HttpResponse` (`head` bytes, `bodyLength`, `body(offset, limit ≤ 4 MiB, decode)`),
    `requests` connection (`first/after`, `last/before`, `query`, `scopeId`, `sort`, `totalCount`), `request(id)`,
    `queryCompletions` (returns `{from, to, items}`), `setRequestColor(ids, color)`, `setRequestNote`, `deleteRequests(ids | query, scopeId)`,
    subscriptions `requestAdded`, `requestUpdated` (with `matches`), `requestsDeleted` (ID batches ≤ 1000). Query/Mutation/Subscription
    are extended there (no `query.graphqls`, same as `project.graphqls`).
  - `gqlgen.yml`: `head`, `body`, `editedFrom` are field resolvers so list queries never load raw bytes. Stubs: `resolvers/stubs_history.go`
    (`//nolint:revive` on `HttpRequest()`/`HttpResponse()`: names fixed by gqlgen's `ResolverRoot`).
  - docs/04 §7: completion offsets are UTF-8 bytes, replace range, `isExtension`, ≤ 100 items, never errors.
  - Web: codegen also emits `generated/schema.graphql` (`@graphql-codegen/schema-ast`, now a direct devDep; docs/06 updated).
    `lib/api/mocks`: `createHistoryMock` (seeded deterministic fake data, reserved hosts only, edited copies, live adds every 2 s),
    `mockHistoryExchange()` (runs the real schema with graphql-js; forwards operations on other root fields), `createMockClient()`.
- How to verify: `make gen` (no diff), `make check`; `bunx --bun vitest run src/lib/api/mocks` in `web/`.
- Limitations: mock HTTPQL is a substring match of quoted literals (see `shortcut:` comment); mock completions assume ASCII;
  `scopeId` is accepted but ignored by the mock (scope presets have no contract yet). Cursors are opaque, so the virtualized
  table (T-017) loads pages as it scrolls; jumping to the end uses `last`.
- Follow-ups: backend history task replaces `stubs_history.go` with `history.go`; T-017 wires `mockHistoryExchange` behind its dev flag.

## Review (reviewer)
