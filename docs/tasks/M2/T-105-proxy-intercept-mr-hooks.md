---
id: T-105
title: Proxy hooks - intercept hold, drop, Match & Replace rewrite, edited pairs
milestone: M2
role: backend
status: in_review
depends_on: [T-100, T-031]
owner: "claude-code (opus-5.5)"
branch: task/T-105-proxy-intercept-mr-hooks
paths:
  - internal/proxy/**
spec:
  - docs/03-architecture.md §4 (pipeline, buffering rule, hook order)
  - docs/01-feature-spec.md#F-INT-1, #F-HIST-4, #F-MR-1 (M2 contracts)
  - docs/adr/0003-own-raw-http-engine.md
---

## Goal
The proxy calls Match & Replace and Intercept at the points fixed in docs/03 §4, sends drops, and stores edited pairs,
without changing the streaming path for traffic that neither feature touches.

## Scope
- In: two interfaces in `internal/proxy`, both optional in `proxy.Deps` (nil = feature off):
  - `Interceptor` — matches the hold API of `intercept.Service` (T-031): would-hold check per kind, hold request, hold response.
  - `Rewriter` — implemented by `matchreplace` (T-035) with exactly this shape:
    ```go
    type Rewriter interface {
        // Active reports whether an enabled rule exists for source and side (decides full buffering).
        Active(source string, response bool) bool
        // Request returns the rewritten request; the same slice when nothing changed.
        Request(ctx context.Context, source string, raw []byte) ([]byte, error)
        // Response returns the rewritten response; req is the request as sent upstream.
        Response(ctx context.Context, source string, req, resp []byte) ([]byte, error)
    }
    ```
  - Full buffering (≤ capture cap) only when one of them is active for that side; over the cap → stream unchanged.
  - Before each hold (request or response), wait for `Capture.Written(ctx, id)` (T-100) so the held item's record is
    readable (F-INT-1); only when the would-hold check says the message will be held.
  - Order: M&R `proxy` → hold → after a Forward that changed bytes, M&R `intercept` → send upstream; same on the response side.
  - Drop request / response per F-INT-1 (status from the hold decision, our own short HTML body, `isDropped`).
  - Edited pair per F-HIST-4 via the T-100 capture API (source `intercept` when the user's edit changed bytes).
  - Destination always from the original connection, never from edited `Host`/target.
- Out: the M&R engine, intercept service, app wiring (T-109).

## Acceptance criteria
- [ ] Integration (testutil raw server, fake Interceptor): request held → client blocks; Forward with edits → server gets
      the edited bytes; history has the original (client bytes, server response) and the edited record (`editedFrom`,
      source `intercept`, response as delivered).
- [ ] Drop request → client gets the configured status, the server sees nothing, record `isDropped`.
- [ ] Drop response → client gets the configured status, record keeps the server response and `isDropped`.
- [ ] Response held and edited → client gets the edited response; pair stored with identical request bytes.
- [ ] Editing `Host` in a held request does not change where it is sent.
- [ ] Hold is called only after `Written` returned for the record (fake Interceptor asserts `history.Service.Get` succeeds).
- [ ] Body over the capture cap with an active Interceptor/Rewriter passes through unheld and unchanged (test with a small cap).
- [ ] Fake Rewriter adding a header → server sees it, pair stored with the original's source; `intercept` rules run only
      after a Forward that changed bytes.
- [ ] With both deps nil, existing proxy tests pass unchanged and the proxy benchmark stays within 5% of `main` (numbers in Handoff).
- [ ] `make check` and `make test-int` green.

## Notes from lead
- Plan mode required. Byte fidelity: no `net/http` on this path (ADR-0003); unchanged messages must go out as the same bytes.
- Edits to a held request may change its framing; re-read the edited bytes with `rawhttp` before writing them upstream.

## Plan (implementer, ≤15 lines)
1. `hooks.go`: `Interceptor` (WouldHold/HoldRequest/HoldResponse, intercept.Service shape) and `Rewriter` (task shape) in `Deps`.
   Extra capture calls (`Written`, `PendingDB`, `CompleteDropped`, `Add`) via optional `HoldCapture`, type-asserted from `Deps.Capture`.
2. `forward` checks per side whether a hook is active; nil deps / inactive → the existing streaming path, untouched.
3. Request side: buffer body ≤ cap (`CopyN cap+1`); over cap → stream head+prefix+rest unchanged. Pending row at once.
   M&R `proxy` → if WouldHold: `Written` → `HoldRequest(PendingDB)` → drop page | Forward (changed → M&R `intercept`).
   Changed bytes are re-read with `rawhttp`, origin-form rewritten outside tunnels, sent as bytes; dst stays the original.
4. Response side (not 101): same buffering; upstream conn returned before the hold; M&R → hold → drop | edited.
5. Hold ctx ends when the client closes (Peek watcher, idle deadline off during the hold) so the item leaves the queue.
6. Original completed as today (drop → `CompleteDropped`); edited pair via `Capture.Add` (`EditedFrom`, source
   `intercept` when a Forward changed bytes, else original's source, `ProjectDB` = owner); no pair on drop.
7. Tests: integration (real history.Capture + Service, testutil.RawServer, fake Interceptor/Rewriter) per AC;
   `BenchmarkProxyGET` run on main vs branch for the 5% budget.

## Out-of-scope change needed (if any)
- Lead: add to F-INT-1 the operator's decision (2026-11-19, T-105 review): a 1xx `dropStatusCode` is sent as that
  interim head followed by our own 502 drop page, then the connection closes. A 101 drop status will look like a
  protocol switch to an upgrading client; the lead may want to narrow the range to exclude 101.

## Handoff (implementer)
- What changed:
  - `internal/proxy/hooks.go`: `Interceptor` (shape of `intercept.Service`), `Rewriter` (task shape), `HoldCapture`
    (`Written`, `PendingDB`, `CompleteDropped`, `Add`; `*history.Capture` implements it, type-asserted from `Deps.Capture`
    so existing fakes keep working). `Deps.Interceptor` / `Deps.Rewriter`, nil = off. An Interceptor without a
    `HoldCapture` is ignored with a warn log.
  - `forward`: per side, full buffering only when M&R `proxy` is active or `WouldHold` says so; nil deps → old path.
    Over the cap (incl. chunked, detected by reading cap+1 bytes) the consumed bytes are sent first, then the rest streams
    unchanged, unheld, record truncated. Order: M&R `proxy` → `Written` → hold → (Forward with changed bytes) M&R
    `intercept`. Edited/rewritten bytes are re-read with `rawhttp` and sent as bytes; destination stays the connection's.
  - Drops: own short HTML page with the decision's status, `Connection: close`; request drop → nothing upstream,
    `CompleteDropped(nil)`; response drop → `CompleteDropped(server response)`. No edited record on any drop.
  - Edited pair via `Capture.Add` (`EditedFrom`, `ProjectDB` = `PendingDB` taken before completion, source `intercept`
    when a Forward changed bytes, else the original's). The upstream connection is returned before a response hold.
  - Client disconnect during a hold cancels the hold ctx (Peek watcher, idle deadline off during the hold) → item leaves
    the queue, record stays without a response. M&R errors are logged and leave the message unchanged.
- How to verify: `go test -race -tags integration -run 'Intercept|OverCap|Rewriter' ./internal/proxy` (8 tests, one per AC
  plus client-gone); `make check`, `make test-int` green. Mutation: removing the `Written` wait fails the fake's `Get` check.
- Benchmark (`BenchmarkProxyGET`, new, nil deps, 8 interleaved runs × 3000 ops, Ryzen 7 8845HS): main median ≈ 1.112 ms/op,
  73.8 KB, 107 allocs; branch ≈ 1.113 ms/op, 73.8 KB, 107 allocs → < 0.5 %, same allocs. Latency is dominated by the
  existing 100 µs pool liveness probe, so the bench is coarse; allocs are the sharper signal.
- Review round 1 (changes_requested → in_review): all 7 findings fixed, see Review. Mutation-checked: reverting the
  watch loop, the head-only rebuild or the HEAD close each fails its new test. `make check` green; `make test-int`
  green except the known `TestLoadNoLeakAndLatency` p99 flake under `-p=2` (5.56 ms); `./internal/proxy` integration
  passed 3/3 alone. Bench after the fixes: 1.12 ms/op, 73.8 KB, 107 allocs (unchanged).
- Known limitations:
  - The edited record of a request edit stores the bytes the origin received (origin-form target); a response-only
    edit keeps the client's request bytes in both records (F-HIST-4).
  - A client that pipelines more than the 4 KiB read buffer during a hold ends the disconnect watch; its disconnect
    is then noticed when the response is written.
  - "Dropping discards edits" is read literally: a request edited in Intercept and then dropped on the response side
    leaves no edited record, although the server got the edited request. Lead may want to clarify.
  - Rewritten responses that no longer parse are delivered as-is, not held, and the client connection is closed.
  - First `make check` run (before `bun install` in this worktree) showed one Go `FAIL` I could not reproduce in later
    full runs or in 5×/3× proxy reruns; likely the known sitemap timing flake under load (see T-031 handoff).
- Follow-ups (proposed tasks): T-109 wiring (`Deps.Interceptor = intercept.Service`, `Deps.Rewriter = matchreplace`).

## Review (reviewer)
- Verdict: changes_requested
- [x] [major] internal/proxy/hooks.go:160 — Rebuilding the absolute-form request drains the body through the parsed framing reader. If a Forward preserves a declared length shorter than the supplied body, the proxy silently discards the bytes beyond that length. Rewrite only the head and append the original byte suffix.
  - Fixed: `upstreamBytes` rebuilds only the head (parsed alone) and appends the exact bytes after it. Test `TestInterceptEditedLengthSendsBytesAsGiven`.
- [x] [major] internal/proxy/forward.go:105 — Editing a GET to HEAD means the origin can return a length header with no body. The proxy relays it to a GET client and can leave the connection open while that client waits for the advertised body. Use the sent method for origin reads and independently handle client-facing framing for method edits. Cover the streaming and hooked response paths.
  - Fixed: the origin is already read with the sent method (`exchange` uses `x.sent`); when a hook adds or removes HEAD, the response is relayed as is and the client connection closes after it (`edits.headChanged`), on the streaming, over-cap and hooked paths. Test `TestMethodEditToHeadClosesClient` (streaming + hooked).
- [x] [major] internal/proxy/hooks.go:434 — Response rules receive the pre-forwarding request bytes. On explicit proxy requests the origin sees an origin-form target and no Proxy-Connection header, but the rule can still match the absolute target or removed header. The edited history request at line 540 also records the pre-forwarding bytes after request edits, and the integration test expects them. Use the upstream byte representation for rule input and for request-edited history rows. Preserve identical stored request bytes for response-only edits.
  - Fixed: response rules get the upstream bytes (origin-form, no `Proxy-Connection`; computed from the client's request when no request hook ran). A request-edited history row stores the upstream bytes and target. Response-only edits keep the client's bytes in both rows. Tests `TestResponseRulesSeeUpstreamRequest`, `TestInterceptRequestForwardWithEdits` (updated).
- [x] [major] internal/proxy/hooks.go:185 — A buffered pipelined request makes Peek return immediately. When that client closes during the first hold, the queue entry survives until operator action and keeps its queue slot and bytes. Keep watching for disconnect while preserving pipelined input within a bound, and exercise it in the disconnect test.
  - Fixed: the watch keeps peeking `Buffered()+1` so pipelined bytes stay in the reader and a later EOF still cancels; bound = the reader buffer (4 KiB), beyond it the watch ends (`shortcut:` comment). Tests `TestInterceptClientGoneLeavesQueue/pipelined`, `TestInterceptPipelinedRequestsKeepOrder`.
- [x] [major] internal/proxy/forward.go:115 — A genuine 101 upgrade response bypasses both response hooks. With response queuing on and a matching exchange, the upgraded stream starts before the operator can review or drop the handshake. Run the response hooks on the handshake and enter the tunnel relay only when the decision still upgrades the connection.
  - Fixed: with response hooks active, a 101 goes through M&R and the hold; the tunnel starts only when the delivered response is still a parsed 101; otherwise the upstream is closed and the client gets the edited response or drop page and is closed. Test `TestInterceptUpgradeResponse` (forward / edited / drop).
- [x] [major] internal/proxy/hooks.go:267 — F-INT-1 allows a 100 drop status, which is interim. Closing after that status leaves the client without a final response even though history marks the exchange dropped. The default drop page also writes body bytes to HEAD clients. Ask the lead to resolve the allowed status range and body-less cases, and make synthetic replies respect the original client method.
  - Fixed after asking the operator (answer: send 1xx, then 502): a 1xx drop status is written as that interim head followed by our 502 page; a HEAD client gets the head without the body (`Content-Length` as for GET). Spec note requested under Out-of-scope. Test `TestInterceptDropPages`.
- [x] [minor] internal/proxy/hooks.go:91 — The early return for a declared Content-Length over the cap skips the info log required by F-INT-1. Log the bypass on both over-cap paths.
  - Fixed: both over-cap paths log at info.

Verification: `make check` and the cached `make test-int` passed. Fresh proxy and repository integration runs both missed the existing latency target once (`p99 added latency 6.301418ms (direct 745.43µs, proxy 7.046848ms), want < 5ms` and `p99 added latency 5.052387ms (direct 2.598851ms, proxy 7.651238ms), want < 5ms`). The uncached suite passed with `GOFLAGS='-count=1 -p=2' make test-int`. The load test passed twice in isolation on each branch at the existing 200-client workload. Three 1,000-operation nil-hook GET samples per branch showed no slowdown, with medians of 1.452 ms/op on `main` and 1.137 ms/op on T-105, and 107–108 allocations per operation on both. The baseline samples varied from 1.162 to 1.723 ms/op.
