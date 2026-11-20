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
  - internal/history/capture.go
  - internal/history/capture_test.go
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

- Lead answer (2026-11-20, unblock): (1) web deps were missing in the worktree; lead ran `bun install`, `make web-check`
  green. (2) `TestRegexSharedGroupsPerMatch` wall-clock flake under -race is pre-existing, outside these paths: not a
  blocker; follow-up task to de-flake. (3) Paused capture → CONFLICT before sending: accepted (pause = no project DB or
  capture failure, an error state). (4) Out-of-scope items (sending flag in toReplaySession, cancel on delete, stale
  comment) go to a lead follow-up task; T-109 wiring stays in T-109. Ready for review.

- Lead answer (2026-11-20, review round 1 block): paths extended with `internal/history/capture.go` and
  `capture_test.go` (no other task owns them). Add the durable completion API as you proposed (`CompleteWritten`
  or similar: returns after the batch holding the completion commits, error on failed/dropped write or project change),
  additive only: do not change existing `Capture` behaviour or signatures. Regression test in `capture_test.go`.
  Replay uses it per hop. Other out-of-scope items stay as lead follow-ups.

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

- Question for lead (2026-11-20, review round 1): findings 1, 3 and 4 are fixed (`make check` green). Finding 2
  (durable completion) needs a `history.Capture` API outside my paths, see "Out-of-scope change needed". Blocked on it.

## Out-of-scope change needed (if any)
- **Blocking (review finding 2), `internal/history/capture.go`:** a durable completion API, e.g.
  `func (c *Capture) CompleteWritten(ctx context.Context, id int64, resp *history.Response) error` that queues the
  completion with a `written` barrier *behind the completion op* and returns nil only after the batch holding the
  completion committed (error when the write failed, was dropped, or the project changed). Today `Written` only
  acknowledges the insert: `Run.appendOp` closes the barrier at once when the insert was flushed in an earlier batch,
  even if the completion is still queued, and a failed completion write is invisible to the caller. With it, Replay
  calls `CompleteWritten` for every hop (nil response = release) and drops its `Written(last)` call; the regression
  test (request committed, completion pending) belongs with that API in history's tests.
  Workaround I rejected: waiting on `history.Event` Updated + polling `responses` couples Replay to the bus and only
  detects a failed write by timeout.
- `internal/api/graphql/resolvers/replay.go` `toReplaySession`: set `Sending` from `r.deps.ReplaySend.Sending(id)`
  (nil-safe). Today `ReplaySession.sending` is always false. `Sender.Sending(id)` is ready.
- `internal/api/graphql/resolvers/resolver.go:39`: comment still says "until T-106 … NOT_IMPLEMENTED".
- `deleteReplaySession` / `deleteReplayCollection` should cancel in-flight sends (schema says so). Needs
  `Sender.Cancel` called from the resolver or session.go. Today the send finishes and `AddEntry` returns NOT_FOUND.
- T-109 wiring: `replay.NewSender(replaySvc, replay.SendDeps{Env: envSvc, Rewriter: mrEngine, Capture: capture})`.

## Handoff (implementer)
- What changed:
  - `internal/replay/send.go`: `Sender` (wraps `*Service`, whose struct is outside my paths) with
    `Send(ctx, id, SendInput)`, `Cancel(ctx, id) (bool, error)` (NOT_FOUND for unknown id), `Sending(id)`.
    Flow: one-per-session slot (CONFLICT) → `UpdateSession` saves draft/connection/options (validates, nothing saved
    on bad input) → `environment.Interpolate` with `Env.Vars` → `Rewriter.Request(ctx, "replay", raw)` when
    `applyMatchReplace` → `sender.UpdateContentLength` → head must parse (BAD_INPUT, draft stays saved) →
    `sender.Send` → each hop `NewPending`+`Complete` (source `replay`, `ProjectDB` pinned) → `Written(last)` →
    `AddEntry(last, error)`. Storing runs on `WithoutCancel` + 10 s bound so cancelled sends are stored.
  - Error texts: `cancelled`, `timeout after N ms`, `dial failed: …`, `TLS handshake failed: …`, `protocol error: …`.
  - `resolvers/replay_send.go` replaces `stubs_replay_send.go`; `ReplaySendService` is now an interface.
  - API server write timeout: `uiSrv` in internal/proxy/server.go has no Read/WriteTimeout, so 300 s sends are fine.
- How to verify: `go test -race ./internal/replay/ ./internal/api/graphql/resolvers/ -run 'Send'`.
  Tests cover every AC: draft+record+entry+Added/Updated, CONFLICT/cancel/false/NOT_FOUND, interpolation incl.
  unknown verbatim and stored interpolated bytes, M&R on/off/nil, Content-Length on/off, redirect hops, dial failure,
  BAD_INPUT parse/connection, paused capture.
- Known limitations:
  - Decision (technical, please confirm): when history capture is paused, Send returns CONFLICT before sending,
    because no record (and so no entry) could be stored. Alternative needs a capture API that ignores the pause.
  - Review round 1: sends are pinned to the project open at start (`{db, id}` in-flight key); `Cancel` acts only
    during the network phase (false while preparing/storing). See Review replies.
  - Still open (review finding 2): `Written` only confirms the last request row, so its response can land one flush
    (≤ 50 ms) after the entry, and a failed completion write is not detected. Needs the history API requested above.
  - Selected-over-Global precedence is `environment.Service.Vars`' job (tested in T-037); here a fake env is used.
  - No timeout test: sender uses real network time, so `synctest` doesn't apply; timeout text mapping is trivial.
- Follow-ups (proposed tasks): the out-of-scope items above; de-flake `TestRegexSharedGroupsPerMatch` (wall-clock bound).

## Review (reviewer)
- Verdict: changes_requested
- Findings:
  - [x] [major] internal/replay/send.go:120 — Saving the draft and resolving its project for capture are separate lookups. A project switch between them lets a request prepared from the old draft reach the new project's capture. A switch between `Written` and `AddEntry` can also link unrelated rows when both IDs exist in the new project. The in-flight map shares session IDs across projects, so a new project's send or cancel can also act on the old project's slot. → Bind draft persistence, preparation, capture and entry insertion to one project and key in-flight operations by project plus session ID. Add a project-switch regression with overlapping IDs. Coordinate any API changes outside the task paths with the lead.
    - Fixed (implementer): `Send` resolves the project DB once and runs `UpdateSession`, `AddEntry` through a
      `replay.New` service pinned to it; records carry `ProjectDB` so capture drops them after a switch and `Written`
      fails, so no entry is added anywhere. In-flight map keyed by `{db, sessionID}`; `Sending`/`Cancel` use the
      open project. Regression `TestSendProjectSwitch`: overlapping session IDs, switch mid-send → error, no
      requests/entries in either project, B neither sees nor cancels A's send, A's draft saved. Env vars still come
      from `Env.Vars` (current project); after a switch those bytes are never stored (capture rejects).
  - [ ] [major] internal/replay/send.go:150 — `Complete` queues the response and `Written` only confirms the request row. If a flush commits the request between those operations, the mutation can return success while `request.response` is still null. A failed completion write can also leave it null after success. → Require durable completion of each hop, including its response when present, before creating the entry. Add a regression covering a committed request with its completion still pending.
    - Open (implementer): needs a history API outside my paths; requested under "Out-of-scope change needed".
  - [x] [major] internal/replay/send.go:105 — `Cancel` returns true throughout preparation and persistence, but only the network sender maps cancellation to a cancelled entry. Cancellation during environment lookup can return a context error with no entry, and cancellation while persistence is blocked can return true while the send returns a successful entry. → Coordinate which phases accept cancellation and ensure a true result leads to a stored cancelled entry. Keep the per-session conflict guard through persistence. Cover both boundaries in tests.
    - Fixed (implementer): only the network phase is cancellable. Preparation uses the request ctx; `sender.Send`
      gets a `WithCancelCause` ctx; a flag set under the mutex right before and after `sender.Send` decides what
      `Cancel` does. Outside that window `Cancel` returns false and the send stores a normal entry. If `Cancel` wins
      the race just after the response arrived (cause = errCancelled), the send honours it: last hop stored without
      response, entry error `cancelled`. So true ⇒ cancelled entry. The conflict guard spans the whole `Send`.
      Tests: `TestSendCancelOutsideNetworkPhase/{preparing,storing}` (Cancel false, CONFLICT held, normal entry);
      network phase still covered by `TestSendConflictAndCancel`.
  - [x] [minor] internal/replay/send_test.go:230 — The wire assertion strips the trailing `ab` before comparing, so the Content-Length-off test passes even if Replay omits those bytes on the wire. → Read the full expected byte count at the loopback listener independently of HTTP framing and compare it byte for byte.
    - Fixed (implementer): `TestSendPreparesBytes` uses a `wire` listener that reads exactly `len(want)` bytes with
      `io.ReadFull`, answers, then reads to EOF; every case compares all received bytes with `want`.

- Verification: The Go race tests for Replay send and its GraphQL resolvers passed five runs. The API server has no write timeout. All source changes are within the task paths. The lead's accepted follow-ups remain deferred.
  - Initial `make check` stopped at lint with ``Running error: context loading failed: no go files to analyze: running `go mod tidy` may solve the problem``. Initial `make test-int` failed in `TestNoLegacyImports` with `go list: exit status 1: go: writing stat cache: open ...: operation not permitted` and a Go build-cache `operation not permitted` error.
  - Retried with a writable temporary `GOCACHE`, a writable temporary `GOLANGCI_LINT_CACHE` for `make check`, and `GOFLAGS=-buildvcs=false` for both commands. `make check` and `make test-int` passed.
