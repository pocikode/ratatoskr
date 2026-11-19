---
id: T-033
title: Replay service - collections, sessions, entries (no sending)
milestone: M2
role: backend
status: blocked
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
- Confirmed during review follow-up: `sourceRequest` reads `requests.truncated`, but
  `internal/history/capture.go` also sets that flag when only the response was truncated.
  `requests.len` is the stored byte count, so it cannot recover request-only truncation.
  F-REP-1 requires accepting a complete request even when its response was truncated.
- Lead: expand this task's paths or provide a prerequisite task for request-only truncation
  persistence in `internal/history/capture.go`, the store request model/insertion in
  `internal/store/requests.go`, and their tests. Migration 0003 and Replay readers/tests
  are already in scope. Define how existing records with the combined flag are handled;
  their request-only truncation cannot be determined from the current flag alone.
- Lead question (minor finding): does an unset SNI override satisfy the AC's "SNI = host",
  using F-REP-2's host fallback? If yes, remove the explicit history-copy override and
  update its test so later host edits use the new host as SNI.
- The reorder-helper nit also needs `internal/api/graphql/resolvers/scope.go` added to
  paths to generalize `parseOrderIDs`' preset-specific error wording before reuse.
- Follow-up stopped under AGENTS.md hard rules 1–2. Review findings remain unchecked;
  no implementation files changed and no checks rerun in this follow-up.

## Handoff (implementer)
- Current status: blocked on the scope expansion and lead decisions above. Verification
  results below describe the previous implementation, before the unresolved review findings.
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
- Verdict: changes_requested
- Checks: all changed files are inside `paths`. `make check` green (exit 0). `make test-int` not re-run (store/proxy/rawhttp
  code untouched; the migration is covered by `internal/replay/migration_test.go`). AC 1–7 are covered by service and resolver tests.
- Findings:
  - [ ] [major] internal/replay/session.go:79 — `requests.truncated` is not "the request was truncated". Capture also sets it
        when only the **response** was cut (`internal/history/capture.go:480-484`: `truncated = truncated OR resp.Truncated`), and
        `requests.len` is the stored length (`internal/store/requests.go:48`), so this code cannot tell the two apart. Example: a
        complete `GET /big.iso` whose response went over the 64 MiB capture cap → Ctrl+R / `createReplaySession(fromRequestId)`
        returns `BAD_INPUT`, but the spec only rejects a truncated *stored request* (F-REP-1 contract, schema
        `CreateReplaySessionInput.fromRequestId`). The fix needs a separate request-only flag (e.g. a column added in 0003 plus
        a write in `internal/history/capture.go`), and that file is outside this task's paths → write it under "Out-of-scope
        change needed" (or set `blocked` for the lead), then add a service test where a request with a truncated response
        can be copied and a truncated request is rejected.
  - [ ] [minor] internal/replay/session.go:82 — copying from History stores `SNI = host` as an explicit override. The schema says
        `sni: null` already means "host as SNI when it is a name", so this gives the same result on the wire, but the override
        stays when the user later changes the host. Example: copy from `a.test`, change the connection host to `b.test` → TLS to
        `b.test` with SNI `a.test`. The AC says "SNI = host", so this is not a defect of this task: ask the lead whether a
        `null` SNI satisfies it. If yes, drop the line and update the history test.
  - [ ] [nit] internal/api/graphql/resolvers/replay.go:141-146 — duplicates `parseOrderIDs` (`resolvers/scope.go:44`), which
        already maps an unparsable ID to `BAD_INPUT`. Reuse it. Its message says "preset", so pass a kind or make the
        wording generic.
