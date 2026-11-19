---
id: T-030
title: Contract - intercept queue and settings, HttpRequest.editedBy
milestone: M2
role: lead
status: changes_requested
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
- Round 2 (review fixes):
  - Settings updates release every affected item in one pass in queue order, so a mixed release stays FIFO across kinds.
  - The demo timer now models arriving traffic: a new request is held only if queuing ∧ `requests` ∧ it matches the
    mock query matcher and the selected scope preset (new `inScope`, shortcut glob matcher); otherwise it passes
    through into history. Changing the condition keeps already held items.
  - Seeded edited pairs are one exchange: the edited record copies the original, adds `X-Edited: 1` to the request
    head, keeps the response bytes, source `INTERCEPT`.
  - `InterceptItem` docs state that releases forward unchanged in FIFO order, before the new project's settings load.
  - Tests: mixed-queue FIFO release, timer query/scope filtering, pair byte fidelity. `make check` green (243 web tests).
- Follow-ups (proposed tasks): none. T-031 replaces `stubs_intercept.go`; T-100 replaces `stubs_history_edited.go`.

## Review (reviewer)
- Verdict: changes_requested
- Findings:
  - [x] [major] web/src/lib/api/mocks/intercept.ts:218 — Turning both traffic sides off in one update releases requests
    before responses. With `ii1`, `ii2`, `ii3` (response), and `ii4` (request) queued in that order, `REMOVED` arrives
    as `ii1`, `ii2`, `ii4`, `ii3`. → Drain all items in queue order when both sides are off and assert the removal order
    with a mixed queue.
  - [x] [major] web/src/lib/api/mocks/intercept.ts:184 — The demo timer checks the toggles alone. A filter for
    `never-matches.invalid` still held `shop.example.org`, and the selected scope is used only to validate the preset ID.
    → Apply the mock's history query matcher and the selected scope before holding new records. Test that an excluded
    request leaves the badge count unchanged and that changing the filter leaves previously held items in place.
  - [x] [major] web/src/lib/api/mocks/history.ts:286 — The edit links connect separately generated exchanges. In the
    tested seed, Original / Modified switches from an Automate `GET /` on `api.example.com` to a proxy `POST` on
    `cdn.example.net`. → Derive the edited fixture from one proxied exchange, change one message, and assert that the
    untouched message retains the same bytes.
  - [x] [minor] internal/api/graphql/schema/intercept.graphqls:70 — The item docs say a project switch ends the hold, but
    leave callers guessing what happens to the traffic. → State the F-INT-1 rule that the old project's queue is forwarded
    unchanged in arrival order before the new settings load.
- Verification: `make check` passed with 240 web tests when run outside the sandbox. The first run stopped at lint with
  ``context loading failed: no go files to analyze: running `go mod tidy` may solve the problem``.
  Two `make gen` runs in an isolated snapshot reproduced the committed files byte for byte. Reproduced the ordering,
  filter, and fixture issues with one local mock instance. Every changed file is in task scope.

### Round 2 (`05af50d`)

- [ ] [major] web/src/lib/api/mocks/history.ts:288 — The seeded pair shares one exchange, but original row `24` still has
  source `AUTOMATE` and links to edited row `25` with source `INTERCEPT`. F-HIST-4 excludes Automate and Replay from
  edited pairs. → Set the original fixture's source to `PROXY` and assert both sources in the existing pair test.
- [ ] [minor] web/src/lib/api/mocks/intercept.ts:200 — With queuing off, the timer can add a request marked as dropped by
  Intercept without any drop action. Starting with `count: 12`, the first timer arrival (`16`) has `isDropped: true`
  and the queue stays empty. → Clear the generated drop flag for timer arrivals before deciding whether to hold them.
- [ ] [minor] web/src/lib/api/mocks/intercept.ts:184 — The held exchange can appear in history after the time it entered
  the queue. `makeRequest` adds a row-ID offset to `Date.now()`, so with the default 500 history rows, the first held
  request's `createdAt` is more than 12 minutes ahead of `heldAt`. → Use the arrival time for intercept fixtures and
  assert that a request's `createdAt` is at or before `heldAt` for seeded and timer arrivals.
- Verification: `make check` passed, including 243 web tests. Two `make gen` runs in an isolated snapshot reproduced
  the committed files. Confirmed mixed release order, timer matching and project-switch documentation. Every changed
  file is in scope. `git diff --check main...HEAD` reported trailing whitespace on lines 514, 523 and 605 of the
  generated `web/src/lib/api/generated/schema.graphql`. No proxy, store or raw HTTP code changed.
