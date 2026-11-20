---
id: T-113
title: Replay - report ReplaySession.sending, cancel in-flight sends on session/collection delete
milestone: M2
role: backend
status: todo
depends_on: [T-106]
owner: ""
branch: task/T-113-replay-sending-flag
paths:
  - internal/api/graphql/resolvers/replay.go
  - internal/api/graphql/resolvers/replay_test.go
  - internal/api/graphql/resolvers/replay_send.go
  - internal/api/graphql/resolvers/replay_send_test.go
  - internal/api/graphql/resolvers/resolver.go
  - internal/replay/send.go
  - internal/replay/send_test.go
spec:
  - internal/api/graphql/schema/replay.graphqls (ReplaySession.sending, deleteReplaySession, deleteReplayCollection, cancelReplaySend)
  - docs/01-feature-spec.md#F-REP-1, #F-REP-2
---

## Goal
The parts of the Replay contract that T-106 left out (listed in its follow-ups) now work: `ReplaySession.sending` reports a
send in flight, and deleting a session or collection cancels its in-flight sends, as the schema says.

## Scope
- In:
  - `ReplaySession.sending` is true while `sendReplaySession` for that session is in flight (`ReplaySend.Sending(id)`),
    false otherwise and when `ReplaySend` is nil. It applies to every resolver that returns a `ReplaySession`.
  - `deleteReplaySession` cancels that session's in-flight send; `deleteReplayCollection` cancels the in-flight sends of
    every session in the collection. Nothing else about the delete changes (errors, `CONFLICT` for Default).
  - A send whose session was deleted while it ran finishes without a stray entry or panic. `sendReplaySession` returns
    `NOT_FOUND` for it (the session is gone); its request records stay in HTTP History.
  - Fix the stale comment at `resolvers/resolver.go` (`ReplaySend ... until T-106 send and cancel report NOT_IMPLEMENTED`):
    say it is nil until T-109 wires it.
- Out: schema changes; UI; wiring in `internal/app` (T-109); a send still preparing (not yet cancellable) may finish its
  network step, but it must still not leave an entry for a deleted session.

## Acceptance criteria
- [ ] Resolver test: `replaySession`/`replaySessions` report `sending: true` while a send blocks on the network and
      `false` after it returns; `false` with `ReplaySend` nil.
- [ ] Test: deleting a session during a blocked send makes the send return promptly (no wait for the timeout), stores no
      entry, and the delete returns the ID.
- [ ] Test: deleting a collection cancels the in-flight sends of all its sessions the same way.
- [ ] `go test -race` on `internal/replay` and `internal/api/graphql/resolvers` passes with `-count=5`.
- [ ] `make check` green.

## Notes from lead
- `Sender.Cancel` only cancels during the network step (`cancellable`); during preparation/storing it reports false. The
  store step after a cancel must cope with the session row being gone (FK / not found) — map that to `NOT_FOUND`
  rather than storing an orphan entry or returning an internal error.
- Collection delete: get the collection's session IDs from the Replay service before deleting, then cancel each.
  Simplest order: cancel, then delete. Do not hold `Sender.mu` across service calls.
- `sending` is per open project (`flightKey{db, id}`); keep it that way.
- Use `testutil.NewRawServer` (or a listener that accepts and never answers) to hold a send on the network.
- Shell rules (autonomous mode): one simple allowlisted command per call, no pipes/chains/`cd`.

## Plan (implementer, ≤15 lines)

## Out-of-scope change needed (if any)

## Handoff (implementer)
- What changed:
- How to verify:
- Known limitations:
- Follow-ups (proposed tasks):

## Review (reviewer)
- Verdict:
- Findings:
