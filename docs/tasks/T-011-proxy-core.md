---
id: T-011
title: Proxy core - listener multiplexing, CONNECT, MITM, forwarding, capture
milestone: M1
role: backend
status: changes_requested
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
**Reviewer:** claude (opus 5.5) · 2026-11-18 · **Verdict: changes_requested**

Scope: all 15 changed files are inside `paths`. `make check` green (exit 0). `make test-int` green.
AC 1-5 are each covered by a test (`TestHTTPAndHTTPSMatchDirect`, `TestCorpusByteExact`, `TestLoadNoLeakAndLatency`,
`TestUpstreamErrorIs502AndRecorded`, `TestCapturePausedWithoutProject`). Hard rules: no net/http on the proxy path
(UI handoff only), nolint is justified with the task id, no generated or schema files touched.

- [x] [major] internal/proxy/server.go:391-401 — After a proxied `GET http://ratatoskr.localhost/...`, the whole
  client connection is handed to the UI `http.Server`. The client (curl with several URLs, a browser after downloading
  the CA) reuses that keep-alive proxy connection, so every later request on it goes to the UI handler. Those requests
  are never forwarded and never recorded. I confirmed this with a temporary test (now deleted): `GET http://ratatoskr.localhost/ca.crt`
  and then `GET http://127.0.0.1:<upstream>/x` on one `http.Transport` returned `"ui:/x"` with 0 recorded entries.
  → Make the replayed request the last one on that connection: add `.SetHeader("Connection", "close")` to the builder
  at :398 so `http.Server` closes after it answers. Add a regression test with exactly these two requests.
- [x] [major] internal/history/capture.go:212-250 — Request rows are inserted in one transaction (`InsertRequests`) and
  their headers and responses in a second one (`db.Write`). If the second transaction fails (busy DB, the 5 s flush
  deadline at shutdown, a project closed between the two), the request rows stay in the DB with no headers and no
  response. No event is published for them, and `dropped` counts them as lost even though they were stored. History then
  shows requests that stay unanswered forever. → Write the requests, headers and responses in one `db.Write`
  transaction. Either run the `INSERT INTO requests` inside the existing closure (capture.go already writes raw SQL for
  responses), or ask the lead under "Out-of-scope change needed" for a `store` helper that takes a `*sql.Tx`. Add a
  test where the response insert fails and check that no request row is left.
- [x] [major] internal/proxy/server.go:360-372 — An origin-form request whose Host is neither the listener nor
  `ratatoskr.localhost` is forwarded to that Host. That is invisible proxying, which F-NET-6 defines as an opt-in
  listener option. F-NET-1 only defines the absolute-form/CONNECT → proxy and the listener/ratatoskr.localhost Host → UI
  cases. This is product behaviour the spec does not define (hard rule 2). → Until F-NET-6 lands, answer these
  requests with `421 Misdirected Request` (or `400`) using `writeSimple`. Or set `blocked` and ask the lead what the
  default should be. Update `TestUIRouting` to match.
- [x] [minor] internal/proxy/server.go:378-386 — `isUIHost` compares port strings, but the forwarder parses them with
  `Atoi`. Absolute target `http://127.0.0.1:<listen-port>/` with `Host: 127.0.0.1:0<listen-port>` makes the proxy forward
  to itself, and that hop sees the same Host and forwards again. The loop stops only at `maxConns` (10 000 conns and
  goroutines). Separately, any Host whose port equals the listener port (e.g. `target.test:8080`) is sent to the UI.
  → Use `parseAuthority` in `isUIHost` and compare numeric ports, and match the host against loopback/listener
  addresses only, not every host with that port (F-NET-1: "Host = listener address").
- [x] [minor] internal/proxy/forward.go:89 — The stale-connection retry repeats any request without a body, including
  `POST`/`DELETE`. If the upstream ran the request and then dropped the connection before answering, the request
  runs twice. → Retry only idempotent methods (`GET HEAD OPTIONS TRACE PUT DELETE` is the RFC 9110 set; drop
  PUT/DELETE if you want to be strict), or only when no response byte was read.
- [x] [minor] internal/proxy/forward.go:118 and server.go:297 — The read deadline is removed once the head is read.
  An upstream that sends its head and then stops sending the body, or a client that stops sending its request body,
  blocks the session goroutine until `Serve` ends. Nothing notices that the client already gave up. → While the body
  is copied, set a per-read idle deadline (for example a small wrapper on `up` that sets `SetReadDeadline(now+idleTimeout)`
  on each Read), or reuse `idleTimeout`.
- [x] [nit] internal/app/app.go:201 — The pool is never closed, so idle upstream connections survive `Serve`. This only
  matters if `Serve` is called again in the same process (tests). → `defer pool.Close()` after `srv.Serve` returns.
- [x] [nit] internal/proxy/server.go:406 — `strings.NewReader(string(head))` copies the head. → Use `bytes.NewReader(head)`.
- [x] [nit] Handoff "No pending row": docs/03 §4 defines `history.NewPending` plus `requestAdded(pending)` before
  forwarding. Deferring this is reasonable, but get the lead to acknowledge it and add a `TODO(T-0xx)` or follow-up
  task id so it is tracked.

**Implementer reply (2026-11-18):** all findings addressed.
- UI download: replayed request carries `Connection: close`; test `TestUIDownloadDoesNotCaptureConnection`.
- Capture: requests, headers, responses in one `db.Write`; test `TestCaptureFailedResponseLeavesNoRequest`.
- Foreign origin-form Host -> `421` until F-NET-6; test `TestOriginFormForeignHostIsMisdirected`.
- `isUIHost` uses `parseAuthority`, numeric port, and only localhost/IP literals on the listener port.
- Retry only for GET/HEAD/OPTIONS/TRACE. Idle read deadline (`netx.IdleConn`) refreshed per read on both client and upstream sides; disabled for 101 tunnels and UI handoff.
- `pool.Close()` after Serve; `bytes.NewReader`.
- Pending row: not built; tracked as follow-up for the intercept task (lead to ack).

### Re-review (round 2)
**Reviewer:** claude (opus 5.5) · 2026-11-18 · **Verdict: changes_requested**

Scope: all changed files still inside `paths`. `make check` green (exit 0). `make test-int` green (exit 0).
All nine round-1 findings are fixed and the new tests cover them (`TestUIDownloadDoesNotCaptureConnection`,
`TestCaptureFailedResponseLeavesNoRequest`, `TestOriginFormForeignHostIsMisdirected`). One older bug remains, and the
stricter retry rule from round 1 now hits it more often.

- [ ] [major] internal/netx/pool.go:94-109 + internal/proxy/forward.go:99 — `take` hands out an idle connection without
  checking that the server is still there. Many servers close idle keep-alive connections long before the pool's 90 s
  (Node and Apache: 5 s). A request with a body, or any non-safe method, sent on such a connection gets EOF and is
  answered with a 502. It is not retried, and that part is correct. I confirmed this with a temporary test (now
  deleted): upstream `IdleTimeout = 100ms`, `POST /login` through the proxy, wait 300 ms, the same POST again →
  `200`, then `502`. Login forms and API calls after a short pause will fail every time.
  → In `take`, check each popped connection before returning it: `c.SetReadDeadline(time.Now())`, `_, err := c.BR.Peek(1)`,
  then `c.SetReadDeadline(time.Time{})`. A timeout error means the connection is alive, so return it. EOF, any other
  error, or bytes that arrive unasked mean it is dead, so close it and try the next one. A read timeout is not sticky on
  `tls.Conn`. Add a pool or proxy test with the setup above (short upstream `IdleTimeout`, two POSTs) that expects 200 twice.
- [ ] [minor] internal/proxy/forward.go:59, server.go:321/424, forward.go:38 — Client writes have no deadline.
  If a client stops reading in the middle of a large response (a suspended process, a laptop going to sleep without
  sending FIN), `resp.WriteTo(c.conn)` blocks once the TCP buffers are full. The session goroutine and the upstream
  connection then stay held until `Serve` ends. Round 1 fixed the read side only.
  → Add the same idea for writes: a small `Write` wrapper on the client conn that calls `SetWriteDeadline(now+idleTimeout)`.
  You can also put it in `netx.IdleConn` next to `Read`. Turn it off for 101 tunnels, as reads already are.
- [ ] [minor] internal/proxy/server.go:448-470 — The TLS handshake used to have one 10 s deadline for the whole
  handshake. Now the 10 s limit applies to each read, and the handshake writes have no deadline. A client that sends
  its ClientHello one byte every 9 s keeps a goroutine for more than an hour. → Before `HandshakeContext`, set
  `tc.SetDeadline(time.Now().Add(handshakeTimeout))` and set `c.idle.Idle = 0`. After the handshake, clear the deadline
  and set `c.idle.Idle = idleTimeout`.
- [ ] [minor] internal/proxy/proxy_test.go — Two round-1 fixes still have no test that would fail if they broke:
  (a) the self-forward loop, `GET http://127.0.0.1:<port>/` with `Host: 127.0.0.1:0<port>`, which must reach the UI
  and must not be forwarded; (b) the stall deadline, where an upstream sends its head and then nothing. For (b), make
  `idleTimeout` a `Deps` field (zero → 2 min), as `HeaderTimeout` already is, so a test can set 100 ms.
- [ ] [nit] internal/history/capture.go:254-266 — `insertRequest` copies the column list of `store.insertRequestSQL`
  (internal/store/requests.go:27). The two copies must change together when the schema changes. → Ask the lead for an
  exported `store.InsertRequestTx(ctx, tx, Request)` (out of scope here) and leave a `TODO(T-011-followup)` until then.
  Also, capture.go:212 says "Write may retry the closure", but `store.runTx` never retries. → Remove the comment or
  make it accurate.
- [ ] [nit] Handoff "Pending row" — It still says "lead to ack", and there is no task id or `TODO(T-###)` for it. The
  Definition of Done requires untracked TODOs to be tracked. → Get the follow-up task id from the lead and write it in
  the Handoff.
