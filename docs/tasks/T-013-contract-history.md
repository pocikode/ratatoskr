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
- docs/03-architecture.md §6 (lead follow-up): subscriptions list should read `requestAdded(query, scopeId)`, `requestUpdated(query, scopeId)`, `requestsDeleted`.

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
  `scopeId` is accepted but ignored by the mock (scope presets have no contract yet). Mock cursors are row IDs; a cursor whose
  row was deleted restarts from the first page. Cursors are opaque, so the virtualized
  table (T-017) loads pages as it scrolls; jumping to the end uses `last`.
- Follow-ups: backend history task replaces `stubs_history.go` with `history.go`; T-017 wires `mockHistoryExchange` behind its dev flag.

## Review (reviewer)
Reviewed `main...HEAD` (df50e90) by claude-code/opus-5.5 (reviewer).
- Paths: all 15 changed files match `paths`. `make gen` → no diff. Hard rules OK: generated code not hand-edited, the `//nolint:revive` comments give a reason and the task id, mock data uses reserved hosts only, no Caido material.
- `make check`: **failed on the first run** (the Go side passed; the web side did not), then passed on the rerun:
  `FAIL src/lib/api/mocks/exchange.test.ts > streams live requestAdded events until unsubscribed — expected [ '4' ] to deeply equal [ '4', '5' ]` (exchange.test.ts:103).
- The contract matches F-HIST-1..4 (all 16 columns can be sorted, colour and note, `editedFrom`, display-only decode), arch §6 (connections plus `totalCount`, `NOT_IMPLEMENTED` stubs, Bytes/Int64 scalars) and docs/04 §7.

Findings:
- [x] [major] web/src/lib/api/mocks/exchange.test.ts:95-106 — Flaky timing test. It uses a 5 ms interval, waits a fixed 40 ms, and expects at least 2 events. When the Go tests run in parallel during `make check` the machine is busy, so only 1 event arrives and `make check` turns red (seen in this review). → Replace the fixed sleep with `await vi.waitFor(() => expect(ids.length).toBeGreaterThanOrEqual(2))` before `unsubscribe()`. Keep the 30 ms "no more events" check after it. Do not loosen the assertion.
- [x] [minor] web/src/lib/api/mocks/exchange.ts:61-62 — graphql-js `execute`/`subscribe` do not validate the document, so the comment at :3 ("errors behave as on the wire") is wrong. Probe result: `{ request(id: "1") { id hots } }` returns `{"request":{"id":"1"}}` with no error; the server would return `GRAPHQL_VALIDATION_FAILED`. A typo in an inline query (as in the tests) goes unnoticed until the backend lands. → Call `validate(schema, op.query)` first. If it returns errors, send `makeResult(op, { errors })` and skip execution.
- [x] [minor] web/src/lib/api/mocks/history.ts:368-371,100-104 — The mock does not check the `BAD_INPUT` rules the schema documents. Probe: `requests(first: 5, last: 5)` returns 5 rows; `body(offset: -3, limit: 2)` returns the last bytes (`subarray` with a negative start). The UI cannot be built against the documented errors. → Throw `GraphQLError(..., { extensions: { code: 'BAD_INPUT' } })` when `first` and `last` are both set, when either is outside 1..1000, when `offset < 0`, or when `limit` is outside 1..4194304.
- [x] [minor] web/src/lib/api/mocks/history.ts:367-371 — Cursors are list positions, so they shift when rows are inserted. With `sort: {ID, DESC}` (newest first) plus live adds, every new row moves all positions down by one. Probe: page 1 = 30,29,28 with endCursor "2". After one live add, `after: "2"` starts at row 28 again, so the same ID shows up twice. A keyed `{#each}` in T-017 would then throw `each_key_duplicate`. This also breaks the schema promise that "cursors stay stable". → Use `node.id` as the cursor and find the start with `list.findIndex((r) => r.id === after) + 1`. Same for `before`.
- [x] [minor] web/src/lib/api/mocks/history.ts:312-320 — Each `requestAdded` subscription starts its own `setInterval`. Every timer emits to every listener. With two subscriptions open (for example, a table and a status counter), twice as many rows get created and each subscriber gets events at twice the rate. → Keep one shared timer: start it when the first `live` listener is added, clear it when the last one is removed (count the live listeners).
- [x] [minor] internal/api/graphql/schema/history.graphqls:86,89,92 — AC "All fields documented" is not fully met. `source`, `port` and `method` have no description. `port` is also ambiguous: `host` comes from the Host header, but does `port` come from the Host header or from the connection target? The backend task has to guess. → Add descriptions, e.g. `"Port of the connection target (443/80 when implied)."`. Also describe `HttpRequestEdge.node`, `HttpRequestConnection.edges/pageInfo` and `Completion.kind`.
- [x] [nit] docs/03-architecture.md §6 still lists the subscriptions as `requestAdded(query)`, `requestUpdated` only. The new `requestsDeleted` and the `scopeId` args are missing. The file is outside this task's paths → note it in "Out-of-scope change needed" so the lead can update it in a follow-up.

Verdict: **changes_requested** (1 major). After the major is fixed and `make check` is green, the minors are cheap to fix in the same pass.

Response (implementer): all findings addressed. `vi.waitFor` in the live test; `validate()` before execute (new test); `BAD_INPUT`
for first+last, first/last outside 1..1000, offset < 0 and limit outside 1..4194304 (new test); cursors are row IDs (new test with
DESC + live adds); one shared live timer, ref-counted (new test with two subscribers); descriptions for `source`, `port`
(connection target, 443/80 implied), `method`, `edges`, `pageInfo`, `node`, `Completion.kind`; docs/03 note in Out-of-scope.
`make check` green twice in a row.
