---
id: T-016
title: History service and resolvers with live filtered subscriptions
milestone: M1
role: backend
status: changes_requested
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
Reviewer: claude-code/opus-5.5 · 2026-11-18 · branch `task/T-016-history-service` @ 276eced

**Checks**
- `make check`: green (go fix, vet, golangci-lint, `go test -race`, web check + unit tests). `make test-int` not run: proxy/store/rawhttp unchanged.
- Paths: all implementer changes are inside `paths`. `gqlgen.yml`, `generated/models.go` (via `make gen`) and `docs/03` §6 come from the lead commit 73a4834. OK.
- Hard rules: no hand edits to generated code, no `net/http` in the traffic path, new deps approved in docs/05 (lead confirmed), lint suppressions carry a reason and a task id. OK.
- AC: all three covered by service-level tests (`TestListCursorSurvivesConcurrentInserts`, `TestListPagesAreConsistentForEverySort`, `TestSubscribeAddedFiltersWithEvaluator`, `TestSlowSubscriberIsResyncedNotBlocking`, `TestDeleteByQueryCascades`).
- Load assumed: one pentester per instance, with 100k–1M rows (docs/00 §3). Captured bodies come from target servers, so they may be hostile.

**Findings**
- [ ] [major] internal/history/body.go:158,172-181,235 — With `decode: true`, every `body` call decodes the **whole** body into memory (up to `maxDecoded` = 256 MiB per coding layer) and then cuts out a window of at most 4 MiB. Example: a hostile server sends a ~300 KB gzip that expands to 256 MiB, or uses `Content-Encoding: gzip, gzip`. Then each viewer fetch allocates about 300–600 MB and burns CPU on the full decompression. Paging a large legitimate body in 1 MiB chunks repeats the full decode on every chunk, so the cost is O(n²). This contradicts the comment "cannot stall the server". → Stream instead of buffering. Chain the readers (`DecodedBody()` → the decoder of each coding, innermost last), then `io.CopyN(io.Discard, r, offset)` and `io.ReadAll(io.LimitReader(r, int64(limit)))`. Memory becomes O(limit), and the `maxDecoded` cap only bounds `offset`. Add a test: a body of 64 MiB of zeros gzipped, read at `offset: 0, limit: 16` and at a large offset, with correct bytes returned.
- [ ] [major] internal/app/app.go:136 + internal/history/service.go:112-115 — The Goal says "query+scope filtering", but `ServiceDeps.Scope` is never wired. In the product, every `requests(scopeId:)`, `deleteRequests(scopeId:)` and subscription with a `scopeId` returns `NOT_FOUND`. Scope presets are glob `Matcher`s (`internal/scope/match.go`), not `core.Node`s, so wiring them needs a design decision (glob → SQL, or post-filter). The handoff lists this as a follow-up, but no task tracks it (DoD: no untracked gaps). → The lead either creates a follow-up task (scope + `preset:` expansion) and links it here and in a `TODO(T-###)` at service.go:113, or narrows this task's Goal. No code change is needed from the implementer for this finding.
- [ ] [minor] internal/history/body.go:124-128 — The `stored()` fallback finds the body with `bytes.Cut(raw, "\r\n\r\n")` and does not use `body_offset`. Example: a head with LF-only line endings plus an undecodable `Content-Encoding` gives an empty body with `decode: true`, but the stored bytes with `decode: false`. → Select `raw, body_offset` at line 93 and fall back to `window(raw[bodyOffset:], …)`. Then the comment on line 125 can go.
- [ ] [minor] internal/history/body.go:174-176 — The `shortcut:` comment says br/zstd "are not in go.mod yet". They are now (lines 219-229). → Change it to "unknown coding or no decodable output: shown as stored".
- [ ] [minor] internal/history/mutate.go:151-162 — The ID-mode loop calls `rows.Close()` but never checks `rows.Err()`. If iteration fails part way, a partial ID set is deleted in a transaction that commits, and the returned count is wrong. → Add `if err := rows.Err(); err != nil { return errors.Join(…, rows.Close()) }` after the loop, like the query branch does at line 140.
- [ ] [minor] internal/history/complete.go:311 — `LIMIT MaxCompletions` runs before the prefix filter (lines 325-330). With more than 100 presets, `preset:zz` never suggests a preset that sorts after position 100. → Filter in SQL (`WHERE lower(name) LIKE lower(?) || '%' OR lower(alias) LIKE …`, with `ESCAPE` for `%`/`_`), or drop the LIMIT and rely on the cap at line 116.
- [ ] [minor] internal/history/subscribe.go:70-71 — `SubscribeDeleted` drops the error from `subscribe`. When `Bus` is nil it returns a nil channel. Then `forward` (resolvers/history.go:288, `for v := range in`) blocks forever and leaks a goroutine per subscription, because the range never sees `ctx.Done()`. → Return `(<-chan []int64, error)` like the other two subscribe methods, and update the `HistoryService` interface and the resolver.
- [ ] [minor] internal/api/graphql/resolvers/history_test.go — `HttpResponse.head`/`body` are not tested. They depend on `toHTTPRequest` setting `RequestID` (history.go:112), which is the reason for the lead's gqlgen change. If that line is lost, every response viewer gets `NOT_FOUND`, and no test fails. → Seed a response row and query `response { head body(decode: true) }`.
- [ ] [nit] internal/history/list.go:156,161,295-302 — `cmpOr` re-implements the stdlib `cmp.Or`, and the call is made twice. → Use `field := cmp.Or(a.Sort.Field, SortID)` once, and delete `cmpOr` (-8 lines).
- [ ] [nit] internal/api/graphql/resolvers/history.go:395,402-414 — `HttpResponse.Head` formats `RequestID` as a string only for `head()` to parse it back. → Make `head` take `int64` like `body`, and parse in `httpRequestResolver.Head`.
- [ ] [nit] internal/api/graphql/resolvers/history.go:250 — The name `cmpDeref` suggests `cmp`, but the function only dereferences a pointer. → Rename it to `deref`, or inline it at line 242.

**Verdict: changes_requested.** Fix the two majors (the body decode streaming, and a tracked follow-up or a narrowed Goal for scope). The minors are cheap and should go in the same pass.
