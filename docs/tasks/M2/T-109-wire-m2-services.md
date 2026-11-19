---
id: T-109
title: Wire M2 services into the app (intercept, M&R, replay, environment)
milestone: M2
role: backend
status: todo
depends_on: [T-105, T-106, T-102]
owner: ""
branch: task/T-109-wire-m2-services
paths:
  - internal/app/**
  - internal/api/graphql/resolvers/resolver.go
spec:
  - docs/03-architecture.md §2 (no globals, wiring in internal/app), §4
  - docs/01-feature-spec.md#F-INT-1 (project switch), #F-MR-1 (reload on switch)
---

## Goal
The running binary uses everything M2 built: the proxy holds and rewrites, Replay sends with environments and M&R, and no
M2 GraphQL field answers `NOT_IMPLEMENTED`.

## Scope
- In: construct `intercept.Service`, `matchreplace.Service`/`Engine`, `replay.Service`, `environment.Service` in
  `app.New`; pass them in `resolvers.Deps` and `proxy.Deps` (`Interceptor`, `Rewriter`) and into Replay (env vars,
  rewriter); on project switch (`project.Service.Subscribe`) release the intercept queue and reload its settings, reload
  M&R rules; compile-time assertions (`var _ proxy.Rewriter = (*matchreplace.Engine)(nil)` and the intercept equivalent).
- Out: new behaviour; anything not already implemented by the listed tasks.

## Acceptance criteria
- [ ] App-level test: with queuing on (via GraphQL), a request through the proxy is listed by `interceptQueue`, forwarded
      with `forwardInterceptItem`, and reaches the test server.
- [ ] App-level test: an enabled `REQUEST_HEADER / ADD` rule adds the header to a proxied request and history shows the pair.
- [ ] App-level test: `sendReplaySession` with `{{host}}` from the selected environment reaches the test server.
- [ ] Project switch with a held request releases it (the client gets the server's answer) and loads the new project's rules.
- [ ] A test walks every M2 root field (intercept, replay, matchReplace, environment) and asserts none returns `NOT_IMPLEMENTED`.
- [ ] `make check` and `make test-int` green.

## Notes from lead
- This is the only M2 task that edits `internal/app`; parallel backend tasks stay testable through their own packages and
  resolver tests with hand-built `resolvers.Deps`.

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
