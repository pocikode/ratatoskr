---
id: T-006
title: Projects service and GraphQL contract
milestone: M0
role: backend
status: in_progress
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
- [ ] All F-WSP-1 operations via GraphQL with tests (httptest + gqlgen client).
- [ ] Deleting the selected project falls back to the default or a new temp project.
- [ ] Switching closes the old DB handle (no open handles remain on the old DB file — test).
- [ ] Temp project removed on clean shutdown unless renamed/saved.

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

## Review (reviewer)
