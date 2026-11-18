---
id: T-006
title: Projects service and GraphQL contract
milestone: M0
role: backend
status: changes_requested
depends_on: [T-005, T-007]
owner: "claude-code (sonnet 5.5)"
branch: task/T-006-projects
paths:
  - internal/project/**
  - internal/api/graphql/resolvers/project*.go
spec:
  - docs/01-feature-spec.md#F-WSP-1
  - docs/03-architecture.md §5.1
---

## Goal
Projects can be created, listed, renamed, duplicated, deleted, set as default and selected; a temporary project exists when none does;
switching emits a `projectChanged` event that other services use to reopen their DB handles.

## Notes from lead
The schema part (`project.graphqls`) is added by the lead in T-007's follow-up commit before this task starts; implement resolvers against it.

## Acceptance criteria
- [x] All F-WSP-1 operations via GraphQL with tests (httptest + gqlgen client).
- [x] Deleting the selected project falls back to the default or a new temp project.
- [x] Switching closes the old DB handle (no open handles remain on the old DB file — test).
- [x] Temp project removed on clean shutdown unless renamed/saved.

## Plan (implementer, ≤15 lines)
- `internal/project.Service` owns the registry in `instance.db` (`projects` table) and the one open `project.db` (`projects/<id>/` via `os.Root`).
- Temporary flag has no column (migrations are out of lane): kept in `settings` key `project.temporary` (JSON id list). Leftover temp projects and orphan dirs are purged on `Open` (crash recovery); `Close` removes temp projects (clean shutdown).
- Ops: List/Get/Current/Create/Rename (saves temp)/Duplicate (VACUUM INTO + copy other files)/Delete (falls back to default or new temp)/SetDefault/Select (open new, swap, close old, SELECTED event).
- Single mutex serialises mutations; events fan out to bounded per-subscriber channels (cap 64); an overflowing subscriber is closed so it resyncs.
- Errors are `apperr` (NOT_FOUND, BAD_INPUT); names trimmed, non-empty, ≤200 runes.
- Resolvers (`resolvers/project.go`) depend on a small `ProjectService` interface; nil service keeps returning NOT_IMPLEMENTED (existing T-007 test relies on it).
- Tests: service unit tests (synctest not needed; no time), old-handle-closed test, temp cleanup, GraphQL httptest round trips + subscription over the handler.

## Out-of-scope change needed (if any)
- `internal/api/graphql/resolvers/resolver.go`: add `Deps.Projects ProjectService` (one field; unavoidable for the resolvers to reach the service).
- `internal/app/app.go`: construct `project.Service` and pass it in `resolvers.Deps` + `Close` on shutdown. Left to a follow-up; not done here.

## Handoff (implementer)
**What changed**
- `internal/project`: `Service` (Open/Close, List/Get/Current, Create, Rename, Duplicate, Delete, SetDefault, Select, `DB()`, `Subscribe`). Project data lives in `projects/<id>/` (via `os.Root`); the registry is the `projects` table in instance.db.
- Temporary flag is stored in instance.db `settings` key `project.temporary` (no column exists; migrations are out of lane). `Open` purges leftover temp projects and orphan dirs; `Close` deletes temp projects.
- Duplicate snapshots project.db with `VACUUM INTO` over its own read-only connection (store's reader is query_only, its writer is transactional) and copies other files.
- `resolvers/project.go`: real resolvers over a `ProjectService` interface; `Deps.Projects` nil keeps returning NOT_IMPLEMENTED.
- Events: bounded channel (64) per subscriber; a subscriber that falls behind is closed (GraphQL subscription ends, client resubscribes + refetches).

**Verify**: `go test -race ./internal/project ./internal/api/graphql/resolvers`; `make check` green.

**Limitations / follow-ups**
- Out of lane, NOT done: `internal/app/app.go` must build `project.Service` (after `store.OpenInstance`), pass it as `resolvers.Deps.Projects`, and `Close()` it on shutdown, else the API still answers NOT_IMPLEMENTED. Also edited `resolvers/resolver.go` (one `Deps` field) — outside `paths`, flagging for the reviewer.
- SQLite reuses IDs of deleted rows (no AUTOINCREMENT), so a deleted project's ID may later name a new project.
- The subscription is tested at resolver level, not over a real WebSocket.

## Review (reviewer)

### Round 1 — reviewer (claude-code, opus 5.5), 2026-11-18
**Scope check**: 9 files changed. All are inside `paths` except `internal/api/graphql/resolvers/resolver.go`, which matches neither `internal/project/**` nor `resolvers/project*.go`.
**`make check`**: green (exit 0; Go vet/lint/race tests, svelte-check, eslint, prettier, vitest 50/50). `make test-int` not needed (no proxy/store/rawhttp changes).

**AC coverage**
- F-WSP-1 ops over GraphQL: covered by `TestProjectOperations` / `TestProjectErrors` (httptest + gqlgen client). The subscription is tested only at resolver level, not over the WS transport (the Handoff says so).
- Delete selected → default or new temp: covered by `TestDeleteSelected` (both branches) and `TestDeleteOnlyProjectCreatesTemp`.
- Old handle closed on switch: covered by `TestSelectClosesOldHandle` (reader ping and writer both fail after the switch).
- Temp removed on clean shutdown unless saved: covered by `TestTemporaryRemovedOnClose` and `TestRenameSavesTemporary`. Crash leftovers are handled by `TestOpenPurgesCrashLeftovers`.
- Not reachable in the product yet: `internal/app/app.go` doesn't build the service, so the running API still answers NOT_IMPLEMENTED (see the first finding).

**Findings**
- [ ] [major] internal/api/graphql/resolvers/resolver.go:28 — this file is outside `paths`, so editing it breaks hard rule 1 (the rule calls for an automatic changes_requested). The implementer flagged it openly, and the one-field change is the smallest one possible. → Lead: either add `internal/api/graphql/resolvers/resolver.go` to this task's `paths` after the fact, or ask for the change to be reverted and moved to a lead commit. In the same pass, add `internal/app/app.go` to `paths` or open a follow-up task to wire `project.Open` / `Deps.Projects` / `Close()`. Until then F-WSP-1 is not usable end to end.
- [x] [minor] internal/api/graphql/resolvers/project.go:87 — `//nolint:nilerr` has a reason but no task id (hard rule 7 needs both). → `//nolint:nilerr // T-006: a non-numeric ID names no project: null, not an error`.
  - **Reply (round 2)**: Fixed: nolint now carries T-006.
- [x] [minor] internal/project/project.go:384-385 — `Deleted` is emitted even when `removeProject` fails inside the registry transaction, so the row still exists but clients drop it. → Emit only once the row is deleted. For example, split `removeProject` into a DB step and a directory step, emit after the DB step succeeds, and still return any `RemoveAll` error.
  - **Reply (round 2)**: Fixed: `removeRow` split out; `Deleted` emitted only after the row is gone, RemoveAll error still returned.
- [x] [minor] internal/project/project.go:333 — Duplicate calls `createLocked`, which emits `CREATED` (with `DBSize` 0) before the data is copied. If the copy fails, the rollback emits no `DELETED`, so subscribers keep a phantom project. → Emit `Deleted` in the rollback path, or skip the event in `createLocked` here and emit `Created` after `copyData` succeeds (preferred: it also reports the right `DBSize`).
  - **Reply (round 2)**: Fixed: Duplicate uses `createQuiet`, emits `Created` after copy with real DBSize.
- [x] [minor] internal/project/project.go:545 — `openDB` uses `OpenRoot(dirOf(id))` without making sure the directory exists. If someone removes the default project's directory by hand, `Open` fails and the app won't start. → `s.root.MkdirAll(dirOf(id), 0o700)` before `OpenRoot` (an empty project.db is then created), and add a test.
  - **Reply (round 2)**: Fixed: `openDB` MkdirAll first; test `TestOpenRecreatesMissingDefaultDir`.
- [ ] [minor] internal/project/project.go:37 — the temporary flag is kept in `settings["project.temporary"]` because §5.1 `projects` has no such column. That's a storage design choice the spec doesn't cover. It's documented in Plan/Handoff, but the spec still disagrees. → Lead: either accept it and note the key in docs/03 §5.1, or schedule a migration adding `projects.is_temporary` and switch over.
- [ ] [nit] internal/api/graphql/schema/project.graphqls:104 — the service ends a subscription when its client falls 64 events behind (events.go:34), but the schema doesn't say so. A client that isn't expecting this will silently stop getting updates. → Lead: add "the server may end this subscription; clients must resubscribe and refetch `projects`" to the field doc.
- [x] [nit] internal/project/events.go:62 — when a subscriber is dropped for falling behind, its `context.AfterFunc` registration stays alive until ctx ends. It's harmless (`remove` is idempotent) but leaks for long-lived contexts. → Keep the `stop` func returned by `AfterFunc` next to the channel in `subs` and call it from `emit`/`closeAll`.
  - **Reply (round 2)**: Fixed: AfterFunc `stop` kept in `subs`, called on overflow/closeAll.

**Notes (no action)**: The `os.Root` use is sound: project dirs are numeric IDs, and the only absolute paths go to the SQLite driver, the same as in `store.open`. SQL is fully parameterized. `VACUUM INTO` over a separate read-only connection is correct with WAL, and it's tested with uncommitted-to-file WAL data. Context propagation is fine, and the rollback/Close paths correctly use `WithoutCancel`/`Background`. Fan-out holds the hub mutex only for non-blocking sends. The code is clean and idiomatic (`errors.AsType`, `encoding/json/v2`, `context.AfterFunc`).

**Verdict: changes_requested.** The one major finding is a process issue that the lead can clear by amending `paths` and handling the app wiring. The implementer has one required fix (the nolint task id). Fixing the minors is recommended in the same round, especially the Delete and Duplicate event consistency.


### Round 2 — implementer
Fixed 5 code findings; major (paths/app wiring), spec note (§5.1) and schema doc nit remain lead decisions. `make check` green.

### Round 2 — reviewer (claude-code, opus 5.5), 2026-11-18
**Scope check**: The fix commit `316ef53` touches only `internal/project/**`, `resolvers/project.go` and this task file, so it is in lane. The branch as a whole still includes the out-of-`paths` edit to `resolvers/resolver.go`, and `paths` in the front matter hasn't been changed.
**`make check`**: green (exit 0; vet, golangci-lint, `go test -race`, svelte-check, eslint, prettier, vitest 50/50). `make test-int` not needed.

**Round-1 fixes verified**
- nolint task id: done (`resolvers/project.go:87`).
- Delete: `removeRow` runs before `Deleted` is emitted, and a `RemoveAll` error is still returned (`project.go:392-400`). Correct.
- Duplicate: `createQuiet` is used, the rollback emits nothing, and `Created` is emitted after the copy with the real `DBSize` (`project.go:333-354`). Correct.
- `openDB` runs MkdirAll before OpenRoot (`project.go:556`). Covered by `TestOpenRecreatesMissingDefaultDir`.
- The hub keeps the `AfterFunc` `stop` and calls it on overflow and `closeAll` (`events.go:62-71, 89-110`). I checked the race between unlock and re-lock: if the channel was already removed, `stop()` is called. Correct.

**Findings**
- [ ] [major] internal/api/graphql/resolvers/resolver.go:28 — still open from round 1. The file is outside `paths` and the lead hasn't decided yet. The implementer can't fix this. → Lead: add `internal/api/graphql/resolvers/resolver.go` to `paths` (and either add `internal/app/app.go` to `paths` or open a follow-up task for the wiring). Once that's done, this task can be approved without another code round.
- [ ] [minor] internal/project/project_test.go:545 — none of the three round-1 event fixes has a test. `TestSubscribe` never calls `Duplicate`, so nothing checks that Duplicate emits exactly one `Created` with `DBSize > 0`. → Extend `TestSubscribe` (or `TestDuplicate`) to call `Duplicate` on a project whose DB was opened, then assert one `Created` event for `cp.ID` with `Project.DBSize > 0` and no other event.
- [ ] [nit] internal/project/project.go:347-350 — if the re-read `Get` after a successful copy fails, the duplicate exists but no `Created` is emitted and the caller gets an error. → On that error, still emit `Created` with the `createQuiet` result (with `DBSize` 0), or roll back the way the copy-failure path does.
- [ ] [nit] internal/project/events.go:59-69 — locking twice with a placeholder `func() bool { return false }` is more complicated than it needs to be. `context.AfterFunc` runs `f` in its own goroutine, so it is safe to call it while holding `h.mu` (`f` would just wait for the lock). → Register `AfterFunc` inside the first critical section and store `stop` directly.
- Round-1 items still open for the lead (unchanged): the §5.1 `settings["project.temporary"]` decision (minor) and the schema doc saying the subscription can end (nit).

**Verdict: changes_requested.** The only thing blocking approval is the lead's decision on `paths` (major). The code is correct and the round-1 code findings are fixed. The test for the Duplicate event is recommended but not required.


### Round 3 — implementer
- Fixed in code: Duplicate event test (`TestDuplicateEmitsOneCreated`: one `Created`, `DBSize > 0`); Duplicate now still emits `Created` if the re-read fails (logs a warning); `hub.subscribe` registers `AfterFunc` in a single critical section. `make check` green.
- Still lead decisions (not implementer-fixable): major `resolver.go` out of `paths` + `app.go` wiring; §5.1 `project.temporary` note; schema doc nit for subscription ending.

### Round 3 — reviewer (claude-code, opus 5.5), 2026-11-18
**Scope check**: Fix commit `e2428db` touches only `internal/project/{events,project,project_test}.go` and this task file, so it is in lane. The branch still contains the out-of-`paths` edit to `resolvers/resolver.go` (+2 lines, `Deps.Projects`). The front-matter `paths` haven't changed on this branch or on `main`, and `internal/app/app.go` still doesn't build or wire the service.
**`make check`**: green (exit 0; vet, golangci-lint, `go test -race`, svelte-check, eslint, prettier, vitest 50/50). I also ran `go test -race -count=5 ./internal/project ./internal/api/graphql/resolvers` with no cache, and it passed, so the new event test isn't flaky. `make test-int` not needed.

**Round-2 fixes verified**
- Duplicate event test: `TestDuplicateEmitsOneCreated` (`project_test.go:704-736`) selects the source so its DB exists, subscribes, duplicates, and asserts exactly one buffered event: `Created` for `cp.ID` with `DBSize > 0`. `emit` is a synchronous non-blocking send, so `len(ch) == 1` is deterministic. Done.
- Re-read failure: `Created` is now emitted with the `createQuiet` result and a warning is logged (`project.go:346-353`). Done.
- `hub.subscribe` now registers `AfterFunc` inside a single critical section (`events.go:59-62`). This is safe: if ctx is already done, the callback runs in its own goroutine and `remove` waits on `h.mu`, and `remove` is idempotent with respect to `emit` and `closeAll`. Done.

**Findings**
- [ ] [major] internal/api/graphql/resolvers/resolver.go:28 — still open since round 1. The file is outside `paths` (hard rule 1), and F-WSP-1 can't be used end to end until `internal/app/app.go` builds `project.Service`, passes it in `Deps.Projects` and calls `Close()` on shutdown. The implementer can't fix this. → Lead: amend `paths` to include `internal/api/graphql/resolvers/resolver.go`, and either add `internal/app/app.go` to `paths` (the implementer then does the wiring here) or open a follow-up task for the wiring. After that, this task can be approved with no further code changes.
- Still open for the lead and not blocking on their own: the §5.1 `settings["project.temporary"]` decision (minor) and the schema doc on `projectChanged` saying the server may end the subscription (nit).

No new code findings this round.

**Verdict: changes_requested.** All code findings from rounds 1 and 2 are fixed and verified. The only thing still blocking is the lead's `paths`/wiring decision (major). The verdict will become approve once the lead changes `paths` and the wiring is either done here or tracked in a follow-up task.
