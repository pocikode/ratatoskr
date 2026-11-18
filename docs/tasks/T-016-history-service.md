---
id: T-016
title: History service and resolvers with live filtered subscriptions
milestone: M1
role: backend
status: in_review
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
- Status: `in_review`. Rebased on main (T-018 included). `make check` green.
- What changed: `internal/history` (service, keyset list, head/body, mutations, subscriptions, completions; see earlier commits) plus, after the lead unblock:
  - `resolvers/history.go`: `HistoryService` interface, `Deps.History` (nil → `NOT_IMPLEMENTED`), all query/mutation/subscription/field resolvers; `stubs_history.go` deleted. `HttpResponse.RequestID` keys `head`/`body`; `editedFrom` loads the original (null if deleted); `request(id)` is null for unknown/non-numeric IDs.
  - `internal/app`: builds `history.NewService` and passes it in `resolvers.Deps`.
  - `body.go`: br (`andybalholm/brotli`) and zstd (`klauspost/compress`, memory-capped) display decoding; both added to go.mod.
  - Tests: `resolvers/history_test.go` (end-to-end through the real handler), br/zstd cases in `service_test.go`.
- How to verify: `go test -race ./internal/history ./internal/api/...`, `make check`.
- Limitations: `ServiceDeps.Scope` is not wired in `internal/app` (no scope-to-AST conversion exists yet), so `scopeId` yields `NOT_FOUND`; `preset:` in queries still `BAD_QUERY`. `totalCount` is an uncached `count(*)`. Subscriptions do not follow a project switch. Subscription resolvers are covered at service level only.
- Follow-ups: wire `Scope` + preset expansion once T-018 exposes a `core.Node` conversion.

## Review (reviewer)
