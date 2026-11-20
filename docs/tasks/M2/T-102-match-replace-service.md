---
id: T-102
title: Match & Replace service - persistence, ordering, import/export, tester, resolvers
milestone: M2
role: backend
status: todo
depends_on: [T-035, T-099]
owner: ""
branch: task/T-102-match-replace-service
paths:
  - internal/matchreplace/**
  - internal/store/migrations/project/0005_matchreplace.sql
  - internal/api/graphql/resolvers/matchreplace*.go
  - internal/api/graphql/resolvers/stubs_matchreplace.go
spec:
  - docs/01-feature-spec.md#F-MR-1 (incl. M2 contract)
  - docs/03-architecture.md §5.2 (`mr_collections`, `mr_rules`)
---

## Goal
Rules and collections persist per project, the engine always runs the current enabled list, and the T-099 contract is served.

## Scope
- In: `matchreplace.Service` over `mr_collections` / `mr_rules` (`definition_json` holds condition, sources, section,
  operation, matcher, term; migration 0005 only if a column is really missing): collection CRUD + reorder, rule CRUD,
  enable, move, delete; limits; export/import (all or nothing, appends); `Test` via the engine (the engine does every tester check and returns the `apperr` error; pass it through); after every change and on
  project switch the service pushes the compiled enabled list into the `Engine` (`Reload(ctx)`); resolvers; delete
  `stubs_matchreplace.go`.
- Out: proxy/replay wiring (T-105/T-106/T-109), UI.

## Acceptance criteria
- [ ] CRUD, enable, move (index clamped, across collections), reorder (exact set else `BAD_INPUT`); deleting a collection
      deletes its rules.
- [ ] Limits and errors as in T-099 (names, ≤ 1000 rules, term size, invalid rule, `BAD_QUERY` with span for the condition).
- [ ] Export → import into an empty project reproduces collections and rules (round-trip test); a file with one invalid
      rule adds nothing; wrong `version` → `BAD_INPUT`.
- [ ] After `updateMatchReplaceRule`/`setMatchReplaceRuleEnabled` the engine's next `Request` call uses the new list (test);
      `Reload` after a project switch loads that project's rules.
- [ ] Presets (F-MR-1 "Presets", lead 2026-11-20): the service passes the project's filter presets to the engine's
      `Set` and `Test`, and exposes `Reload(ctx)` (re-reads rules and presets; test: after deleting a preset a rule uses,
      `Reload` skips that rule without failing). Calling it on preset changes is T-109's job.
- [ ] `testMatchReplace` resolver returns output/conditionMatched/changed for a draft rule without saving it.
- [ ] `testMatchReplace` rejects a request with bytes after the head and neither `Content-Length` nor
      `Transfer-Encoding` (`BAD_INPUT`, rawhttp `FramingNone`); a header-only request with a `FULL` body rule still gains a body (test).
      Framing comes from `internal/rawhttp` itself, not a second parser: an HTTP/1.0 request with `Transfer-Encoding:
      chunked`, no `Content-Length` and trailing bytes is `BAD_INPUT` (test).
- [ ] Tester output over 16 MiB is `BAD_INPUT`, detected while producing replacement bytes (before allocating or
      base64-encoding the full result); test with a small input whose replacement crosses the cap (F-MR-1 size budget).
- [ ] `make check` green.

## Notes from lead
- Ordering is `(collection.sort_order, rule.sort_order, id)`; keep it in one SQL `ORDER BY` so the engine list is deterministic.

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
