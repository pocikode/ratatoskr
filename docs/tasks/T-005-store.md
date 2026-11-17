---
id: T-005
title: SQLite store - open, WAL, migrations, write serializer, schema v1
milestone: M0
role: backend
status: in_progress
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
- [ ] `store.OpenInstance(ctx, root *os.Root)` / `store.OpenProject(ctx, root)` return handles; pragmas verified by test.
- [ ] Migration runner is idempotent, records version, refuses to open a DB with a newer schema (clear error).
- [ ] Concurrent writers (100 goroutines × 1k inserts) complete without `SQLITE_BUSY` errors (`-race`).
- [ ] Batch insert helper for requests achieves ≥ 20k rows/s on CI hardware (benchmark).
- [ ] SQLite user functions registered: `regexp(pattern, text)`, `bcontains_fold(blob, needle)` with tests.

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

## Review (reviewer)
