---
id: T-005
title: SQLite store - open, WAL, migrations, write serializer, schema v1
milestone: M0
role: backend
status: in_review
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
