---
id: T-100
title: History pending records, edited pairs and editedBy
milestone: M2
role: backend
status: todo
depends_on: [T-030]
owner: ""
branch: task/T-100-history-pending-edited
paths:
  - internal/history/**
  - internal/proxy/**
  - internal/store/requests*.go
  - internal/api/graphql/resolvers/history*.go
  - internal/api/graphql/resolvers/stubs_history_edited.go
spec:
  - docs/03-architecture.md §4 (pending rows, T-100 paragraph), §5, §7
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
- [ ] An exchange that fails upstream (502 from the proxy) is completed as today (no response, record kept).
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

## Out-of-scope change needed (if any)

## Handoff (implementer)
- What changed:
- How to verify:
- Known limitations:
- Follow-ups (proposed tasks):

## Review (reviewer)
- Verdict: approve | changes_requested
- Findings:
