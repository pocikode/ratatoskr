---
id: T-016
title: History service and resolvers with live filtered subscriptions
milestone: M1
role: backend
status: in_progress
depends_on: [T-011, T-013, T-015]
owner: "claude-code/sonnet-5.5"
branch: task/T-016-history-service
paths:
  - internal/history/**
  - internal/api/graphql/resolvers/history*.go
spec:
  - docs/01-feature-spec.md#F-HIST-1..4
  - docs/03-architecture.md §5.2, §6, §7
---

## Goal
Implement the T-013 contract: cursor pagination with sort, query+scope filtering, totals, colour/note, delete-by-query, completions (distinct values),
and `requestAdded` subscriptions filtered server-side with the evaluator.

## Acceptance criteria
- [ ] Pagination stable under concurrent inserts (cursor = sort key + id).
- [ ] Subscription filter uses evaluator; slow subscriber gets `stale` resync instead of blocking capture (test).
- [ ] Delete-by-query deletes responses/headers too (FK cascade) and returns count.

## Plan (implementer, ≤15 lines)
Scope decision (operator): stay in `paths`; whatever needs edits outside them is recorded below and the task ends `blocked`.
1. `internal/history/service.go`: `Service{DB, Bus, Scope}`; domain `Request`/`Response` rows (no raw bytes), parse → `httpql.Compile` → one SQL over `requests r LEFT JOIN responses s`; `core.Error` → `apperr.Query(span)`.
2. `list.go`: `List(Page{First|Last, After|Before, Query, ScopeID, Sort})`. Every sort field maps to a never-NULL key expression (int or text); cursor = base64url(JSON{field, key, id}); keyset `(key op ? OR (key = ? AND r.id op ?))`; fetch n+1 for `hasMore`; `last/before` queries reversed and flipped. `Count` for totalCount.
3. `Get`, `Head`/`Body(offset, limit, decode)`: stored raw is re-parsed with `rawhttp` (chunked), gzip/deflate via stdlib. br/zstd need a dependency not in go.mod: returned as stored (follow-up).
4. `mutate.go`: `SetColor`, `SetNote`, `Delete(ids | query)` (FK cascade, batches of 1000 IDs in a `Deleted` event). Events: add `Deleted` kind + `IDs` to `Event`.
5. `subscribe.go`: `Added`/`Updated`/`Deleted` streams over `events.Bus`; filter with `httpql.Eval` on the loaded message; `Msg.Stale` ends the stream (never blocks capture).
6. `complete.go`: `Complete(language, text, cursor)` (namespaces, fields, operators, distinct values top 50, sources, colours; never errors).
7. Tests: table-driven + `testing/synctest`; stale-subscriber, delete cascade, cursor stability under concurrent inserts.
8. Resolvers (`history*.go`) cannot be written inside `paths`: see below.

## Out-of-scope change needed (if any)
- `internal/api/graphql/resolvers/resolver.go`: add `History HistoryService` to `Deps` (resolvers cannot reach the service otherwise).
- `internal/app/app.go`: build `history.Service` and pass it in `resolvers.Deps` (not in `paths`).
- `gqlgen.yml` (lead): `HttpResponse.head/body` resolvers receive only `*generated.HTTPResponse`, which carries no request ID. Add `models.HttpResponse.extraFields: { requestID: { type: int64 } }`, then `make gen`, so `Head`/`Body` can find the row.
- go.mod: `github.com/klauspost/compress` + `github.com/andybalholm/brotli` (approved in docs/05) for br/zstd display decoding.
- `docs/03-architecture.md §6`: list `requestUpdated(query, scopeId)`/`requestsDeleted` (already noted by T-013).

## Handoff (implementer)

## Review (reviewer)
