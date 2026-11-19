---
id: T-101
title: Replay sender - raw send over a fresh connection, Content-Length rule, timeout, cancel, redirects
milestone: M2
role: backend
status: in_review
depends_on: []
owner: codex
branch: task/T-101-replay-sender
paths:
  - internal/replay/sender/**
  - internal/testutil/tls*.go     # new files only; T-100/T-105 may touch other testutil files in parallel
spec:
  - docs/01-feature-spec.md#F-REP-2 (incl. M2 contract: steps, fresh connection, redirects, AC)
  - docs/01-feature-spec.md#F-INT-1 (Content-Length rule)
  - docs/adr/0003-own-raw-http-engine.md
---

## Goal
A pure network package that sends exact request bytes and reads the response with `rawhttp`, used by Replay now and by
Automate/workflows later.

## Scope
- In: `sender.Send(ctx, Request{Raw, Host, Port, TLS, SNI, Options}) (Result, error)` (names are a suggestion):
  fresh dial (`net.Dialer`, no pool), TLS with `InsecureSkipVerify`, SNI rule, ALPN `http/1.1`; timeout from dial to last
  response byte; ctx cancel closes the connection; response via `rawhttp.ReadResponse` up to the capture cap (64 MiB,
  then truncated); `UpdateContentLength(raw) []byte` exported (same rule as F-INT-1, also used by Intercept/M&R if they
  want it); redirect following that returns every hop (request bytes, connection, response) so the caller can store them.
  Errors classified: dial, TLS, timeout, cancelled, protocol.
- In: new `internal/testutil/tls*.go` if needed (TLS raw server reporting SNI/ALPN); redirect fixtures stay in the package tests.
- Out: interpolation, M&R, storage (T-106); HTTP/2 (T-081); pipelining (T-081/T-082).

## Acceptance criteria
- [x] F-REP-2 AC: `GET / HTTP/1.1` with a wrong `Content-Length` and update off → the test server receives exactly those bytes.
- [x] Content-Length table test: `Transfer-Encoding` present → untouched; body without CL → added; wrong CL → fixed; no
      body and no CL → untouched; duplicate CL headers → one, first position kept.
- [x] TLS test server sees the SNI override (and the host when none), ALPN `http/1.1`; self-signed cert accepted.
- [x] Timeout covers a server that stalls mid-body (error kind timeout); cancel closes the connection, returns kind
      cancelled, leaves no goroutine (`testing/synctest` or a leak check).
- [x] Response over the cap → result truncated, connection closed.
- [x] Redirects: off by default; on → 10-hop limit (11th returns the 3xx as final), 303 after POST → GET without body and
      without `Content-Length`/`Content-Type`/`Transfer-Encoding`, 307 keeps method and body, relative `Location`
      resolved, cross-host hop changes host/port/TLS/SNI and `Host`; every hop returned.
- [x] No `net/http` import outside tests (ADR-0003); `make check` and `make test-int` green.

## Notes from lead
- Plan mode recommended (`rawhttp`-adjacent). Reuse `rawhttp.Builder` for header edits instead of hand-splicing bytes.
- Runs fully in parallel with T-033 (no shared paths).
- 2026-11-19 unblock: an implementer set this to `blocked` because the sender sends malformed framing verbatim.
  That is intended behaviour, not a spec gap: `docs/00-overview.md §2` goal 2 (clarified today), ADR-0003, F-REP-2 AC.
  Sends are always started by the operator, to the host/port they entered. The package adds no target discovery, payload
  generation or automatic retries. Request validation or "fixing" before send is **rejected** because it breaks the
  F-REP-2 AC. The only rewrite is the opt-in Content-Length update (F-INT-1). Acceptance criteria are unchanged. Tests
  run only against local `testutil` servers on 127.0.0.1 (AGENTS.md rule 7: no real targets).
- 2026-11-19 second unblock: re-blocked with no new question. The decision above stands and is final for this task:
  the sender writes the bytes it is given, unvalidated, like any HTTP repeater. "Validated framing" is not an accepted
  contract and is not a follow-up. An implementer who will not build that should leave the task unclaimed rather than block it.
  Clarified in F-INT-1: identical duplicate CLs merge, conflicting CLs or unparseable heads are left untouched, so the
  existing `UpdateContentLength` behaviour is correct; add a test for the conflicting case. Remaining work: the sender
  itself (dial, TLS/SNI/ALPN, timeout, cancel, cap, redirects, error kinds) and its tests.
- 2026-11-19 third note: the Codex session declined the sender and left the task unclaimed, as asked above. Accepted, no
  spec change. Whichever backend agent claims T-101 next keeps `UpdateContentLength` (025912c, 3b699b5, 6191f59) and
  builds the sender on top of it. Reviewer minor (docs on this branch): the lead accepts `c4cc6d3`/`948432b`, and they
  ride with this branch.

## Plan (implementer, ≤15 lines)
- Keep `UpdateContentLength` (caller applies it before `Send`; `Send` writes `Request.Raw` unchanged).
- `Send(ctx, Request{Target{Host,Port,TLS,SNI}, Raw, Options{Timeout, FollowRedirects}}) (Result{Hops}, error)`.
- Dial: fresh `net.Dialer` per exchange, closed after it. TLS: `tls.Client`, `InsecureSkipVerify`,
  `ServerName = cmp.Or(SNI, Host)` (crypto/tls drops IP literals from SNI), `NextProtos: ["http/1.1"]`.
- Timeout: `context.WithTimeout` around the whole send (dial to last byte); `context.AfterFunc(ctx, conn.Close)`
  unblocks any read/write on cancel or timeout. No goroutine outlives `Send`.
- Read: `rawhttp.ReadResponse` with `DefaultLimits`, body drained through `io.LimitReader(cap+1)`; over the cap the
  response is `Truncated()` and the connection is closed by the deferred Close.
- Redirects (opt-in): 301/302/303/307/308 + Location, resolved with `net/url` against the hop's URL; `Builder` sets
  target, `Host` (cross-host only), method/body/framing rules; at most 10 followed (11 hops). Every hop returned,
  failed hop included with nil Response.
- Errors: `*Error{Kind, Err}`; ctx deadline → timeout, ctx cancel → cancelled, else stage kind dial/tls/protocol.
- Tests: verbatim send (wrong CL), TLS SNI/ALPN via new `testutil.NewTLSRawServer`, error kinds table,
  synctest + `net.Pipe` for timeout/cancel (bubble exit = leak check), 64 MiB cap, redirect table.

## Out-of-scope change needed (if any)

## Handoff (implementer)
- What changed (claude-backend, 2026-11-19):
  - `internal/replay/sender/sender.go`: `Send(ctx, Request{Target{Host,Port,TLS,SNI}, Raw, Options{Timeout, FollowRedirects}}) (Result{Hops}, error)`.
    Fresh `net.Dialer` per hop, TLS with `InsecureSkipVerify`, `ServerName = SNI or Host` (no SNI for IP hosts), ALPN
    `http/1.1`. `Raw` is written unchanged. The timeout is a `context.WithTimeout` over the whole send (dial to last byte,
    redirects included); `context.AfterFunc` closes the connection on timeout/cancel. Body is read through
    `rawhttp.ReadResponse` up to 64 MiB (`DefaultLimits`), then `Truncated()` and the connection is closed.
    Redirects (opt-in, `MaxRedirects = 10`): 301/302/303/307/308 + `Location`, resolved with `net/url`, edits via
    `rawhttp.Builder` (target, `Host` only when host/port change, method/body/framing rules from F-REP-2). Every hop is
    returned; a failed hop is last with `Response == nil`. Errors are `*sender.Error{Kind, Err}` with kinds
    dial/tls/timeout/cancelled/protocol (write errors count as protocol).
  - `internal/testutil/tls.go`: `NewTLSRawServer` (self-signed localhost/127.0.0.1 cert, records ClientHello SNI/ALPN,
    serves like `RawServer`).
  - `content_length.go`: package doc fixed (reviewer nit). `UpdateContentLength` unchanged; `Send` does not call it, the
    caller (T-106) applies it before `Send` when the option is on.
- How to verify: `go test -race ./internal/replay/sender`; `make check`; `make test-int` (both green).
  Timeout/cancel tests run in `testing/synctest` over `net.Pipe` (fake clock; the bubble fails on any leaked goroutine).
- Known limitations: Redirect hops always use origin-form targets. A request that `rawhttp` cannot parse is sent as-is but never redirected.
- Review fixes (codex): skip interim responses, frame malformed HEAD requests correctly, and test failed redirect hops.
  Verified with `make check` and `make test-int`.
- Follow-ups (proposed tasks): none.

## Review (reviewer)
- Verdict: changes_requested (2026-11-19, re-reviewed f47d2d8)
- Checks: `make check` green; `make test-int` green; `go test -race -count=5 ./internal/replay/sender` green. No `net/http`
  import in `internal/replay/sender` (tests included). Changed files are inside `paths`; the two docs files ride with
  this branch per the lead's third note.
- Previous findings: blocker (no sender), major (owner/plan), minor (docs outside paths, accepted by lead), nit (package doc): all resolved.
- Findings:
  - [x] [major] internal/replay/sender/sender.go:146 — one `ReadResponse` call returns the first response, including an
    interim 1xx. A server that sends `103 Early Hints` (common on CDNs) or `100 Continue` (request has
    `Expect: 100-continue`) before its `200` makes Send return the 1xx as the final response and close the connection,
    so the real response is lost. F-REP-2 says Send returns "when the response is complete", and the proxy already
    skips interims (internal/proxy/forward.go:133-139). Listing it as a known limitation does not make it spec behaviour.
    → Read in a loop on the same `bufio.Reader` while `resp.Status()/100 == 1 && resp.Status() != 101` (about 4 lines).
    Add a test where the server writes `HTTP/1.1 100 Continue\r\n\r\n` and `HTTP/1.1 103 Early Hints\r\nLink: </a>\r\n\r\n`,
    then `RawReply`, and expect the 200. Remove the limitation from the Handoff.
  - [x] [minor] internal/replay/sender/sender.go:142-145 — the method used to frame the response comes from a full
    `rawhttp.ReadRequest`, so any request that does not parse gets method `""`. Example:
    `HEAD / HTTP/1.1\r\nContent-Length: x\r\n\r\n` (malformed requests are the point of the tool). The server answers
    the HEAD with `Content-Length: 1234` and no body, and Send waits for 1234 body bytes until the timeout (or forever
    when the timeout is 0). → Take the method from the first token of the request line:
    `method, _, _ := bytes.Cut(raw, []byte(" "))` (only HEAD matters to `ReadResponse`). Add a table row with an
    unparseable HEAD.
  - [x] [minor] internal/replay/sender/sender_test.go:413 — no test for a redirect chain whose second hop fails. The
    Handoff promises that the failed hop comes last with `Response == nil` and the error is returned, and T-106 stores hops
    based on that. → Add a test: origin 302 to `http://127.0.0.1:<closed port>/`, then expect 2 hops,
    `Hops[1].Response == nil`, `Hops[0].Response.Status() == 302` and kind `dial`.
  - [x] [nit] internal/replay/sender/sender.go:222-225 — draining `req.Body()` so that `Builder.WriteTo` writes no body
    depends on the builder sharing the drained reader. That works with today's `rawhttp`, but the reason is not obvious.
    → Extend the comment: "Builder streams m.Body(); once it is drained, nothing is written and no Content-Length is added (SetBody would add one)."
- Notes: the design is small and matches the plan. It uses a fresh dial per hop, `context.AfterFunc` closes the
  connection to unblock I/O, and the capped read goes through `rawhttp` capture. Error kinds come from `ctx.Err()`. The
  synctest timeout and cancel tests are good leak checks. `pipeDial` swaps the package-level `dial`. That is safe today
  because the tests that use it are sequential and top-level parallel tests start only after them. Keep it that way.
