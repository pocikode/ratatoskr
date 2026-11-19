---
id: T-093
title: Fix CI flakes — UI hand-off shutdown race, slow-subscriber test timing
milestone: M1
role: backend
status: done
depends_on: []
owner: "claude-code/opus"
branch: task/T-093-ci-flaky-shutdown-stream
paths:
  - internal/proxy/server.go
  - internal/history/service_test.go
  - docs/tasks/T-093-ci-flaky-shutdown-stream.md
  - docs/08-roadmap.md
spec:
  - docs/03-architecture.md §5
---

## Goal
`make check` is green on CI again (`main` and PR #24 fail on these two tests).

## Scope
- In: race in `proxy.(*session).handOffToUI`; `TestSlowSubscriberIsResyncedNotBlocking` timing.
- Out: any behaviour change of the bus or the history stream.

## Acceptance criteria
- [x] AC1 — A connection handed to the UI server is never closed by the proxy's shutdown `closeAll`
      (`TestServeGracefulShutdownDrains` no longer sees `EOF`).
- [x] AC2 — The slow-subscriber test still proves: Publish never blocks, an overflowed stream ends,
      and every event queued before the overflow is delivered, without doing ~1000 SQLite reads under `-race`.
- [x] `make check` green

## Notes from lead
CI failures: run 44310294605 (PR #24), 44297810008 and 44294859396 (`main`).

## Plan (implementer, ≤15 lines)
1. `handOffToUI`: untrack the conn *before* sending it on `ui.ch`. Previously the UI server could already be
   serving it while it was still in `s.conns`, so a shutdown in that window closed it under the in-flight request.
   If the hand-off fails (`done`/ctx), `serveConn` still closes the conn since `handedOff` stays false.
2. Test: prime the stream with 32 `Added` events (fills the 16-slot output, parks the goroutine in `send`),
   then flood with `Updated` events, which `SubscribeAdded` drops without a DB read. Assert the stream ends and
   exactly the 32 primed rows arrive.

## Out-of-scope change needed (if any)

## Handoff (implementer)
- What changed: `internal/proxy/server.go` hand-off ordering; `internal/history/service_test.go` slow-subscriber test.
- How to verify: `go test -race -count=20 -run 'TestServeGracefulShutdownDrains' ./internal/app/`;
  `go test -race -count=5 -run TestSlowSubscriberIsResyncedNotBlocking ./internal/history/`; `make check`.
- Known limitations: none.
- Follow-ups (proposed tasks): none.

## Review (reviewer)
- Verdict: approve (lead, retroactive). Merged in PR #25 before a review was recorded; CI on `main` has
  stayed green since (runs 44467782046, 44468680155, 44470829179).
- Findings: none.
