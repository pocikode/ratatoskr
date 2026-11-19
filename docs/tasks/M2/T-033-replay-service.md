---
id: T-033
title: Replay service - collections, sessions, entries (no sending)
milestone: M2
role: backend
status: in_review
depends_on: [T-098]
owner: "codex (GPT-6)"
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
- [x] Default collection: created once even with concurrent first calls; rename allowed; delete → `CONFLICT`; sessions
      created without `collectionId` land in it.
- [x] Create from history request copies its stored raw request and host/port/TLS (SNI = host); truncated record →
      `BAD_INPUT`; unknown request → `NOT_FOUND`; default name `METHOD path` cut to 60 characters.
- [x] Update/duplicate/move: `(copy)` naming, draft/connection/options/colour copied without entries; `moveReplaySession`
      places the session at `index` (clamped) in the target collection; `reorderReplayCollections` with a set that is not
      exactly the current one → `BAD_INPUT`.
- [x] Validation: names 1–200 characters after trim, port 1–65535, timeout 1000–300000 ms → `BAD_INPUT`.
- [x] Deleting a collection deletes its sessions and entries; deleting a session deletes its entries; request records stay.
- [x] Entries paging stable and newest first; `AddEntry` with an error string stores it (service tests).
- [x] Resolver tests for the main paths; `make check` green.

## Notes from lead
- Migration number 0003 is reserved for this task; T-037 uses 0004 and T-102 0005 if they need one.
- `sending` on `ReplaySession` comes from T-106; until then the resolver returns false.

## Plan (implementer, ≤15 lines)
1. Add migration 0003: unique Default marker, draft BLOB, entry error and stable entry IDs.
2. Persist connection/options/colour in existing state_json; preserve draft bytes separately.
3. Implement project-scoped transactional collection/session CRUD and stable entry paging.
4. Replace CRUD stubs with thin service adapters; retain sending=false until T-106.
5. Test concurrency, validation, history copies, ordering, cascades, paging and GraphQL paths.
6. Run make check, complete Handoff and submit in_review.

## Out-of-scope change needed (if any)

## Handoff (implementer)
- What changed: transactional, project-scoped Replay collection/session CRUD, lazy unique Default,
  history-copy validation, byte-exact drafts, atomic options updates, colour, duplication and position moves;
  stable newest-first entry paging and AddEntry failure text. Replaced CRUD stubs with thin GraphQL resolvers
  and moved ReplayService into replay.go; sending remains false as assigned.
- Storage: migration 0003 adds a unique is_default marker, a draft BLOB, and nullable entry error.
  Connection/options/colour use existing state_json. Entry IDs are AUTOINCREMENT so deleted cursor IDs
  cannot be reused. Collection deletion explicitly deletes sessions because the original FK uses SET NULL;
  entry cascades retain History records. AddEntry takes createdAt from the linked request's creation time.
- How to verify: make check passed (Go fix/vet/lint, all Go race tests, web diagnostics/lint/formatting,
  42 web test files / 329 tests); make test-int passed. Service tests cover concurrent first use, validation,
  history byte fidelity and size/truncation guards, atomic updates, duplicate/move order, cascades,
  deleted-cursor paging, project isolation and migration of populated v2 databases. GraphQL tests exercise
  CRUD, nested collections/sessions/entries/History, empty versus null Bytes, and contract error codes.
  Initial make check lacked worktree web dependencies; bun install --frozen-lockfile resolved it without
  changing tracked dependency files. Final check exited 0.
- Known limitations: no sending/cancellation/interpolation/redirects (T-101/T-106), no WS/Pipeline kinds.
  No browser E2E was run; GraphQL integration tests cover this backend task. App wiring is T-109, so the
  running binary continues to report NOT_IMPLEMENTED until that task injects replay.New(projects.DB).
- Follow-ups (proposed tasks): existing T-106 consumes AddEntry and replaces sending=false; it must bind
  sends to their starting project and preserve send-start timestamps across redirect hops. Existing T-109
  wires the service into app.New. No new tasks needed.

## Review (reviewer)
- Verdict: approve | changes_requested
- Findings:
