---
id: T-033
title: Replay service - collections, sessions, entries (no sending)
milestone: M2
role: backend
status: todo
depends_on: [T-098]
owner: ""
branch: task/T-033-replay-service
paths:
  - internal/replay/*.go
  - internal/store/migrations/project/0003_replay.sql
  - internal/api/graphql/resolvers/replay.go
  - internal/api/graphql/resolvers/replay_test.go
  - internal/api/graphql/resolvers/stubs_replay.go
spec:
  - docs/01-feature-spec.md#F-REP-1 (incl. M2 contract), #F-REP-2 (session fields)
  - docs/03-architecture.md §5.2, §6
---

## Goal
Replay collections and HTTP sessions persist per project with their draft, connection, options and entries, served
through the T-098 contract (everything except `sendReplaySession`/`cancelReplaySend`, which are T-106).

## Scope
- In: `internal/replay` service (top-level package files only; `internal/replay/sender` is T-101): lazy Default collection,
  collection CRUD + reorder, session create (from history request or from scratch) / update / colour / duplicate / move /
  delete, entries list (newest first, cursor paging) and `AddEntry(ctx, sessionID, requestID, errText)` for T-106;
  migration `0003_replay.sql` (adds `replay_entries.error`, plus any session columns you need for draft/colour/connection —
  `state_json` is fine too; document the choice); resolvers in `replay.go`; delete `stubs_replay.go`, move the
  `ReplayService` interface into `replay.go`.
- Out: sending, interpolation, redirects (T-101/T-106), WS/Pipeline kinds.

## Acceptance criteria
- [ ] Default collection: created once even with concurrent first calls; rename allowed; delete → `CONFLICT`; sessions
      created without `collectionId` land in it.
- [ ] Create from history request copies its stored raw request and host/port/TLS (SNI = host); truncated record →
      `BAD_INPUT`; unknown request → `NOT_FOUND`; default name `METHOD path` cut to 60 characters.
- [ ] Update/duplicate/move: `(copy)` naming, draft/connection/options/colour copied without entries; `moveReplaySession`
      places the session at `index` (clamped) in the target collection; `reorderReplayCollections` with a set that is not
      exactly the current one → `BAD_INPUT`.
- [ ] Validation: names 1–200 characters after trim, port 1–65535, timeout 1000–300000 ms → `BAD_INPUT`.
- [ ] Deleting a collection deletes its sessions and entries; deleting a session deletes its entries; request records stay.
- [ ] Entries paging stable and newest first; `AddEntry` with an error string stores it (service tests).
- [ ] Resolver tests for the main paths; `make check` green.

## Notes from lead
- Migration number 0003 is reserved for this task; T-037 uses 0004 and T-102 0005 if they need one.
- `sending` on `ReplaySession` comes from T-106; until then the resolver returns false.

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
