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
- Known limitations:
  - Edited record's request bytes are stored before the proxy's own absolute→origin-form rewrite (so a response-only
    edit keeps identical request bytes, as F-HIST-4 requires).
  - "Dropping discards edits" is read literally: a request edited in Intercept and then dropped on the response side
    leaves no edited record, although the server got the edited request. Lead may want to clarify.
  - Pipelined bytes arriving during a hold end the disconnect watch early; a later disconnect is then noticed only when
    the response is written.
  - Rewritten responses that no longer parse are delivered as-is, not held, and the client connection is closed.
  - First `make check` run (before `bun install` in this worktree) showed one Go `FAIL` I could not reproduce in later
    full runs or in 5×/3× proxy reruns; likely the known sitemap timing flake under load (see T-031 handoff).
- Follow-ups (proposed tasks): T-109 wiring (`Deps.Interceptor = intercept.Service`, `Deps.Rewriter = matchreplace`).

## Review (reviewer)
- Verdict: approve | changes_requested
- Findings:
