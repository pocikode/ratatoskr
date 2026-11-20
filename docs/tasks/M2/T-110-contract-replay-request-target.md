---
id: T-110
title: Contract - ReplaySession.requestTarget for the collections tree
milestone: M2
role: lead
status: in_review
depends_on: [T-098]
owner: claude-sonnet-5-5
branch: task/T-110-contract-replay-request-target
paths:
  - internal/api/graphql/schema/replay.graphqls
  - internal/api/graphql/resolvers/stubs_replay.go
  - gqlgen.yml
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
- [x] Schema field documented with the rule above; `make gen` clean.
- [x] Mock returns `/a?b=1` for `GET /a?b=1 HTTP/1.1`, `""` for non-request bytes (mock test).
- [x] `make check` green.

## Notes from lead
- Contract tasks run in sequence (shared generated files): land before T-033 starts.

## Plan (implementer, ≤15 lines)
1. Schema field `requestTarget` with the rule; gqlgen.yml `resolver: true` for it (stub like Collection).
2. Stub in resolvers/stubs_replay.go returns ErrNotImplemented.
3. Mock computes it from the draft (REQUEST_LINE regex, UTF-8 replace, 2048 chars) + mock test.
4. make gen, make check.

## Out-of-scope change needed (if any)

## Handoff (implementer)
- What changed: `ReplaySession.requestTarget: String!` (schema + docs), `gqlgen.yml` resolver flag, stub in `stubs_replay.go` (ErrNotImplemented), mock + test; `make gen`.
- (gqlgen.yml added to paths: needed for the resolver stub.)
- How to verify: `make check`; `bun run test` in web (replay.test.ts "requestTarget").
- Review round 1: target with an ASCII control byte (0x00-0x1F, 0x7F) → `""` (rule added to the schema description, so T-033 must match); leading U+FEFF kept (`ignoreBOM`); decode at most 8192 bytes (2048 code points max) before cutting. Tests cover tab, DEL, BOM, 3000×`€` → 2048, and a 4-byte char at position 2048.
- Known limitations: backend returns ErrNotImplemented until T-033.
- Follow-ups (proposed tasks): T-033 implements; T-111 switches the UI query.

## Review (reviewer)
- Verdict: approve
- Findings:
  - [x] [major] web/src/lib/api/mocks/replay.ts:119 — The mock returns a target for a malformed first line with an embedded ASCII tab, such as `GET /a\tb HTTP/1.1`. The target character class excludes spaces and line endings but accepts tabs and other ASCII control bytes → reject ASCII controls for this field and add a malformed-line test.
  - [x] [minor] web/src/lib/api/mocks/replay.ts:133 — Decoding the captured target on its own strips an initial U+FEFF even though it is valid UTF-8 within the draft → preserve U+FEFF when decoding the target and cover it in the mock test.
  - [x] [minor] web/src/lib/api/mocks/replay.ts:133 — The spread creates an array containing every decoded code point before applying the 2048-character limit. A target near the 16 MiB draft cap can cause large temporary allocations on a tree fetch → collect at most 2048 code points and test the boundary with a multibyte target.
- Verification: `make check` passed (403 web tests). `make gen` in a temporary checkout reproduced all tracked files byte for byte.
- Re-review (`6e62545`): Confirmed the control-byte rejection, BOM preservation, and bounded UTF-8 decoding.
  - [ ] [minor] web/src/lib/api/mocks/replay.test.ts:38 — The UTF-8 cases cover valid sequences only → add an assertion that a malformed sequence in the target yields U+FFFD.
- Re-review verification: `make check` passed (403 web tests). `make gen` reproduced the tracked files in a temporary checkout.
