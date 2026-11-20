---
id: T-110
title: Contract - ReplaySession.requestTarget for the collections tree
milestone: M2
role: lead
status: todo
depends_on: [T-098]
owner: ""
branch: task/T-110-contract-replay-request-target
paths:
  - internal/api/graphql/schema/replay.graphqls
  - internal/api/graphql/resolvers/stubs_replay.go
  - internal/api/graphql/generated/**
  - web/src/lib/api/generated/**
  - web/src/lib/api/mocks/replay.ts
  - web/src/lib/api/mocks/replay.test.ts
  - docs/tasks/M2/T-110-contract-replay-request-target.md
spec:
  - docs/01-feature-spec.md#F-REP-1 (M2 contract, T-034 escalation bullet)
---

## Goal
The Replay tree can search by path without downloading every draft (T-034 review: ten 5 MiB drafts ≈ 67 MiB per refresh).

## Scope
- In: field `requestTarget: String!` on `ReplaySession` — the request target of the draft's first line
  (`METHOD target HTTP/1.x`), decoded as UTF-8 with invalid bytes replaced, cut to 2048 characters; empty string when the
  draft does not start with an HTTP/1.x request line. Stub resolver returns `ErrNotImplemented`; mock computes it from
  the mock draft; `make gen`.
- Out: backend implementation (T-033), switching the UI query (T-111).

## Acceptance criteria
- [ ] Schema field documented with the rule above; `make gen` clean.
- [ ] Mock returns `/a?b=1` for `GET /a?b=1 HTTP/1.1`, `""` for non-request bytes (mock test).
- [ ] `make check` green.

## Notes from lead
- Contract tasks run in sequence (shared generated files): land before T-033 starts.

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
