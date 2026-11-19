---
id: T-031
title: Intercept service - queue, settings, conditions, release semantics, resolvers
milestone: M2
role: backend
status: in_review
depends_on: [T-030]
owner: "Codex"
branch: task/T-031-intercept-service
paths:
  - internal/intercept/**
  - internal/api/graphql/resolvers/intercept*.go
  - internal/api/graphql/resolvers/stubs_intercept.go
  - internal/replay/sender/content_length*.go
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
- [x] Hold blocks until Forward/Drop; Forward with bytes returns them (parsed with `rawhttp`, else `BAD_INPUT` with the
      parse error), Forward without bytes returns the held bytes unchanged (`updateContentLength` ignored); Content-Length
      rule table-tested (TE present, body without CL, CL present and wrong, no body no CL, identical duplicate CL,
      `updateContentLength: false`); replacement with conflicting duplicate CL → `BAD_INPUT`, item stays held.
- [x] Replacement `raw` over 16 MiB (decoded) → `BAD_INPUT`, item stays held (F-INT-1); exactly 16 MiB is accepted.
- [x] Any replacement `raw` for a held message over 16 MiB → `BAD_INPUT`, item stays held; forwarding it unchanged and
      dropping it work; `InterceptItem.raw` returns the whole held message.
- [x] "Should hold" matrix table test: queuing × requests/responses × query × scope; `resp.*` clauses are false for a request.
- [x] Turning queuing off releases every held item in FIFO order (synctest, order asserted); turning `requests` off releases
      only request items; a project-switch call releases all and loads the new project's settings.
- [x] Project switch: each item records the selected `*store.DB` at hold time; items of another DB are invisible to
      `interceptQueue`, Forward/Drop (`NOT_FOUND`), the all-counts and `interceptQueueChanged`. The switch releases them with
      no `REMOVED` event, then emits `interceptSettingsChanged` (queuing false). Test: hold under DB A, swap to B, the queue
      query is empty and `request` is never resolved against B.
- [x] ctx cancelled while held (client gone) → item removed and a `REMOVED` event emitted; no goroutine leak (synctest).
- [x] Settings: persisted per project except `queuing` (reopen → `queuing=false`, others kept); `dropStatusCode` outside
      100–599 → `BAD_INPUT`; bad `query` → `BAD_QUERY` with span; unknown `scopeId` → `NOT_FOUND`.
- [x] Resolver tests: unknown/released item → `NOT_FOUND`; forwardAll/dropAll counts; `interceptQueueChanged` and
      `interceptSettingsChanged` deliver events; `InterceptItem.request` loads the record through `Deps.History`.
- [x] Queue limit: a hold that would exceed 1 000 items or 256 MiB of held bytes returns forward-unchanged at once,
      logs at info and emits no event; held items stay. Test fails without the cap (both limits).
- [x] Response replacement is parsed with the exchange's request method; table cases: response to `HEAD` with
      `Content-Length: 1234` and no body, 204, 304 keep their `Content-Length`; a 200 still gets the rule.
- [x] A deleted active scope preset or an eval error holds nothing and logs at warn (test asserts not held).
- [x] One Content-Length helper in `internal/replay/sender` used by both Replay and Intercept; Replay tests still pass.
- [x] `make check` green.

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

- T-020 owns the load-sensitive timeout in `web/src/pages/sitemap/sitemap.svelte.test.ts:132`. The review records the
  failing test and passing reruns for the T-020 owner.

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

- **Resolved (lead, 2026-11-19, review questions):**
  - Queue limit: 1 000 items and 256 MiB of held `raw` bytes, both kinds together. Past either limit the message passes
    through unheld (info log); nothing already held is released. F-INT-1 and the `InterceptSettings` description updated.
  - Content-Length: parse a replacement response with the exchange's request method; responses to `HEAD` and status
    1xx/204/304 keep their `Content-Length`. F-INT-1 and `forwardInterceptItem` updated.
  - Deleted scope preset: holds nothing, logs at warn per failed check; same for eval errors. `scopeId` description updated.
  - Helper duplication: `internal/replay/sender/content_length*.go` added to this task's paths (T-101 is done, T-033 does
    not own it). Extract one helper there and call it from both sides.
  - Sitemap test timeout: belongs to T-020, not this task; ignore a flaky rerun here.
  - `persisted` nit: implementer's call.

## Handoff (implementer)
- Review fixes in `c2ea79b` cap the shared request/response queue at 1 000 items and 256 MiB of held raw bytes.
  Overflow forwards unchanged without a queue event. Release and client cancellation return capacity. Scope lookup and
  evaluation failures log warnings and pass through. Response edits parse with the request method and preserve
  Content-Length for HEAD, 1xx, 204, and 304. Replay and Intercept share the editor length rule.
- Verification after review fixes: `make check` (295 web tests), `make test-int`, and focused Intercept, Replay sender,
  and resolver race tests passed. Each queue regression test failed when its corresponding limit was removed via a
  temporary Go overlay. A separate proxy package run intermittently missed a capture in
  `TestUIDownloadDoesNotCaptureConnection`; the full integration suite and three focused reruns passed. A diagnostic
  25-second history run timed out in `TestSitemapCountsAndQueries`; the full integration history package took 32 seconds.
- Implemented per-project settings, HTTPQL and scope matching, FIFO release decisions, replacement validation, and queue
  and settings events. Queue IDs include a sequence so an old ID cannot address a new project's item with the same
  history ID. The resolvers load records through `Deps.History` and discard lookups that overlap a project switch.
- `make check` passed, including 295 web tests. The focused intercept and resolver tests also passed three consecutive
  Go race runs. They cover SQLite reopen, FIFO release, cancellation, edit boundaries and GraphQL operations.
- A nested history field such as `request.head` that executes during a project switch can still follow the global
  History selection after its queue item was resolved. Clients reset on `projectChanged` per the switch contract.
- T-105 calls `WouldHold`, then `HoldRequest` or `HoldResponse` with the history record's DB and a fully buffered
  `query.Message` after `Capture.Written`. T-109 initializes the service and calls `ProjectChanged` on selection.

## Review (reviewer)
- Verdict: changes_requested
- Checks (reviewer, 2026-11-19):
  - `make check` run 1: exit 2. Only failure: `web/src/pages/sitemap/sitemap.svelte.test.ts:132` "drops a stale expansion result when filters change…" → `Test timed out in 5000ms` (294 of 295 pass). The sitemap files alone pass 10/10. Run 2: exit 0 (Go all `ok`, web 295/295). The timeout is load-sensitive and not in T-031's files.
  - `make test-int`: not run. `proxy`, `store` and `rawhttp` are not changed.
  - Scope: all code changes are inside `paths`. Other changed files (`docs/01`, `docs/03`, `docs/tasks/M2/T-105`, `T-109`, `schema/intercept.graphqls`, generated `exec.go`/`models.go`/web `schema.graphql`) come from the lead's resolution commits `f68c801` and `086230b`. The generated files match the schema. Lead to confirm the schema text is theirs (rule 5).
  - AC: each checked item has a test. I checked the hold matrix, release order and cancel (synctest), project switch, settings reopen and validation, the exact and over 16 MiB limits, conflicting `Content-Length` → `BAD_INPUT`, and resolver `NOT_FOUND`.
- Findings:
  - [x] [major] `internal/intercept/service.go:245` — the held queue has no limit on count or total bytes. Each held item keeps its whole message (up to 64 MiB) until the user acts. Case: queuing and responses on, a script sends 1000 responses of 1 MB each → about 1 GB held, with no end. → Lead decides a cap (count or total bytes) and what happens past it (for example pass through and log at info, like the 64 MiB rule). Then add a test that fails if the cap is missing. Not an implementer decision.
    - Enforced both limits under the queue lock. Boundary tests cover mixed kinds, overflow without events, released capacity, and cancellation.
  - [x] [minor] `internal/intercept/replacement.go:62-78` duplicates `internal/replay/sender/content_length.go:15-38` (Transfer-Encoding check, `bodyAt` math, length compare, builder write). Both must change together (F-INT-1 and F-REP-2). → Move one helper into `sender` (or `rawhttp`) that takes `(raw, msg *rawhttp.Message, bodyAt int)`. Both call sites use it. About 12 lines saved.
    - Replay and Intercept now share `sender.UpdateParsedContentLength`. Intercept passes the head it already validated.
  - [x] [minor] `internal/intercept/replacement.go:35,46-48` — the response is parsed with method `""` (treated as GET). Case, from reading the code, not run: a 304 with `Content-Length: 1234` and no body. The body is empty, so an edit of one header with `updateContentLength` (default true) turns the length into `Content-Length: 0`. The spec text says this literally, so it is a spec question. → Lead decides whether HEAD, 1xx, 204 and 304 skip the rule. If yes, pass the request method (from `msg.Request()`) into `replacement` and add a table case.
    - Response edits use the held exchange's request method and preserve the bytes for `rawhttp.FramingNone`. Added service-level table tests.
  - [x] [minor] `internal/intercept/service.go:221-231` — `matches` swallows errors. An eval error or a failed scope read returns false, so the message goes out unheld with no log. Case: the scope preset is deleted while active → nothing is ever held, silently. → Log the error with `slog.Warn` before returning false. Lead to confirm "deleted preset holds nothing" (the query side already does this in `compile`).
    - Logged evaluation and scope lookup failures at warn before passing traffic through. Added regression tests for both cases.
  - [x] [nit] `internal/intercept/settings.go:189-191` — `persisted` exists only to skip a write on a queuing-only change. → Remove it and always call `save`. It is one transaction of five upserts. Same result, fewer lines.
    - Kept the guard per the lead’s discretion. A queuing-only change can still release the queue without needing a successful SQLite write.
  - [x] [minor] `web/src/pages/sitemap/sitemap.svelte.test.ts:132` — times out at 5 s under full-suite load (outside T-031; passed on rerun). → Report to the T-020 owner. The fix belongs there, not in a timeout bump here.
    - Recorded the test, failure, and passing rerun for T-020 above. The lead confirmed T-020 owns the investigation.
- Lean: -12 lines possible (finding 2), -2 (finding 5).
- Not checked: `make test-int` (paths not touched). Real HEAD and 304 behaviour (finding 3, reasoned from code). Load test of the queue (finding 1, reasoned from code).
- To reach approve: decide and fix finding 1. Findings 2 to 5 can be fixed here or taken as follow-ups with the lead's agreement.

### Questions for the lead

- What aggregate queue limit should Intercept enforce, and what should happen when a message would exceed it?
  F-INT-1 specifies a per-message capture cap. The review requires a separate queue limit and a regression test.
- Should Forward preserve Content-Length on responses to HEAD and on 1xx, 204, and 304 responses when
  `updateContentLength` is enabled? The current F-INT-1 rule rewrites an existing length to the body size, which
  would replace the representation length on a bodyless 304 with zero.
- When the selected scope preset is deleted, should Intercept pass traffic through with a warning, as proposed
  in review?
