---
id: T-011
title: Proxy core - listener multiplexing, CONNECT, MITM, forwarding, capture
milestone: M1
role: backend
status: in_review
depends_on: [T-005, T-008, T-010]
owner: "claude (sonnet 5.5)"
branch: task/T-011-proxy-core
paths:
  - internal/proxy/**
  - internal/netx/**
  - internal/history/capture*.go
  - internal/events/**
  - internal/testutil/**
  - internal/app/**
spec:
  - docs/01-feature-spec.md#F-NET-1, #F-NET-2, #F-NET-3
  - docs/03-architecture.md §4, §7
---

## Goal
The :8080 listener routes proxy traffic vs UI traffic, MITMs TLS using `internal/ca`, forwards through `rawhttp` with a keep-alive pool,
and captures every request/response into the project DB (batched) emitting `RequestAdded/RequestUpdated` events.

## Notes from lead
- (2026-11-18, T-006) `app.New` is now `app.New(ctx, opts)`. It opens instance.db plus `project.Service` (`ctx` is wrapped in `context.WithoutCancel`). Rebase on T-006 and keep that signature.
- T-006 also edits `internal/app/app.go` (project service wiring). Start from a `main` that includes T-006, or rebase onto it before review.
- (2026-11-18, T-008) `internal/app/app.go` already opens `ca.Open(...)` and mounts `/ca.crt` + `/ca.der` on the UI listener: rebase, do not re-add it. Call `ca.Service.TLSConfig()` for MITM. F-NET-2's `http://ratatoskr.localhost/ca.crt` arrives through the proxy listener, so answer it there too (and let `ratatoskr.localhost` pass the loopback Host check).

## Acceptance criteria
- [x] `curl -x` HTTP and HTTPS (with `--cacert`) through the proxy return identical bodies to direct requests (integration test against testutil servers).
- [x] Malformed requests from the golden corpus are forwarded byte-exact to a raw TCP test server.
- [x] 200 concurrent clients × 50 requests: no goroutine leak (goroutineleak profile/test), p99 added latency < 5 ms locally.
- [x] Upstream errors produce a 502 page from Ratatoskr and a stored record with error text.
- [x] Capture can be paused when no project is open (requests still forwarded).

## Plan (implementer, ≤15 lines)
1. `internal/events`: generic `Bus[T]`, per-subscriber buffer 1024, non-blocking publish, slow subscribers get a `Stale` marker.
2. `internal/netx`: dialer with timeouts + keep-alive `Pool` keyed host:port:tls:sni (idle timers, per-key cap, no goroutines).
3. `internal/history/capture.go`: batched writer (50 ms / 200 rows) into project.db requests/responses/*_headers; paused when no project DB; emits Added/Updated on a Bus. Upstream errors are stored as the synthetic 502 response (no schema change).
4. `internal/proxy`: accept loop; per-conn goroutine tied to ctx. First request decides: CONNECT (TLS sniffed -> MITM via ca.TLSConfig with CONNECT-host SNI fallback, else plain), absolute-form -> forward, origin-form with listener/`ratatoskr.localhost` Host -> UI (conn replayed into an `http.Server` via a chan listener; absolute-form `ratatoskr.localhost` GET/HEAD rewritten to origin-form).
5. Forward only through rawhttp + netx; absolute-form target rewritten to origin-form with `Builder.SetTarget`, everything else byte-exact; 1xx relayed, 101 tunnelled raw; stale pooled conn retried once for bodyless requests; errors -> 502 page + record.
6. `internal/testutil`: raw TCP server, HTTP/TLS upstreams. Tests: unit, byte-exact corpus, capture, integration (`-tags integration`): 200x50 leak + latency, HTTP/HTTPS parity.
7. `internal/app`: serve the listener through `proxy.Server` (UI handler as fallback). Multi `--listen` / `--ui-listen` stay gated (follow-up).

## Out-of-scope change needed (if any)

## Handoff (implementer)
**Changed:** new `internal/events` (Bus[T]), `internal/netx` (dial + keep-alive pool), `internal/history/capture.go` (batched writer 50 ms/200 rows, paused when no project DB), `internal/proxy` (listener mux, CONNECT/MITM, forward, 100-continue, 101 tunnel, 502 page), `internal/testutil` (RawServer), `internal/app` (Serve now runs proxy + capture; `app.Events()`).
**Verify:** `make check`; `go test -race -tags integration ./internal/proxy` (200x50 clients, goroutine count back to baseline, p99 added < 5 ms).
**Decisions / limitations:**
- Upstream TLS is not verified (`InsecureSkipVerify`, justified nolint): a pentest proxy must reach broken targets. A Settings toggle is a follow-up.
- No pending row: an exchange is inserted once complete, so `Added` and `Updated` are published together. The pending state arrives with intercept (T-0xx).
- Upstream errors have no schema column: the synthetic 502 page is stored as the response (error text is in its body).
- `http://ratatoskr.localhost/...` through the proxy supports GET/HEAD only (request is replayed into the UI server).
- Absolute-form targets are rewritten to origin-form for the origin server; inside a CONNECT tunnel bytes are forwarded untouched.
- Tests use Go's HTTP client as the "curl" stand-in; the golden-corpus test sends every forwardable wire request in `rawhttp/testdata/requests.txtar` through a CONNECT tunnel to a raw TCP server.
- Multi `--listen` and `--ui-listen` stay gated with ErrNotImplemented (follow-up; needs Settings -> Network).
- The integration test admits connects 32 at a time: a 200-connection burst overflows macOS's listen backlog before the proxy sees it.
- WebSocket frames are not recorded (relayed raw after 101); matchreplace/intercept hooks not wired yet.
- `make check` needed `bun install` in the worktree (no change to web files).

## Review (reviewer)
