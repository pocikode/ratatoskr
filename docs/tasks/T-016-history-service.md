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
  # added by lead 2026-11-18 (unblock): Deps.History field, delete the stubs, wire the service, approved compression deps
  - internal/api/graphql/resolvers/resolver.go
  - internal/api/graphql/resolvers/stubs_history.go
  - internal/app/app.go
  - internal/app/app_test.go
  - go.mod
  - go.sum
spec:
  - docs/01-feature-spec.md#F-HIST-1..4
  - docs/03-architecture.md §5.2, §6, §7
---

## Goal
Implement the T-013 contract: cursor pagination with sort, query+scope filtering, totals, colour/note, delete-by-query, completions (distinct values),
and `requestAdded` subscriptions filtered server-side with the evaluator.

## Acceptance criteria
- [x] Pagination stable under concurrent inserts (cursor = sort key + id). `TestListCursorSurvivesConcurrentInserts`, `TestListPagesAreConsistentForEverySort` (service level).
- [x] Subscription filter uses evaluator; slow subscriber gets `stale` resync instead of blocking capture (test). `TestSubscribeAddedFiltersWithEvaluator`, `TestSlowSubscriberIsResyncedNotBlocking` (service level).
- [x] Delete-by-query deletes responses/headers too (FK cascade) and returns count. `TestDeleteByQueryCascades` (service level).

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

## Lead resolution (2026-11-18)
- `gqlgen.yml`: `HttpResponse.extraFields.RequestID int64` (not in the schema); `make gen` done. Resolvers set it when building `HttpResponse`, `Head`/`Body` read it.
- `docs/03-architecture.md §6`: subscription list now `requestAdded(query, scopeId)`, `requestUpdated(query, scopeId)`, `requestsDeleted`.
- `paths` widened (see frontmatter): add `History HistoryService` to `resolvers.Deps` (interface in `resolvers/history.go`, nil → `NOT_IMPLEMENTED` like `Projects`), delete `stubs_history.go`, build `history.NewService` in `internal/app` and pass it in `buildHandler`.
- br/zstd: `klauspost/compress` + `andybalholm/brotli` are already approved in `docs/05` §deps, no ADR needed. Add them in this task.
- Next: implement the resolvers and the remaining Handoff follow-ups, `make check`, then `in_review`.

## Handoff (implementer)
- Status: **blocked** on the out-of-scope items above. The operator chose "stay in lane": everything inside `paths` is done, the GraphQL resolvers (`resolvers/history*.go`) are not, because they cannot reach the service without the `Deps.History` field and the `HttpResponse` request ID. `stubs_history.go` is untouched and still returns `NOT_IMPLEMENTED`.
- What changed (`internal/history`, `make check` green):
  - `service.go` `Service`/`NewService(ServiceDeps{DB, Bus, Scope})`, domain `Request`/`ResponseInfo`, `Get`; query+scope → `httpql.Compile` (`core.Error` → `BAD_QUERY` with span).
  - `list.go` `List(PageArgs)`: keyset cursor (base64url JSON of sort field, key, id), all 16 sort fields via never-NULL key expressions, `first/after` and `last/before`, `hasNext/hasPrevious`, `Total`.
  - `body.go` `Head`, `Body(offset, limit ≤ 4 MiB, decode)`: raw window in SQL; decode re-parses with `rawhttp` (chunked) + stdlib gzip/deflate, capped at 256 MiB decoded.
  - `mutate.go` `SetColor`, `SetNote`, `Delete(ids | query)` (one tx, FK cascade, `Deleted` events in batches ≤ 1000 IDs).
  - `subscribe.go` `SubscribeAdded/Updated/Deleted`: filter with `httpql.Eval`; a stale marker from `events.Bus` closes the stream (resolver ends the subscription, client refetches).
  - `complete.go` `Complete(lang, text, cursor)`: namespaces, fields, operators, closed value sets, distinct project values (top 50), presets; never errors on text.
  - `capture.go`: `Event` gained `Deleted` kind and `IDs` (backward compatible).
- How to verify: `go test -race ./internal/history`, `go test ./internal/history -fuzz FuzzComplete -fuzztime 15s`, `make check`.
- Limitations: `totalCount` is an uncached `count(*)` per page (`shortcut:` in list.go). `br`/`zstd` decode needs the approved compression deps in go.mod (shown as stored until then). `scopeId` always yields `NOT_FOUND` until T-018 provides `ServiceDeps.Scope`. `preset:` in queries is not expanded (BAD_QUERY) until the preset backend lands. Subscriptions do not follow a project switch. Colour is stored as the enum name (`RED`), so `req.color.eq:"RED"`.
- Follow-ups: lead applies the out-of-scope items; then `resolvers/history.go` maps `history.Service` onto the generated types (ID parse, `Colors`↔`RequestColor`, cursor passthrough, `NotFound` → null for `request(id)`), deletes `stubs_history.go`, and `internal/app` builds `history.NewService`.

## Review (reviewer)
