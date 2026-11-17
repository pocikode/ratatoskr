---
id: T-005
title: SQLite store - open, WAL, migrations, write serializer, schema v1
milestone: M0
role: backend
status: changes_requested
depends_on: [T-002]
owner: "claude-code (sonnet-5.5)"
branch: task/T-005-store
paths:
  - internal/store/**
  - go.mod
  - go.sum
spec:
  - docs/03-architecture.md §5
  - docs/adr/0002-sqlite-storage.md
---

## Goal
`internal/store` opens instance and project databases with the required pragmas, runs embedded forward-only migrations, provides a
serialized writer + read pool, transaction helpers, and the v1 schemas (tables from docs/03 §5, created even if unused yet).

## Acceptance criteria
- [x] `store.OpenInstance(ctx, root *os.Root)` / `store.OpenProject(ctx, root)` return handles; pragmas verified by test.
- [x] Migration runner is idempotent, records version, refuses to open a DB with a newer schema (clear error).
- [x] Concurrent writers (100 goroutines × 1k inserts) complete without `SQLITE_BUSY` errors (`-race`).
- [x] Batch insert helper for requests achieves ≥ 20k rows/s on CI hardware (benchmark).
- [x] SQLite user functions registered: `regexp(pattern, text)`, `bcontains_fold(blob, needle)` with tests.

## Plan (implementer, ≤15 lines)
1. Add `modernc.org/sqlite` (approved list). Register `regexp` and `bcontains_fold` once via `sync.OnceValue` + `RegisterScalarFunction`.
2. `DB` type: write handle (`MaxOpenConns(1)`, `_txlock=immediate`) + read pool (`query_only`), pragmas via DSN `_pragma`.
3. `OpenInstance/OpenProject(ctx, *os.Root)` → files `instance.db` / `project.db` inside root.
4. Embedded forward-only migrations (`migrations/instance/*.sql`, `migrations/project/*.sql`), `schema_version` table, refuse newer DBs (`ErrNewerSchema`).
5. `DB.Write(ctx, fn(tx))` / `DB.Read` helpers; writes serialized by the single conn (no SQLITE_BUSY).
6. `BatchInsertRequests` helper (prepared stmt inside one tx) + benchmark.
7. v1 schemas for all tables in docs/03 §5.
8. Tests: pragmas, migrations idempotent/newer, 100x1k concurrent writers, funcs, bench.

## Out-of-scope change needed (if any)

## Handoff (implementer)
**Changed:** new `internal/store` (store.go, migrate.go, funcs.go, requests.go, embedded `migrations/{instance,project}/0001_init.sql`), `modernc.org/sqlite` added.
- `OpenInstance/OpenProject(ctx, *os.Root)` -> `*DB` with a single serialized writer (`_txlock=immediate`, 1 conn) and a read pool (`query_only`, <=8 conns). `DB.Write` / `DB.View` tx helpers, `DB.Reader()`.
- Migrations: `schema_version` table, forward-only, contiguous; newer DB -> `*NewerSchemaError` (`errors.Is(err, ErrNewerSchema)`).
- UDFs `regexp` (RE2, bounded compile cache) and `bcontains_fold` (ASCII fold, no alloc), registered once.
- `DB.InsertRequests` batch helper: ~149k rows/s locally (no race).
**Verify:** `go test -race ./internal/store/`, `go test -bench . -run x ./internal/store/`, `make check`.
**Limitations:** schema columns for tables only named in docs/03 §5 (presets, replay, automate, mr_*, exports, assistant) are my best-fit v1 guesses; later tasks add migrations. Concurrent-writer test is skipped under `-short`. `make check` needed `bun install` in web/ (untracked node_modules).
**Follow-ups:** batching goroutine (50 ms / 200 rows flush) belongs to the capture pipeline task, built on `InsertRequests`.

## Review (reviewer)
**Reviewer:** claude-code (opus-5.5) · 2026-11-18 · commits `d1fb847..9954507`

**Scope:** every changed file is inside `paths` (internal/store/**, go.mod, go.sum) plus this task file. `modernc.org/sqlite` is on the approved list (docs/05). No hard-rule violations found.

**Checks:** `make check` green (go fix, vet, golangci-lint 0 issues, `go test -race`, web check, vitest 37/37). `make test-int` green.
`go test -race -count=1` (uncached): TestConcurrentWriters passed in 6.7 s. `BenchmarkInsertRequests`: about 97k rows/s on a Ryzen 7 8845HS, well above the 20k AC.

**AC coverage:** all 5 ACs are covered by tests: pragmas on writer and reader, idempotent migrations, `ErrNewerSchema` checked with both `errors.Is` and `errors.AsType`, 100×1k writers, the benchmark, and UDF table tests plus a fuzz target.

**Findings**
- [x] [major] internal/store/migrations/project/0001_init.sql:15,62,140,164,173 — `foreign_keys=ON` makes SQLite check every child table on each `DELETE FROM requests`. Five child columns have no index: `requests.edited_from` (self-reference), `ws_streams.request_id`, `replay_entries.request_id`, `automate_results.request_id` and `findings.request_id`. Each deleted request therefore causes a full scan of those tables, including `requests` itself. A bulk delete or history clear on a large project becomes O(N·M), e.g. deleting 100k of 1M rows means about 10^11 row visits. → Add `CREATE INDEX … ON requests(edited_from)`, `ws_streams(request_id)`, `replay_entries(request_id)`, `automate_results(request_id)` and `findings(request_id)` to 0001 now, while it is unmerged and still free to edit. Also index the other unindexed FK children: `mr_rules(collection_id)`, `replay_sessions(collection_id)`, `automate_attacks(session_id)`. Add a test that runs `PRAGMA foreign_key_list` for each table and asserts every FK child column is the leftmost column of some index (`PRAGMA index_list`/`index_info`), so future migrations can't regress this.
- [x] [minor] internal/store/migrate.go:85-95 — the current version is read outside the transaction that applies migrations. If two processes open the same new DB at once, both read 0. The second one then fails with `table requests already exists` instead of seeing the migration as done. → Re-read `MAX(version)` inside `applyMigration` after `BEGIN IMMEDIATE` (the writer DSN already uses `_txlock=immediate`), and skip the migration if `version >= m.version`.
- [x] [minor] internal/store/store.go:99 — `url.URL{Scheme:"file", Path: path}` builds a broken URI for Windows paths. For `C:\x\project.db` it produces `file:./C:%5Cx%5Cproject.db`. docs/02 lists Win/Linux keybindings, so Windows is plausibly a target. → Use `filepath.ToSlash(path)` and prefix `/` when `filepath.VolumeName(path) != ""`, giving `file:///C:/x/project.db`. Add a unit test for `dsn` with a Windows-style path, guarded by `runtime.GOOS` or done as pure string construction.
- [x] [minor] internal/store/store.go:56 — after the `os.Root` check, the driver reopens the file by `filepath.Join(root.Name(), file)`. That has two consequences. First, if `root.Name()` is relative, the DB breaks once the process changes its working directory. Second, the containment guarantee is only check-then-use: a symlink swapped in after the check is followed. → Store `filepath.Abs(root.Name())` at open. Add a doc comment on `open` stating that SQLite opens the main, -wal and -shm files by path, so `os.Root` only guards creation.
- [x] [minor] internal/store/store.go:125 — `DB.View` (the `ReadOnly` read-pool transaction) has no test. → Add a test asserting that `View` sees committed rows and that an `INSERT` inside `View` fails (`query_only`).
- [x] [minor] internal/store/funcs.go:46,58 — `asBytes` copies every TEXT value (`[]byte(x)`) on each `regexp()` call. HTTPQL runs this per row and has a budget of <300 ms per 100k rows (docs/04). → In `sqlRegexp`, branch on type and call `re.MatchString(x)` for `string` and `re.Match(x)` for `[]byte`, with no copy.
- [x] [minor] internal/store/funcs.go:71-88 — `containsFold` is a naive O(n·m) byte loop, which is slow on MB-sized bodies. → Jump to candidate positions with `bytes.IndexByte` on the lower- and upper-case forms of `needle[0]` (or use `bytes.IndexFunc`), then verify. The existing fuzz target keeps it correct.
- [x] [nit] internal/store/store.go:120 — the named result `(err error)` on `Write` is unused. → Drop it. Also document that calling `d.Write` from inside `fn` deadlocks, because there is only one writer connection.
- [x] [nit] internal/store/store_test.go:12 — `"os"` sits in its own import group. → Merge it into the stdlib group (goimports).
- [x] [nit] internal/store/store_test.go:150-154 — the `testing.Short()` skip runs after `openProject`. → Move the skip to the top of the test.
- [x] [nit] internal/store/store_test.go:292 — `b.ResetTimer()` is redundant, because `b.Loop()` resets the timer on its first call. → Remove it.
- [x] [nit] internal/store/migrations/project/0001_init.sql:36,79 — `responses.edited_from` and `ws_messages.edited_from` have no FK, unlike `requests.edited_from`. → Add `REFERENCES requests(id)` / `ws_messages(id) ON DELETE SET NULL`, or add a comment explaining why they are left loose.

**Verdict:** `changes_requested`. Fix the major item. The minors are strongly recommended while 0001 is still editable.

**Author reply:** all findings addressed (FK indexes + FK-index test, in-tx migration re-check, Windows URI helper, abs root path, View test, no-copy regexp, IndexByte containsFold, nits). `responses/ws_messages.edited_from` left loose with an SQL comment.

### Review round 2
**Reviewer:** claude-code (opus-5.5) · 2026-11-18 · commit `147b219`

**Scope/checks:** all changes are inside `paths`. `make check` is green (golangci-lint reports 0 issues, `go test -race` passes, vitest 37/37), and `make test-int` is green (store 8.2 s).

**Round-1 findings:** all are resolved and verified. 0001 now indexes every FK child column. `TestForeignKeysIndexed` covers both schemas and correctly treats a sole INTEGER PRIMARY KEY as indexed. The migration version is re-checked under BEGIN IMMEDIATE. `uriPath` has a unit test, the root path is now absolute and its limits are documented, `TestView` covers `View`, and `regexp()` no longer copies TEXT values. The comment on the loose `edited_from` columns is acceptable.

**New findings**
- [ ] [major] internal/store/funcs.go:84-89 — the new `containsFold` scan is quadratic. When `lo != up`, every iteration runs `bytes.IndexByte(hay[i:…], up)` over the whole remaining haystack. When the other case of `needle[0]` is rare or absent, which is the common case in lowercase text, each candidate occurrence triggers a full rescan. Measured on a Ryzen 7 8845HS by copying the function into a scratch module:
  - 1 MB of English text with needle `"ERROR"`: 387 ms per call. The previous naive loop took 1.26 ms, and `bytes.Contains(bytes.ToLower…)` takes 1.19 ms.
  - Bodies made only of `e` with needle `"ex"`: 0.35 s, 1.4 s and 5.7 s for 256 KB, 512 KB and 1 MB, so 4× the time per doubling.
  
  `bcontains_fold` runs per row on response bodies that the target controls. One crafted body can therefore stall the HTTPQL budget of <300 ms per 100k rows (docs/04), and it ties up a reader connection. This is a regression against round 1.
  → Cache the next-occurrence index of each case: rescan for `lo` only when `i` passes `nextLo`, and for `up` only when it passes `nextUp`, so each byte is scanned at most once per case. The simplest alternative is to drop the second `IndexByte` and scan with one loop that checks `c == lo || c == up` before verifying. Add a regression test that bounds the worst case, e.g. a 1 MB haystack of `e` and needle `ex` must finish in under ~50 ms, or that checks scaling against a 2× larger input. Also add a `BenchmarkContainsFold` with a lowercase-heavy haystack.
- [ ] [nit] internal/store/store.go:111 — `uriPath` mishandles UNC paths: `\\srv\share\p.db` becomes `///srv/share/p.db`. → Either leave UNC unsupported and say so in the doc comment, or emit `file://srv/share/p.db`.
- [ ] [nit] internal/store/store_test.go:357 — `rows.Close()` runs only after the loop, so a `t.Fatal` inside the loop leaves rows open. → Use `defer rows.Close()` right after `QueryContext`.

**Verdict:** `changes_requested`. Fix the `containsFold` regression and add its test. Everything else is ready.
