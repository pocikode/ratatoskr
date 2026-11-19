---
id: T-031
title: Intercept service - queue, settings, conditions, release semantics, resolvers
milestone: M2
role: backend
status: blocked
depends_on: [T-030]
owner: ""
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
      parse error), Forward without bytes returns the held bytes; Content-Length rule table-tested (TE present, body without
      CL, CL present and wrong, no body no CL, `updateContentLength: false`).
- [ ] Replacement `raw` over 16 MiB (decoded) → `BAD_INPUT`, item stays held (F-INT-1); exactly 16 MiB is accepted.
- [ ] Any replacement `raw` for a held message over 16 MiB → `BAD_INPUT`, item stays held; forwarding it unchanged and
      dropping it work; `InterceptItem.raw` returns the whole held message.
- [ ] "Should hold" matrix table test: queuing × requests/responses × query × scope; `resp.*` clauses are false for a request.
- [ ] Turning queuing off releases every held item in FIFO order (synctest, order asserted); turning `requests` off releases
      only request items; a project-switch call releases all and loads the new project's settings.
- [ ] ctx cancelled while held (client gone) → item removed and a `REMOVED` event emitted; no goroutine leak (synctest).
- [ ] Settings: persisted per project except `queuing` (reopen → `queuing=false`, others kept); `dropStatusCode` outside
      100–599 → `BAD_INPUT`; bad `query` → `BAD_QUERY` with span; unknown `scopeId` → `NOT_FOUND`.
- [ ] Resolver tests: unknown/released item → `NOT_FOUND`; forwardAll/dropAll counts; `interceptQueueChanged` and
      `interceptSettingsChanged` deliver events; `InterceptItem.request` loads the record through `Deps.History`.
- [ ] `make check` green.

## Notes from lead
- Item ID = opaque string derived from the history request ID and kind (e.g. `req:12`); never parse it in the UI.
- Events: reuse `events.Bus[T]`; a slow subscriber must not block a hold.
- The hold call is what T-105 calls; keep it free of proxy types (bytes in, decision out) so `internal/intercept` does not
  import `internal/proxy`.

## Plan (implementer, ≤15 lines)

## Out-of-scope change needed (if any)

- Lead clarification needed in F-INT-1. The Forward contract requires `BAD_INPUT` when replacement bytes fail to
  parse. It also says an unparseable head, explicitly including conflicting duplicate `Content-Length` values,
  goes out byte-for-byte. Both `rawhttp.ReadRequest` and `rawhttp.ReadResponse` reject conflicting lengths
  during head parsing. Does the pass-through rule apply only to the Content-Length helper, with Forward
  still rejecting invalid replacements and keeping the item held?

## Handoff (implementer)
- What changed: Recorded the conflicting Forward requirements. T-030 is merged on main.
- How to verify: Compare F-INT-1 and the Forward mutation contract with `contentLength` in `internal/rawhttp/read.go`.
- Known limitations: Implementation and `make check` are pending.
- Follow-ups (proposed tasks): Resolve the Forward contract before implementing T-031.

## Review (reviewer)
- Verdict: approve | changes_requested
- Findings:
