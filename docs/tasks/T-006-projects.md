---
id: T-006
title: Projects service and GraphQL contract
milestone: M0
role: backend
status: in_review
depends_on: [T-005, T-007]
owner: "claude-code (sonnet 5.5)"
branch: task/T-006-projects
paths:
  - internal/project/**
  - internal/api/graphql/resolvers/project*.go
  # added by lead 2026-11-18 (review round 1 major, round 1 minor):
  - internal/api/graphql/resolvers/resolver.go
  - internal/app/app.go
  - internal/app/app_test.go
  - internal/store/migrations/instance/0002_*.sql
  # added by lead 2026-11-18 (review round 7 major): one-line call site `app.New(ctx, …)`
  - cmd/ratatoskr/main.go
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

### Lead decisions (2026-11-18, answers the round 1–4 questions)
1. **`paths`**: `resolvers/resolver.go` is added retroactively (the `Deps.Projects` field is accepted as is). `internal/app/app.go` (+ `app_test.go`) is added: **do the wiring in this task** — `project.Open` after `store.OpenInstance`, pass it as `resolvers.Deps.Projects`, `Close()` it on shutdown (before the instance DB closes). Add an app-level test that `projects` answers without NOT_IMPLEMENTED. T-011 and T-027 also own `internal/app/**`; they are `todo` and rebase on T-006.
2. **Temporary flag**: not a settings key. Add migration `internal/store/migrations/instance/0002_projects.sql` that rebuilds `projects` with `id INTEGER PRIMARY KEY AUTOINCREMENT` and `is_temporary INTEGER NOT NULL DEFAULT 0` (create new table, copy rows, drop old, rename; STRICT). AUTOINCREMENT because project IDs are GraphQL node IDs and directory names and must never be reused (fixes the Handoff limitation). Drop `tempKey`/`project.temporary` handling; crash-recovery purge uses `WHERE is_temporary = 1`. docs/03 §5.1 updated.
3. **Subscription end**: `projectChanged` schema doc now says the server may end it and clients must resubscribe + refetch (`make gen` run by lead).
4. **`cmd/ratatoskr/main.go`** (2026-11-18, round 7): added to `paths`. Reviewer verified that the in-lane alternative fails (contextcheck reports at the call site in `main.go`). `app.New(ctx, opts)` using `context.WithoutCancel` is the accepted signature. No other active task owns `cmd/ratatoskr/**`. T-011 and T-027 are notified of the new signature.

## Plan (implementer, ≤15 lines)
- `internal/project.Service` owns the registry in `instance.db` (`projects` table) and the one open `project.db` (`projects/<id>/` via `os.Root`).
- ~~Temporary flag kept in `settings` key `project.temporary`~~ (superseded in round 5: `projects.is_temporary` column, migration 0002). Leftover temp projects and orphan dirs are purged on `Open` (crash recovery); `Close` removes temp projects (clean shutdown).
- Ops: List/Get/Current/Create/Rename (saves temp)/Duplicate (VACUUM INTO + copy other files)/Delete (falls back to default or new temp)/SetDefault/Select (open new, swap, close old, SELECTED event).
- Single mutex serialises mutations; events fan out to bounded per-subscriber channels (cap 64); an overflowing subscriber is closed so it resyncs.
- Errors are `apperr` (NOT_FOUND, BAD_INPUT); names trimmed, non-empty, ≤200 runes.
- Resolvers (`resolvers/project.go`) depend on a small `ProjectService` interface; nil service keeps returning NOT_IMPLEMENTED (existing T-007 test relies on it).
- Tests: service unit tests (synctest not needed; no time), old-handle-closed test, temp cleanup, GraphQL httptest round trips + subscription over the handler.

## Out-of-scope change needed (if any)
- `internal/api/graphql/resolvers/resolver.go`: add `Deps.Projects ProjectService` (one field; unavoidable for the resolvers to reach the service).
- `internal/app/app.go`: construct `project.Service` and pass it in `resolvers.Deps` + `Close` on shutdown. **Resolved (round 5)**: done here; `app.go` added to `paths`.
- `cmd/ratatoskr/main.go`: one-line call-site change `app.New(ctx, …)` (contextcheck). **Resolved (round 7)**: added to `paths` by the lead.

## Handoff (implementer)
**What changed**
- `internal/project`: `Service` (Open/Close, List/Get/Current, Create, Rename, Duplicate, Delete, SetDefault, Select, `DB()`, `Subscribe`). Project data lives in `projects/<id>/` (via `os.Root`); the registry is the `projects` table in instance.db.
- Temporary flag is the `projects.is_temporary` column (migration 0002, round 5). `Open` purges leftover temp projects and orphan dirs; `Close` deletes temp projects.
- Duplicate snapshots project.db with `VACUUM INTO` over its own read-only connection (store's reader is query_only, its writer is transactional) and copies other files.
- `resolvers/project.go`: real resolvers over a `ProjectService` interface; `Deps.Projects` nil keeps returning NOT_IMPLEMENTED.
- Events: bounded channel (64) per subscriber; a subscriber that falls behind is closed (GraphQL subscription ends, client resubscribes + refetches).

**Verify**: `go test -race ./internal/project ./internal/api/graphql/resolvers`; `make check` green.

**Limitations / follow-ups**
- Resolved in round 5: app wiring done; IDs never reused (AUTOINCREMENT). `resolver.go` is now in `paths`.
- Follow-up (store-owned): test upgrading a v1 instance.db with rows through migration 0002.
- The subscription is tested at resolver level, not over a real WebSocket.

### Round 5 — implementer (lead decisions applied)
- Migration `0002_projects.sql`: `projects` rebuilt with AUTOINCREMENT + `is_temporary`; `project.temporary` settings handling removed (`loadTemp`/`saveTemp` gone; purge uses `WHERE is_temporary = 1`; Rename clears the flag). Test `TestIDsAreNeverReused`.
- `internal/app/app.go`: `New` opens instance.db + `project.Service`, passes it as `Deps.Projects`; `Close` closes project, instance db, data dir (errors joined). Tests `TestProjectsAreWired`, `TestCloseRemovesTemporaryProject`.
- **Out-of-paths, flagged**: `New` now takes `ctx` (`New(ctx, opts)`) because contextcheck rejects opening DBs without one; one-line call-site change in `cmd/ratatoskr/main.go`. **Round 6**: I tried reverting to `New(opts)` + nolint, but contextcheck then flags the caller in `main.go` itself, which no in-lane nolint can silence. So `main.go` must be added to `paths` by the lead. New uses `context.WithoutCancel(ctx)` so an already-cancelled ctx still shuts down cleanly (main tests rely on it). `project.Service` keeps Open's ctx (WithoutCancel) for `Close`.
- Handoff limitation "IDs reused" and "app.go not wired" are now resolved. `make check` green.

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


### Round 4 — implementer
No unchecked finding is fixable inside this task's `paths`. Nothing changed in code.
- **Blocked on lead** (major, `resolvers/resolver.go:28`): add `internal/api/graphql/resolvers/resolver.go` to `paths`, and either add `internal/app/app.go` to `paths` (I do the wiring here) or open a follow-up wiring task.
- **Lead** (minor): record `settings["project.temporary"]` in docs/03 §5.1, or schedule a migration adding `projects.is_temporary`.
- **Lead** (nit): `project.graphqls` `projectChanged` doc should say the server may end the subscription and clients must resubscribe and refetch `projects`.
Set `status: changes_requested` (or `in_review`) once the lead amends `paths`.

### Round 5 — reviewer (claude-code, opus 5.5), 2026-11-18
**Scope check**: The lead commit `0c75bf6` (docs/03 §5.1, `project.graphqls` doc, generated `exec.go` via `make gen`, T-011/T-027 notes, `paths` amendment) is lead-owned and fine. Implementer commit `5dacb5a` touches `internal/project/**`, `internal/app/app.go`, `internal/app/app_test.go`, `internal/store/migrations/instance/0002_projects.sql` and this file, which are all inside the amended `paths`. It also touches **`cmd/ratatoskr/main.go`** (1 line, `app.New(ctx, …)`), which is **not** in `paths`.
**`make check`**: green (exit 0; vet, golangci-lint, `go test -race`, svelte-check, eslint, prettier, vitest 50/50). **`make test-int`**: green (exit 0; a store migration changed). I also ran `go test -race -count=3 ./internal/{project,app,store} ./internal/api/graphql/resolvers ./cmd/...` uncached, and it passed.

**Lead decisions verified**
- Migration `0002_projects.sql` rebuilds `projects` as STRICT with `AUTOINCREMENT` + `is_temporary`, copies rows, drops and renames. No FK or index referenced the old table (`0001_init.sql:6-13`), so nothing is lost. Inserting explicit ids into an AUTOINCREMENT table seeds `sqlite_sequence` with the max id, so ids from before the upgrade can't be reused either. It runs inside the migration's `BEGIN IMMEDIATE` transaction.
- `project.temporary` settings handling is gone (`loadTemp`/`saveTemp` removed; no references remain). `Rename` clears `is_temporary`, `SetDefault` reads `r.temp`, and `tempIDs` queries `WHERE is_temporary = 1`. `TestIDsAreNeverReused` actually tells the two behaviours apart: without AUTOINCREMENT, the new id would equal the deleted one.
- App wiring: `openStores` runs `store.OpenInstance` and then `project.Open`, and `Deps.Projects` is passed (`app.go:272`). `Close` closes the project service, then instance.db, then the data dir, joining the errors. A partial failure in `New` runs `Close`. `TestProjectsAreWired` (no NOT_IMPLEMENTED, a temp project is listed) and `TestCloseRemovesTemporaryProject` cover the lead's request.
- The schema doc for subscription end and the §5.1 docs are done by the lead. The round 1 minor and nit for the lead are now resolved.

**Findings**
- [ ] [major] cmd/ratatoskr/main.go:82 — this file is outside `paths` (hard rule 1). The Handoff says "approved by operator", but neither the front-matter `paths` nor the lead decisions record that approval, and an in-lane alternative exists. → Do one of these: (a) lead/operator adds `cmd/ratatoskr/main.go` to `paths` with a dated note, the same way `resolver.go` was handled, and then this can be approved with no code change; or (b) implementer reverts `main.go`, keeps `New(opts)` and opens the stores with `context.Background()` plus `//nolint:contextcheck // T-006: opening the DBs must not be cancelled by the caller`.
- [ ] [nit] internal/app/app.go:74-76 — the doc comment puts a long parenthetical into the first sentence (one line of about 190 chars), and it's hard to read. → `// New prepares an instance: it rejects options that are not implemented yet, creates the data dir (0700), opens instance.db and the project service, and builds the UI/API handler.` and then a second paragraph: `// ctx only carries values; a cancelled ctx does not abort opening the databases.`
- [ ] [nit] internal/store/migrations/instance/0002_projects.sql — no test upgrades a v1 instance.db that has rows (rows kept, `is_temporary = 0`, next id > max old id). No release has shipped, so the risk is low. → Add the test in a store-owned follow-up (`internal/store` is outside this task's `paths`), or note it as a follow-up in the Handoff.
- [ ] [nit] docs/tasks/T-006-projects.md:43,57,65-66 — Plan/Handoff still say the temporary flag is in `settings["project.temporary"]`, that `app.go` is not wired, and that IDs are reused. All three are now false, and only Round 5 says otherwise. → Edit the Handoff "What changed" and "Limitations" bullets so they match the current state (strike through or remove the resolved items).

**Verdict: changes_requested.** The code is correct and every AC is covered and green, including the integration tests. The only blocker is the out-of-`paths` edit to `cmd/ratatoskr/main.go`. It needs a recorded `paths` amendment (then approve without another code round) or a switch to the in-lane alternative.

### Round 6 — implementer
- [nit] app.go doc comment split; [nit] Handoff text corrected; migration upgrade test noted as store follow-up.
- [major] `cmd/ratatoskr/main.go`: option (b) is not feasible (contextcheck reports at the `app.New` call in main.go, nolint in app.go is "unused"). **Lead: add `cmd/ratatoskr/main.go` to `paths`** (one-line change, `app.New(ctx, …)`). No code change otherwise.

### Round 7 — reviewer (claude-code, opus 5.5), 2026-11-18
**Scope check**: Round 6 commit `db7b26b` changes only a doc comment in `internal/app/app.go` (in `paths`) and this file. The branch still has the 1-line `cmd/ratatoskr/main.go:82` edit (`app.New(ctx, …)`). The front-matter `paths` and the lead decisions still don't include it.
**`make check`**: green (exit 0; vet, golangci-lint, `go test -race`, svelte-check, eslint, prettier, vitest 50/50). **`make test-int`**: green (exit 0).

**Round-5 fixes verified**
- `app.go` doc comment: split into a summary and a ctx paragraph (`app.go:74-78`). Done.
- Handoff "What changed"/"Limitations": now match the current state (is_temporary column, wiring done, AUTOINCREMENT). The migration upgrade test is recorded as a store-owned follow-up. Done.
- I checked the implementer's claim that option (b) can't work in a throwaway worktree. With `New(opts)`, `openStores(context.Background())` and `main.go` reverted, golangci-lint reports `cmd/ratatoskr/main.go:82:26: Function \`New\` should pass the context parameter (contextcheck)`. The diagnostic lands in `main.go`, so no in-lane `nolint` can suppress it. Option (b) is ruled out, and `main.go` has to change. `New(ctx, …)` with `context.WithoutCancel` is the right design.

**Findings**
- [x] [major] cmd/ratatoskr/main.go:82 — this file is still outside `paths` (hard rule 1). The edit can't be avoided (verified above) and is as small as it can be, but only the lead can clear it. → Lead/operator: add `cmd/ratatoskr/main.go` to the front-matter `paths` with a dated note, the same way `resolver.go` was handled. No code change is needed after that, and this task can be approved without another code round.
- [x] [nit] docs/tasks/T-006-projects.md:43,52 — two lines are stale. Plan line 43 still describes the `settings["project.temporary"]` key. "Out-of-scope change needed" still says the `app.go` wiring is "left to a follow-up; not done here", and it doesn't list the `cmd/ratatoskr/main.go` request, which so far appears only in the Round 5/6 notes. → Mark line 43 as superseded (round 5), mark the `app.go` bullet as resolved, and add a `cmd/ratatoskr/main.go` bullet under "Out-of-scope change needed" so the lead can find the request.

**Verdict: changes_requested.** The code is correct, every AC is covered, and both make targets are green. The only open blocker is the lead's `paths` amendment for `cmd/ratatoskr/main.go`. After the amendment, approve with no code change.


### Round 8 — implementer
- [major] main.go: resolved by lead (`paths` amended, Lead decision 4). No code change.
- [nit] Task file: Plan line is struck through as superseded; "Out-of-scope change needed" marks `app.go` resolved and lists `cmd/ratatoskr/main.go`. `make check` green.
