---
id: T-106
title: Replay send and cancel - interpolation, M&R hook, records and entries
milestone: M2
role: backend
status: in_progress
depends_on: [T-033, T-101, T-100, T-037]
owner: "claude-code (opus)"
branch: task/T-106-replay-send
paths:
  - internal/replay/send*.go
  - internal/api/graphql/resolvers/replay_send*.go
  - internal/api/graphql/resolvers/stubs_replay_send.go
spec:
  - docs/01-feature-spec.md#F-REP-2 (M2 contract: steps, entries, failures)
  - docs/01-feature-spec.md#F-ENV-1 (interpolation)
  - docs/01-feature-spec.md#F-HIST-4 (Replay never creates a pair)
---

## Goal
`sendReplaySession` and `cancelReplaySend` work end to end: draft saved, `{{var}}` interpolated, optional Match & Replace,
Content-Length rule, sent with T-101, stored as history records + a Replay entry.

## Scope
- In: `replay.Service.Send(ctx, sessionID, input)` and `Cancel(sessionID) bool` in `send*.go`: save draft/connection/options
  (T-033 API), interpolate with the environment variables (selected over Global, via a small interface satisfied by
  `environment.Service` from T-037), apply a rewriter for source `replay` when `applyMatchReplace` is on (interface with
  the `Request` method shape of `proxy.Rewriter`, see T-105; nil = skip), send via `sender`, store each hop as a request
  record (source `replay`) through the T-100 capture API, `AddEntry` for the last one (with the error text on failure),
  one in-flight send per session, `sending` flag for the resolver; resolvers in `replay_send.go`; delete `stubs_replay_send.go`.
- Out: app wiring (T-109), M&R engine itself.

## Acceptance criteria
- [ ] Send stores the draft, one request record (source `replay`) with its response and one entry; returns the entry; the
      history bus gets `Added` then `Updated` for the record.
- [ ] Second send on the same session while one is running → `CONFLICT`; `cancelReplaySend` returns true and the running
      send returns an entry whose error says it was cancelled; without a running send it returns false.
- [ ] Interpolation: selected env value wins over Global; unknown `{{x}}` sent verbatim; the stored record holds the
      interpolated bytes (test with a fake env source).
- [ ] `applyMatchReplace` on → fake rewriter called with source `replay`; off or nil rewriter → not called.
- [ ] Redirect hops each become a record; the entry points to the last; dial failure → entry with error, record without response.
- [ ] `make check` and `make test-int` green.

## Notes from lead
- Records must be stored with their final bytes before the entry is added, so `ReplayEntry.request` never dangles.
- The blocking mutation can last up to the timeout (≤ 300 s): check the API server has no shorter write timeout and
  report it under "Out-of-scope change needed" if it does.

- Lead answer (2026-11-20, orchestrator): No rescope. Replay send is the core, user-initiated feature of this local
  proxy (F-REP-2); the user sends their own edited request to a target they choose, as in every intercepting proxy.
  The scope above stands. Previous implementer (sol) declined; task reassigned to `opus` per routing §3.

## Plan (implementer, ≤15 lines)
- `Service`'s struct lives in service.go (outside `paths`), so the send state sits in a new `replay.Sender` (send.go)
  wrapping `*Service`: `Send(ctx, id, SendInput) (Entry, error)`, `Cancel(ctx, id) (bool, error)`, `Sending(id) bool`.
- Deps as small interfaces: `Env.Vars` (environment.Service), `Rewriter.Request` (proxy.Rewriter shape, nil = skip),
  `Recorder` = `NewPending/Complete/Written/Paused` (*history.Capture).
- Send: claim per-session slot (CONFLICT) → `UpdateSession` (validate + save draft/connection/options) → interpolate
  (`environment.Interpolate`) → M&R `replay` → `sender.UpdateContentLength` → parse head (BAD_INPUT) → `sender.Send`
  → per hop `NewPending`+`Complete` (source `replay`, ProjectDB pinned) → `Written(last)` → `AddEntry(last, errText)`.
- Storing uses `WithoutCancel` + 10 s bound so a cancelled send still stores its record and entry.
- Error texts: `cancelled`, `timeout after N ms`, `dial failed: …`, `TLS handshake failed: …`, `protocol error: …`.
- Resolvers in replay_send.go; tests: service tests with testutil raw server + fake env/rewriter + real Capture.
- API server: `uiSrv` (internal/proxy/server.go) has no Read/Write timeout, only ReadHeaderTimeout/IdleTimeout: OK.

## Out-of-scope change needed (if any)

## Handoff (implementer)
- What changed:
- How to verify:
- Known limitations:
- Follow-ups (proposed tasks):

## Review (reviewer)
- Verdict: approve | changes_requested
- Findings:
