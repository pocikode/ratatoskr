---
id: T-031
title: Intercept service - queue, settings, conditions, release semantics, resolvers
milestone: M2
role: backend
status: in_progress
depends_on: [T-030]
owner: "Codex"
branch: task/T-031-intercept-service
paths:
  - internal/intercept/**
  - internal/api/graphql/resolvers/intercept*.go
  - internal/api/graphql/resolvers/stubs_intercept.go
spec:
  - docs/01-feature-spec.md#F-INT-1 (incl. M2 contract)
  - docs/03-architecture.md §4, §7
  - docs/04-query-language.md (evaluator)
---

## Goal
`internal/intercept` holds requests and responses until the user forwards or drops them, decides what to hold from the
per-project settings, and serves the T-030 contract. Proxy integration is T-105.

## Scope
- In: `intercept.Service` with a blocking hold call per kind (returns forward-with-optional-bytes or drop, or ends when
  ctx ends), a cheap "would hold this kind now" check for the proxy's buffering decision, FIFO queue, settings in
  `intercept_settings` (key/value JSON, per project; `queuing` in memory only), HTTPQL condition via the evaluator +
  scope preset matching, Content-Length update helper, release-all on queuing off / kind off / project switch, events for
  queue and settings changes; resolvers (queries, mutations, both subscriptions); delete `stubs_intercept.go` and move the
  `InterceptService` interface into `intercept.go`.
- Out: proxy changes (T-105), app wiring (T-109), WebSocket items (T-053).

## Acceptance criteria
- [ ] Hold blocks until Forward/Drop; Forward with bytes returns them (parsed with `rawhttp`, else `BAD_INPUT` with the
      parse error), Forward without bytes returns the held bytes unchanged (`updateContentLength` ignored); Content-Length
      rule table-tested (TE present, body without CL, CL present and wrong, no body no CL, identical duplicate CL,
      `updateContentLength: false`); replacement with conflicting duplicate CL → `BAD_INPUT`, item stays held.
- [ ] Replacement `raw` over 16 MiB (decoded) → `BAD_INPUT`, item stays held (F-INT-1); exactly 16 MiB is accepted.
- [ ] Any replacement `raw` for a held message over 16 MiB → `BAD_INPUT`, item stays held; forwarding it unchanged and
      dropping it work; `InterceptItem.raw` returns the whole held message.
- [ ] "Should hold" matrix table test: queuing × requests/responses × query × scope; `resp.*` clauses are false for a request.
- [ ] Turning queuing off releases every held item in FIFO order (synctest, order asserted); turning `requests` off releases
      only request items; a project-switch call releases all and loads the new project's settings.
- [ ] Project switch: each item records the selected `*store.DB` at hold time; items of another DB are invisible to
      `interceptQueue`, Forward/Drop (`NOT_FOUND`), the all-counts and `interceptQueueChanged`. The switch releases them with
      no `REMOVED` event, then emits `interceptSettingsChanged` (queuing false). Test: hold under DB A, swap to B, the queue
      query is empty and `request` is never resolved against B.
- [ ] ctx cancelled while held (client gone) → item removed and a `REMOVED` event emitted; no goroutine leak (synctest).
- [ ] Settings: persisted per project except `queuing` (reopen → `queuing=false`, others kept); `dropStatusCode` outside
      100–599 → `BAD_INPUT`; bad `query` → `BAD_QUERY` with span; unknown `scopeId` → `NOT_FOUND`.
- [ ] Resolver tests: unknown/released item → `NOT_FOUND`; forwardAll/dropAll counts; `interceptQueueChanged` and
      `interceptSettingsChanged` deliver events; `InterceptItem.request` loads the record through `Deps.History`.
- [ ] `make check` green.

## Notes from lead
- Item ID = opaque string derived from the history request ID and kind (e.g. `req:12`); never parse it in the UI.
- Events: reuse `events.Bus[T]`; a slow subscriber must not block a hold.
- Hold precondition (F-INT-1): the caller has already waited for `Capture.Written`, so the record behind `id` is readable.
  Emit `ADDED` on enqueue without waiting further. Resolver tests use a fake `HistoryService`.
- The hold call is what T-105 calls; keep it free of proxy types (bytes in, decision out) so `internal/intercept` does not
  import `internal/proxy`.

## Plan (implementer, ≤15 lines)

- Implement project settings, HTTPQL and scope matching, and the FIFO hold/release API.
- Validate and handle replacement bytes with the existing raw HTTP parser and builder.
- Implement GraphQL queries, mutations, and subscriptions against the service.
- Cover release order, cancellation, project isolation, validation, and event delivery, then run `make check`.

## Out-of-scope change needed (if any)

- Lead decision needed for the history record on queue events during a project switch. F-INT-1 releases the old
  queue on switch, and the GraphQL contract requires the `REMOVED` item as it was when it left, with a non-null
  `request`. T-109 wires the switch through `project.Service.Subscribe`, but `internal/project/project.go:510`
  swaps the selected database and closes the old one before publishing `Selected`. `history.Service.Get`
  (`internal/history/service.go:224`) reads the selected database by numeric ID. T-100 allocates IDs per project,
  so resolving an old item through `Deps.History` can return a different project's record. Nested history fields
  such as `head` and `body` also read the selected database.
  - **Resolved (lead, 2026-11-19):** first option. Items belong to the DB selected when they were held, and only the
    selected DB's items are visible. A switch releases the old items without `REMOVED`, because clients reset on
    `projectChanged` `SELECTED`. History reads are never project-bound. See F-INT-1 and the `interceptQueueChanged`
    description.
- Should subscribers resync the queue on project switch and receive removal events only while their project is
  selected, or must old-project removal events carry project-bound history reads? The first option needs the
  lead to clarify the subscription contract. The second needs changes to the project/history lifecycle outside
  T-031's paths. Please also specify how history lookups should see pending records before T-100's first batch
  flush so `ADDED` and immediate queue reads can return the required `request`.
  - **Resolved (lead, 2026-11-19):** the proxy (T-105) waits for `Capture.Written(ctx, id)` (added to T-100) before it
    holds a message, so the record is always readable. T-031 assumes this precondition and does not depend on T-100's
    code. See F-INT-1 and docs/03 §4.

- Lead clarification needed in F-INT-1. The Forward contract requires `BAD_INPUT` when replacement bytes fail to
  parse. It also says an unparseable head, explicitly including conflicting duplicate `Content-Length` values,
  goes out byte-for-byte. Both `rawhttp.ReadRequest` and `rawhttp.ReadResponse` reject conflicting lengths
  during head parsing. Does the pass-through rule apply only to the Content-Length helper, with Forward
  still rejecting invalid replacements and keeping the item held?

- **Resolved (lead, 2026-11-19):** yes. Forward parses replacement bytes with `rawhttp` first; any parse failure,
  conflicting `Content-Length` values included, is `BAD_INPUT` and the item stays held. The pass-through clause describes
  the Content-Length helper itself (shared with Replay, which sends unparsed bytes); keep that behaviour in the helper
  (unparseable input → returned unchanged) but Intercept never calls it on such bytes. Without `raw`, the held bytes go
  out unchanged and `updateContentLength` is ignored. F-INT-1 and the `forwardInterceptItem` description updated.

## Handoff (implementer)
- What changed: Verified T-030 is merged on main and recorded the project-switch history lookup conflict.
- How to verify: Compare the non-null `InterceptItem.request` contract with project selection and history lookup at the locations above.
- Known limitations: Implementation and `make check` are pending.
- Follow-ups (proposed tasks): Clarify the switch subscription contract and pending history lookup before implementation.

## Review (reviewer)
- Verdict: approve | changes_requested
- Findings:
