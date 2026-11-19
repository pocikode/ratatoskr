---
id: T-101
title: Replay sender - raw send over a fresh connection, Content-Length rule, timeout, cancel, redirects
milestone: M2
role: backend
status: changes_requested
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
- 2026-11-19 second unblock: re-blocked with no new question. The decision above stands and is final for this task:
  the sender writes the bytes it is given, unvalidated, like any HTTP repeater. "Validated framing" is not an accepted
  contract and is not a follow-up. An implementer who will not build that should leave the task unclaimed rather than block it.
  Clarified in F-INT-1: identical duplicate CLs merge, conflicting CLs or unparseable heads are left untouched, so the
  existing `UpdateContentLength` behaviour is correct; add a test for the conflicting case. Remaining work: the sender
  itself (dial, TLS/SNI/ALPN, timeout, cancel, cap, redirects, error kinds) and its tests.

## Plan (implementer, ≤15 lines)
- Implement and test the offline Content-Length correction with `rawhttp.Builder`.

## Out-of-scope change needed (if any)

## Handoff (implementer)
- What changed: Added offline `sender.UpdateContentLength` with byte-counting and header-preservation tests.
- How to verify: `go test -race ./internal/replay/sender ./internal/rawhttp`.
- Known limitations: The helper preserves input when `rawhttp` rejects the head, including conflicting Content-Length values. The sender is unimplemented.
- Follow-ups (proposed tasks):

## Review (reviewer)
- Verdict: changes_requested (2026-11-19, reviewed 3b699b5)
- Checks: `make check` green. `make test-int` not run: no proxy/store/rawhttp change on this branch, and the sender it must cover does not exist yet.
- Findings:
  - [ ] [blocker] internal/replay/sender/ — the sender is not implemented. No `Send`, dial, TLS/SNI/ALPN, timeout, cancel, 64 MiB cap, redirects or error kinds. Six of the seven AC are uncovered: F-REP-2 verbatim send, TLS SNI/ALPN, timeout/cancel/no goroutine leak, cap truncation, redirects, `make test-int`. The Handoff says so itself ("The sender is unimplemented"). → Implement the "Remaining work" list from the second lead note, each with the AC test, before `in_review`.
  - [ ] [major] docs/tasks/M2/T-101-replay-sender.md:6 — status was `todo` with `owner: ""`, and the Plan has one line about the helper only. Lifecycle (docs/07) wants `in_progress` + owner + a ≤15-line plan covering the scope. → Claim the task and write a plan for the sender (dial/TLS, deadline + ctx close, read with cap, redirect loop, error classification, tests).
  - [ ] [minor] docs/00-overview.md:23, docs/01-feature-spec.md:167 — spec files are outside this task's `paths` (rule 1). These are lead clarifications (commits c4cc6d3, 948432b), so the content is fine. → Lead: confirm they belong on this branch, or land them on `main` separately so the task diff holds only `internal/replay/sender/**`.
  - [ ] [minor] internal/replay/sender/content_length.go:19 — the `WarnMalformedHeader` early return has no test. If it is removed, a head with a malformed field gets rebuilt and the bytes change. → Add a table row, e.g. `"Bad header line\r\n\r\nabc"` (or whatever rawhttp flags as malformed-header) expecting unchanged output.
  - [ ] [nit] internal/replay/sender/content_length.go:1 — the package doc says "prepares HTTP requests for Replay". The Goal says it sends raw bytes and is reused by Automate/workflows. → "Package sender sends raw HTTP/1.x requests over a fresh connection and reads the response with rawhttp."
- Notes: `UpdateContentLength` is correct against F-INT-1 as clarified. TE untouched, missing CL added, wrong CL fixed, identical duplicate CLs merged at the first position, conflicting CLs untouched, leading blank line kept, no `net/http`. It reuses `rawhttp.Builder` as the lead asked. Bodies are counted from the bytes after the head, so editor bodies with a wrong CL are handled.
