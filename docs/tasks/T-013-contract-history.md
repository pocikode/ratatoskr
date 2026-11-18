---
id: T-013
title: Contract - history queries, subscriptions, completions, colour/note mutations
milestone: M1
role: lead
status: in_progress
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
- [ ] All fields documented; Bytes fields base64; large bodies fetched via separate field with `offset/limit`.
- [ ] `make gen` clean; frontend can render mock history with no backend.

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

## Review (reviewer)
