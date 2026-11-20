---
id: T-105
title: Proxy hooks - intercept hold, drop, Match & Replace rewrite, edited pairs
milestone: M2
role: backend
status: blocked
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
- Lead: add to F-MR-1 the operator's decision (2026-11-19, T-105 review round 2): when the request of an exchange is over
  the capture cap, no response rule runs on that exchange (logged at info); its response may still be held.

- Lead: add to F-INT-1 (T-105 review round 7): bytes a client pipelines while its message is held are buffered up to
  the head limit (1 MiB default); beyond that the client is treated as disconnected (item leaves the queue, connection
  closes).
- Lead: add to F-MR-1/F-INT-1 the operator's decision (2026-11-20, T-105 review round 9): when an edit makes the sent
  request CONNECT and the origin answers 2xx, the client gets that head only and both connections close; the response
  is not held or rewritten.
- BLOCKED (2026-11-20, T-105 review round 10, operator chose to block): `rawhttp.ReadResponse` validates
  Transfer-Encoding/Content-Length (`frame()` in `internal/rawhttp/read.go`) before `noResponseBody`, and
  `noResponseBody` does not know CONNECT 2xx. So a CONNECT 2xx carrying e.g. `Transfer-Encoding: gzip` or a bad
  `Content-Length` returns `ErrFraming`; the proxy answers 502 and the upstream head is lost. Proposed fix (~5 lines,
  outside T-105 paths): in `frame()`, decide "no body" (1xx/204/304/HEAD **and CONNECT 2xx**) before validating the
  framing fields; keep validation for non-2xx CONNECT. Lead: authorize adding `internal/rawhttp/read.go` (+ its test) to
  T-105 `paths`, or move it to a rawhttp task. The proxy side then only needs the regression extended
  (`TestMethodEditToConnectClosesBoth` with ignored framing fields).

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
- Review round 4: 2 findings fixed (see Review). `make check` and `GOFLAGS='-count=1 -p=2' make test-int` green; nil-hook
  path untouched, benchmark not rerun.
- Review round 5: 1 finding fixed (see Review). `make check` and `GOFLAGS='-count=1 -p=2' make test-int` green.
- Review round 6: 1 finding fixed (see Review). `make check` and `GOFLAGS='-count=1 -p=2' make test-int` green.
- Review round 7: 1 finding fixed (see Review). `make check` and `GOFLAGS='-count=1 -p=2' make test-int` green;
  nil-hook path untouched, benchmark not rerun.
- Review round 8: 2 findings fixed (see Review). `make check` and `GOFLAGS='-count=1 -p=2' make test-int` green;
  nil-hook path untouched (only an extra `if` on `edits`), benchmark not rerun. Test helper `scripted.received` now
  returns connections in the order they ended.
- Review round 9: 1 finding fixed (see Review). `make check` and `GOFLAGS='-count=1 -p=2' make test-int` green;
  nil-hook path gains one method compare per exchange, benchmark not rerun.
- Review round 2: 3 findings fixed (see Review). `make check` and `GOFLAGS='-count=1 -p=2' make test-int` green.
- Known limitations:
  - A capture pause that starts between the `Paused` check and `NewPending` still buffers that one request (≤ cap);
    it is then sent unheld.
  - The edited record of a request edit stores the bytes the origin received (origin-form target); a response-only
    edit keeps the client's request bytes in both records (F-HIST-4).
  - A client that pipelines more than the head limit (1 MiB default) during a hold is treated as gone: the hold
    ends and the connection closes.
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

### Round 2

- Verdict: changes_requested
- [x] [major] internal/proxy/hooks.go:578 — An edited response with a valid head and an incomplete body can leave the client connection reusable. With automatic length updates off, shortening a held response body while preserving its declared length leaves the client waiting for bytes the proxy has already finished sending. Check the delivered message boundary before allowing reuse and close on a mismatch. Preserve the forwarded bytes.
  - Fixed: `parseResponse` also reads the body and reports `whole` (bytes end exactly at the framed end); a changed response that is not whole is sent as given and the client connection closes. Test `TestInterceptEditedResponseBadLengthClosesClient` (short / long), mutation-checked.
- [x] [major] internal/proxy/hooks.go:419 — When a streamed upload exceeds the capture cap and its response fits, both response rule paths receive the captured request prefix as if it were the whole upstream request. A rule testing the full request body can therefore produce the wrong result. Carry the truncation state into the response-rule decision and settle the bounded-input behavior with the lead. Add a mixed-size integration case. The existing over-cap test puts both bodies over the cap.
  - Fixed after asking the operator (2026-11-19: skip): when the request was cut at the capture cap (`req.Truncated()`), no response rule (`proxy` or `intercept`) runs, logged at info; the response can still be held. Spec note requested under Out-of-scope. Test `TestOverCapRequestSkipsResponseRules` (request over, response fits), mutation-checked.
- [x] [minor] internal/proxy/hooks.go:507 — Response Intercept rebuilds and caches a copy of the upstream request even when Match & Replace is disabled. At the existing 200-client test workload, 8 MiB uploads with small held responses would produce roughly 1.6 GiB of extra cached request copies. Build those bytes only for an active response rule and reuse the captured request bytes for the unedited hold view.
  - Fixed: `rewrite` takes the request as a func, called only after `Active` says a rule runs; the hold view uses `req.Raw()`, or `x.upRaw` already built by a request edit.

Verification: `make check`, `GOFLAGS='-count=1 -p=2' make test-int`, and `git diff --check main...HEAD` passed. This round used the existing 200-client load test and did not repeat the nil-hook benchmark comparison recorded above.

### Round 3

- Verdict: changes_requested
- [x] [major] internal/proxy/hooks.go:608 — Forwarding a response edited to a standalone 103 can leave a keep-alive client waiting for a final response that will never arrive. Treat a standalone informational response as incompatible with connection reuse, preserve the edited bytes, and add an integration case.
  - Fixed: a delivered response whose status is below 200 (and not an entered upgrade) closes the client after its bytes, sent as given. Case `103` in `TestInterceptEditedResponseBadLengthClosesClient`, mutation-checked (times out without the fix).
- [x] [minor] internal/proxy/hooks.go:540 — Response holds copy the captured request again even though the pending history entry already has the same bytes. At 200 simultaneous holds after 8 MiB uploads, this adds about 1.6 GiB of allocations that the response queue's byte limit does not account for. Reuse the pending entry's request bytes when the request was unedited.
  - Fixed: an unedited request's hold view aliases `entry.Req.Raw` (the pending record's copy) instead of `req.Raw()`.

Verification: `go test -count=1 -race -tags integration -run 'Intercept|OverCap|Rewriter|ResponseRules|MethodEdit' ./internal/proxy` passed. The full repository checks and benchmark were not rerun.

Round 3 fixes verified with `make check` and `GOFLAGS='-count=1 -p=2' make test-int` (green); benchmark not rerun (nil-hook path untouched).

### Round 4

- Verdict: changes_requested
- [x] [major] internal/proxy/hooks.go:427 — The response parser uppercases the method, while the client connection reuse guard compares it case-sensitively. If a rule turns GET into lowercase head and the origin answers with HEAD framing, the guard can leave the GET client waiting for a body. Use the same method normalization in both checks and cover the lowercase edit.
  - Fixed: `isHead` (case-insensitive, as `rawhttp` frames responses) in `headChanged` and the drop page. `TestMethodEditToHeadClosesClient` now runs `HEAD` and `head` × streaming/hooked, mutation-checked.
- [x] [major] internal/proxy/forward.go:115 — An over-cap upload disables response rules inside `respondHooked`, after the proxy has already chosen to buffer. When no response can be held, an active response rule still delays a slow download until the entire body or the capture cap arrives. Apply the truncation check before choosing to buffer and cover early delivery with a gated upstream response.
  - Fixed: `hookResponse(req.Truncated())` buffers for a rule only when the request was not cut (the skip is logged there); a hold still buffers. Test `TestOverCapRequestStreamsResponse` (gated upstream, client must see head + prefix first), mutation-checked.

Verification: the focused proxy integration suite passed under the race detector and `git diff --check main...HEAD` passed. The full repository checks and benchmark were not rerun in this review.

### Round 5

- Verdict: changes_requested
- [x] [major] internal/proxy/forward.go:115 — When history capture is paused, `NewPending` returns zero. With response queuing on and no active response rule, the proxy still buffers the response even though the hold path requires a nonzero history ID. A slow download can wait for upstream completion or the capture cap before the client gets its response head. Require a nonzero pending ID for Intercept-driven response buffering and add a gated streaming test with paused capture.
  - Fixed: `hookResponse(id, …)` buffers for a hold only when `id != 0`. Test `TestPausedCaptureStreamsResponse`
    (gated upstream shared with `TestOverCapRequestStreamsResponse`), mutation-checked.

Verification: `make check`, `GOFLAGS='-count=1 -p=2' make test-int` (including the 200-client proxy load test), and `git diff --check main...HEAD` passed. The nil-hook benchmark comparison was not rerun.

### Round 6

- Verdict: changes_requested
- [x] [major] internal/proxy/hooks.go:62 — With capture already paused and request queuing enabled, the proxy buffers the upload before calling `NewPending`. That call returns zero, so the request cannot be held. With no active request rewrite, a slow upload delays the start of upstream forwarding until the body completes or crosses the cap, contrary to the streaming rule in docs/03 §4. Check capture availability before buffering for a request hold, retain the zero-ID check for changes during capture, and add a gated upload test that requires the origin to see the head and first body bytes before the client sends the rest.
  - Fixed: `HoldCapture` gains `Paused()` (`*history.Capture` has it); `hookRequest` buffers for a hold only when capture
    is not paused; the `id != 0` check stays for a pause that starts later. Test `TestPausedCaptureStreamsRequest`
    (origin must see head + "ab" before the client sends "cd"), mutation-checked.

The round 5 fix gates response buffering on the pending record ID and covers early delivery with paused capture.

Verification: `make check`, `GOFLAGS='-count=1 -p=2' make test-int`, and `git diff --check main...HEAD` passed. This round reused the recorded nil-hook benchmark comparison.

### Round 7

- Verdict: changes_requested
- [x] [major] internal/proxy/hooks.go:230 — The disconnect watcher stops when the pipelined input fills the reader buffer. If the client then disconnects, its held request stays in the queue until operator action, contrary to F-INT-1. Keep disconnect handling effective when pipelined input fills the buffer while preserving bounded memory and request ordering. Extend the disconnect regression test to cover a full reader buffer.
  - Fixed: once the client reader is full, the watcher keeps reading into an ordered side buffer bounded by the head
    limit (1 MiB default); after the hold those bytes are put back in front of the reader (`session.src`). Read error
    → hold ends; more than the bound pipelined during a hold → treated as gone (lead note under Out-of-scope).
    Tests: `TestInterceptClientGoneLeavesQueue/full_buffer`, `TestInterceptPipelinedRequestsKeepOrder` (12 KiB request
    between two small ones); both mutation-checked.

The round 6 fix checks whether capture is paused before buffering an upload for Intercept. The gated upload test verifies that the origin receives the first body bytes before the client sends the rest.

Verification: `make check`, `make test-int`, and `git diff --check main...HEAD` passed. This round reused the recorded nil-hook benchmark comparison.

### Round 8

- Verdict: changes_requested
- [x] [major] internal/proxy/hooks.go:527 — The edited request is reparsed only through its head. If its body ends outside the declared boundary, the proxy can still take a shared upstream connection and return it to the pool at hooks.go:668. A later exchange can inherit an upstream connection whose request and response boundaries no longer agree. The existing byte-fidelity test forces `Connection: close` and misses this case. Check the full edited request boundary and send mismatched bytes exactly once on a fresh connection that is closed after the exchange. Add a regression that verifies byte fidelity and connection isolation without relying on a close header.
  - Fixed: a changed request is re-read whole (`wholeRequest`); bytes that end off their framing set `edits.split`:
    `roundTrip` takes `pool.Fresh` (never the pool, no retry) and `release` closes that upstream after the exchange;
    the client connection stays usable. Test `TestInterceptSplitRequestGetsOwnConnection` (warm pooled upstream,
    overlong body, keep-alive, then a later request), mutation-checked for both the fresh dial and the close.
- [x] [minor] internal/proxy/hooks.go:254 — The documented hold limit counts all pipelined bytes, but the check runs only after the first 4 KiB has filled the reader and another read has completed. With a small configured head limit, a client can exceed it while the watcher is still peeking. Count buffered and newly read bytes against the same budget and cover the boundary with a small configured limit.
  - Fixed: the watcher checks `Buffered()` before each peek and `Buffered()+len(over)` after each read against the
    head limit. Test `TestInterceptPipelinedOverHeadLimitEndsHold` (512-byte limit, ~1 KiB pipelined), mutation-checked.

The round 7 changes keep watching for disconnects past a full reader and replay buffered input in order. `make check`, `make test-int`, and `git diff --check main...HEAD` passed. This review reused the recorded nil-hook benchmark comparison. Lead escalation under docs/07 §7 is due because the review has exceeded three rounds.

### Round 9

- Verdict: changes_requested
- [x] [major] internal/proxy/forward.go:209 — An edit can change the upstream method to CONNECT after client routing has finished. A successful upstream CONNECT changes the protocol at the end of the response head, but the raw HTTP response reader leaves that transition to the caller. The forwarding path can wait for a response body on an idle tunnel or treat the tunnel as reusable HTTP. Handle successful upstream CONNECT responses as a separate connection state and keep those connections out of the HTTP pool. Ask the lead to confirm the client-facing behavior when the original request was an ordinary HTTP request.
  - Fixed after asking the operator (2026-11-20: head only, then close): when the sent request is CONNECT (any case)
    and the origin answers 2xx, the client gets the response head only, the upstream and the client connection close,
    no response hook or hold runs; both records are stored with that head. Spec note requested under Out-of-scope.
    Test `TestMethodEditToConnectClosesBoth` (streaming / hooked), mutation-checked (both time out without the fix).

The round 8 changes isolate edited requests with incomplete or surplus body bytes from the shared upstream pool on the ordinary HTTP path. The hold watcher now counts the reader's buffered bytes toward the configured limit.

`make check`, `make test-int`, and `git diff --check main...HEAD` passed. This review reused the nil-hook benchmark comparison in Handoff. The CONNECT finding is based on code inspection.

Escalated to the lead in this review because the task has exceeded the three-round limit in docs/07 §7. The lead should resolve the client-facing CONNECT behavior before implementation resumes.

### Round 10

- Verdict: changes_requested
- [ ] [major] internal/proxy/forward.go:222 — The new head-only branch handles parsed CONNECT successes, but the response reader validates framing fields before that branch can see the status. A successful CONNECT with unsupported transfer coding or invalid length metadata becomes a synthetic 502. The upstream response head is lost from history. → Apply the CONNECT 2xx boundary before validating body framing, preserve the raw head, and extend the regression to cover ignored framing fields. Keep ordinary framing validation for unsuccessful CONNECT responses. [RFC 9112 §6.3](https://www.rfc-editor.org/rfc/rfc9112.html#section-6.3)

The round 9 regression passes for a CONNECT success without framing fields in both streaming and hooked configurations. The remaining parser issue was identified by inspection. Lead escalation remains open under docs/07 §7. A shared fix in `internal/rawhttp` requires lead authorization because it falls outside T-105's source paths.

`make check`, `GOFLAGS='-count=1 -p=2' make test-int`, and `git diff --check main...HEAD` passed. The nil-hook benchmark comparison in Handoff was reused.
  - Blocked: needs a `internal/rawhttp` change (outside paths); question for the lead under Out-of-scope.
