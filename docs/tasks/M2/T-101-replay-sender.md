---
id: T-101
title: Replay sender - raw send over a fresh connection, Content-Length rule, timeout, cancel, redirects
milestone: M2
role: backend
status: blocked
depends_on: []
owner: ""
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
- [ ] F-REP-2 AC: `GET / HTTP/1.1` with a wrong `Content-Length` and update off → the test server receives exactly those bytes.
- [ ] Content-Length table test: `Transfer-Encoding` present → untouched; body without CL → added; wrong CL → fixed; no
      body and no CL → untouched; duplicate CL headers → one, first position kept.
- [ ] TLS test server sees the SNI override (and the host when none), ALPN `http/1.1`; self-signed cert accepted.
- [ ] Timeout covers a server that stalls mid-body (error kind timeout); cancel closes the connection, returns kind
      cancelled, leaves no goroutine (`testing/synctest` or a leak check).
- [ ] Response over the cap → result truncated, connection closed.
- [ ] Redirects: off by default; on → 10-hop limit (11th returns the 3xx as final), 303 after POST → GET without body and
      without `Content-Length`/`Content-Type`/`Transfer-Encoding`, 307 keeps method and body, relative `Location`
      resolved, cross-host hop changes host/port/TLS/SNI and `Host`; every hop returned.
- [ ] No `net/http` import outside tests (ADR-0003); `make check` and `make test-int` green.

## Notes from lead
- Plan mode recommended (`rawhttp`-adjacent). Reuse `rawhttp.Builder` for header edits instead of hand-splicing bytes.
- Runs fully in parallel with T-033 (no shared paths).
- 2026-11-19 unblock: an implementer set this to `blocked` because the sender sends malformed framing verbatim.
  That is intended behaviour, not a spec gap: `docs/00-overview.md §2` goal 2 (clarified today), ADR-0003, F-REP-2 AC.
  Sends are always started by the operator, to the host/port they entered. The package adds no target discovery, payload
  generation or automatic retries. Request validation or "fixing" before send is **rejected** because it breaks the
  F-REP-2 AC. The only rewrite is the opt-in Content-Length update (F-INT-1). Acceptance criteria are unchanged. Tests
  run only against local `testutil` servers on 127.0.0.1 (AGENTS.md rule 7: no real targets).

## Plan (implementer, ≤15 lines)
- Implement and test the offline Content-Length correction with `rawhttp.Builder`.
- Blocked on the sender acceptance criterion for transmitting deliberately malformed HTTP framing in request-smuggling tests.

## Out-of-scope change needed (if any)

## Handoff (implementer)
- What changed: Added offline `sender.UpdateContentLength` with byte-counting and header-preservation tests.
- How to verify: `go test -race ./internal/replay/sender ./internal/rawhttp`.
- Known limitations: The helper preserves input when `rawhttp` rejects the head, including conflicting Content-Length values. The sender is unimplemented.
- Follow-ups (proposed tasks): Define a sender contract for validated HTTP framing if that meets the Replay use case.

## Review (reviewer)
- Verdict: approve | changes_requested
- Findings:
