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
  - internal/api/graphql/resolvers/scope.go
  - internal/store/requests.go
  - internal/store/requests_test.go
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

- **Blocked questions answered (lead, 2026-11-19)** — see F-REP-1 and `docs/03-architecture.md §5.2`:
  1. *Request-only truncation*: migration 0003 adds `requests.request_truncated INTEGER NOT NULL DEFAULT 0` and backfills
     `UPDATE requests SET request_truncated = truncated` (old combined flags cannot be split, so they stay rejected).
     `store.InsertRequestTx` writes `request_truncated = r.Truncated`: at insert time `Truncated` is request-only
     (`history/capture.go` only ORs response truncation into `truncated` later), so `capture.go` needs no change.
     Replay reads `request_truncated`. Tests: store insert sets the column; Replay copies a request whose response was
     truncated and rejects a truncated request; migration backfill of a populated v2 database.
  2. *SNI*: yes, unset SNI satisfies "SNI = host". Drop the explicit override on history copy; the test asserts `sni` is
     null and that a later host edit is used as SNI.
  3. *Reorder nit*: `scope.go` added to paths; give `parseOrderIDs` neutral wording (e.g. `unknown ID %q in order`) and reuse it.

## Plan (implementer, ≤15 lines)
1. Add migration 0003: unique Default marker, draft BLOB, entry error and stable entry IDs.
2. Persist connection/options/colour in existing state_json; preserve draft bytes separately.
3. Implement project-scoped transactional collection/session CRUD and stable entry paging.
4. Replace CRUD stubs with thin service adapters; retain sending=false until T-106.
5. Test concurrency, validation, history copies, ordering, cascades, paging and GraphQL paths.
6. Run make check, complete Handoff and submit in_review.

## Out-of-scope change needed (if any)
- None remaining. Lead expanded paths and resolved request-truncation backfill and SNI semantics on 2026-11-19.

## Handoff (implementer)
- Current status: in_review; all three review findings addressed. Follow-up verification: make check and
  make test-int both passed (including Go race tests and 42 frontend test files / 329 tests).
- Review fixes: migration 0003 adds and backfills request_truncated from the old combined flag;
  InsertRequestTx persists request-only truncation before response capture updates the combined flag.
  Replay uses request_truncated, so truncated responses no longer prevent copying complete requests.
  History copies leave SNI unset so it follows host edits. Replay reorder reuses parseOrderIDs with neutral errors.
- Regression coverage: store insertion with both truncation states, populated v2 backfill, Replay copy with
  response-only truncation versus request truncation, null SNI after copy/host edit, and GraphQL null SNI.
- What changed: transactional, project-scoped Replay collection/session CRUD, lazy unique Default,
  history-copy validation, byte-exact drafts, atomic options updates, colour, duplication and position moves;
  stable newest-first entry paging and AddEntry failure text. Replaced CRUD stubs with thin GraphQL resolvers
  and moved ReplayService into replay.go; sending remains false as assigned.
- Storage: migration 0003 adds a unique is_default marker, a draft BLOB, and nullable entry error.
  Migration 0003 also tracks request-only truncation and conservatively backfills old combined flags.
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
- Verdict: changes_requested
- Checks: all changed files are inside `paths`. `make check` green (exit 0). `make test-int` not re-run (store/proxy/rawhttp
  code untouched; the migration is covered by `internal/replay/migration_test.go`). AC 1–7 are covered by service and resolver tests.
- Findings:
  - [x] [major] internal/replay/session.go:79 — `requests.truncated` is not "the request was truncated". Capture also sets it
        when only the **response** was cut (`internal/history/capture.go:480-484`: `truncated = truncated OR resp.Truncated`), and
        `requests.len` is the stored length (`internal/store/requests.go:48`), so this code cannot tell the two apart. Example: a
        complete `GET /big.iso` whose response went over the 64 MiB capture cap → Ctrl+R / `createReplaySession(fromRequestId)`
        returns `BAD_INPUT`, but the spec only rejects a truncated *stored request* (F-REP-1 contract, schema
        `CreateReplaySessionInput.fromRequestId`). The fix needs a separate request-only flag (e.g. a column added in 0003 plus
        a write in `internal/history/capture.go`), and that file is outside this task's paths → write it under "Out-of-scope
        change needed" (or set `blocked` for the lead), then add a service test where a request with a truncated response
        can be copied and a truncated request is rejected.
  - [x] [minor] internal/replay/session.go:82 — copying from History stores `SNI = host` as an explicit override. The schema says
        `sni: null` already means "host as SNI when it is a name", so this gives the same result on the wire, but the override
        stays when the user later changes the host. Example: copy from `a.test`, change the connection host to `b.test` → TLS to
        `b.test` with SNI `a.test`. The AC says "SNI = host", so this is not a defect of this task: ask the lead whether a
        `null` SNI satisfies it. If yes, drop the line and update the history test.
  - [x] [nit] internal/api/graphql/resolvers/replay.go:141-146 — duplicates `parseOrderIDs` (`resolvers/scope.go:44`), which
        already maps an unparsable ID to `BAD_INPUT`. Reuse it. Its message says "preset", so pass a kind or make the
        wording generic.

- Implementer follow-up (2026-11-19): all three findings addressed per lead decisions above. Request-only
  truncation is written in InsertRequestTx; capture.go needs no change. Legacy combined flags remain
  conservatively rejected. The SNI override stays null, and reorder ID parsing now uses the shared helper.

### Re-review (reviewer, 2026-11-19, commit 33e41b1)
- Verdict: approve
- Checks: every code file is inside `paths`. The `docs/01`/`docs/03` edits are the lead's decision commit (fc326ae).
  `make check` passed (exit 0; Go race tests, lint, web check, 42 files / 329 tests). `make test-int` passed (exit 0),
  run because `internal/store/requests.go` changed.
- Previous findings verified:
  - major (truncation): migration 0003 adds `request_truncated` and copies the old `truncated` value into it.
    `InsertRequestTx` writes it from `r.Truncated`. At insert time that value is request-only: `capture.go:563`
    takes `e.Truncated`, which is `req.Truncated()` (`proxy/forward.go:34`). The later
    `UPDATE … truncated = truncated OR ?` (`capture.go:483`) does not touch `request_truncated`.
    `sourceRequest` reads the new column. Tests cover the insert (`store/requests_test.go`), the backfill of a
    populated v2 DB (`replay/migration_test.go`), and copy allowed after response-only truncation vs rejected
    for request truncation (`replay/service_test.go`).
  - minor (SNI): the override was removed. The service and GraphQL tests now check that `sni` is null, and that it
    stays null after a host edit.
  - nit (reorder): `ReorderReplayCollections` now uses `parseOrderIDs`, which has neutral wording.
- Findings: none.
