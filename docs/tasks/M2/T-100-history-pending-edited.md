---
id: T-100
title: History pending records, edited pairs and editedBy
milestone: M2
role: backend
status: in_progress
depends_on: [T-030]
owner: Codex
branch: task/T-100-history-pending-edited
paths:
  - internal/history/**
  - internal/proxy/**
  - internal/store/requests*.go
  - internal/api/graphql/resolvers/history*.go
  - internal/api/graphql/resolvers/stubs_history_edited.go
  - internal/app/app.go
spec:
  - docs/03-architecture.md §4 (pending rows, T-100 paragraph), §5, §7
  - docs/01-feature-spec.md#F-NET-1 (upstream failures)
  - docs/01-feature-spec.md#F-HIST-4 (pair model)
---

## Goal
Requests appear in history as soon as they are read (pending row, response null), are completed when the response
arrives, and the capture API can store the edited record of a pair. `HttpRequest.editedBy` resolves.

## Scope
- In: `history.Capture.NewPending(ctx, Entry) int64` (ID returned at once; IDs from `MAX(id)+1` at project open),
  `Complete(ctx, id, *Response)`, a dropped flag, `EditedFrom` + `Source` on entries; the proxy switched from "insert when
  complete" to `NewPending` at request read and `Complete` after the response (or failure); `editedBy` lookup in
  `history.Service` + resolver (delete `stubs_history_edited.go`).
- Out: creating edited pairs from the proxy (T-105 calls the API added here), intercept, M&R.

## Acceptance criteria
- [ ] `NewPending` returns without waiting for a flush (`testing/synctest`: returns before `FlushInterval` elapses); IDs are
      unique and increasing, also after closing and reopening the project.
- [ ] Proxy integration test with a slow test server: the request is listed by `Service.List` with no response before the
      server answers; `Added` then `Updated` events fire in that order; the final row equals what M1 stored (bytes, timing).
- [ ] An exchange that fails upstream is completed as today (F-NET-1): the pending row is completed with the
      proxy-generated `502` as its response, error text in the body. `TestUpstreamErrorIs502AndRecorded` stays as is.
- [ ] `Complete` for an ID whose project was closed meanwhile is dropped and counted in `Dropped()`, no error, no panic.
- [ ] Dropped flag stored as `is_dropped = 1`; an entry with `EditedFrom` stores `edited_from`; `editedBy` returns the edited
      record, null otherwise (service + resolver tests).
- [ ] Capture benchmark (add `BenchmarkCapture` if missing): pending + complete within 15% of the M1 single insert; numbers in Handoff.
- [ ] `make check` and `make test-int` green.

## Notes from lead
- Keep one writer goroutine per DB: pending inserts and completions go through the same channel so a completion can never
  overtake its insert. A completion may land in the same batch as its insert; then emit `Added` before `Updated`.
- The in-memory ID counter is per open project; reset it on project switch (`project.Service.Subscribe`).
- Plan mode required (`internal/proxy`, `internal/store`).

## Plan (implementer, ≤15 lines)
- Confirm how history should record upstream failures and authorize the project subscription wiring.
- Allocate IDs per open project and batch pending inserts and completions on the capture writer.
- Publish pending proxy records while preserving streaming and final request and response bytes.
- Persist dropped and edited metadata and resolve `editedBy`.
- Cover capture, proxy, and resolver behavior, then benchmark and run both check targets.

## Out-of-scope change needed (if any)
- ~~Add `internal/app/app.go` to `paths`~~ → granted by lead (see below).

## Blocked questions
- Should a failed upstream exchange keep `response: null` in history, as the acceptance criterion
  says, or store the proxy-generated 502, as `TestUpstreamErrorIs502AndRecorded` currently requires?
  The client receives a 502 in either case.
  - **Lead (2026-11-19):** store the 502. F-NET-1 is the source of truth; the AC wording was wrong and is fixed.
    `Complete(ctx, id, resp)` gets the generated 502 response, same as M1 stores it today. `response: null` remains
    only for rows still pending (and dropped rows, T-105).
- Can T-100 include `internal/app/app.go` for the project subscription wiring?
  - **Lead (2026-11-19):** yes, `internal/app/app.go` added to `paths`, limited to wiring capture to
    `project.Service.Subscribe` (ID-counter reset on project switch). No other app changes; the rest of M2 wiring stays
    in T-109, which runs after T-105 → T-100, so there is no overlap.

## Handoff (implementer)
- What changed:
- How to verify:
- Known limitations:
- Follow-ups (proposed tasks):

## Review (reviewer)
- Verdict: approve | changes_requested
- Findings:
