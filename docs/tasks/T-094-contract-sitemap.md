---
id: T-094
title: Contract - sitemap
milestone: M1
role: lead
status: changes_requested
depends_on: [T-013]
owner: "claude-code (opus-5.5)"
branch: task/T-094-contract-sitemap
paths:
  - internal/api/graphql/schema/sitemap.graphqls
  - internal/api/graphql/resolvers/sitemap.go
  - internal/api/graphql/generated/**
  - web/src/lib/api/generated/**
  - web/src/lib/api/mocks/**
  - docs/01-feature-spec.md
  - docs/08-roadmap.md
  - docs/tasks/T-094-contract-sitemap.md
  - docs/tasks/T-020-sitemap.md
spec:
  - docs/01-feature-spec.md#F-SITE-1
  - docs/03-architecture.md §6
---

## Goal
GraphQL contract for the sitemap tree so T-020 can implement the backend and the page. Unblocks T-020, which was
blocked on the missing `sitemap.graphqls`.

## Scope
- In: F-SITE-1 derivation rules written into the spec; `sitemap.graphqls` (`SitemapNodeKind`, `SitemapNode`,
  `sitemapNodes(parentId, query, scopeId, recursive)`); stub resolver returning `ErrNotImplemented`; `make gen`;
  mock derived from the history mock rows, with node queries understood by the history mock's `requests`.
- Out: a sitemap subscription (the page reuses `requestAdded`/`requestUpdated`/`requestsDeleted` and refetches, spec
  F-SITE-1); a delete-node mutation (`deleteRequests(query: "(<node query>) and (<bar>)", scopeId)`).

## Acceptance criteria
- [x] Every type/field/argument documented, incl. errors (`BAD_INPUT` malformed id / > 5000 nodes, `BAD_QUERY` and
      `NOT_FOUND` as `requests`).
- [x] `make gen` clean; stub compiles; `make check` green.
- [x] Mock serves `sitemapNodes` (lazy + recursive); `requests(query: "(<node.query>)")` count equals `requestCount` (vitest).

## Notes from lead
- Design choice: each node carries an HTTPQL `query` selecting exactly its requests, so table, delete and live updates
  reuse the history API unchanged. T-020 backend must test the invariant
  `requests(query: "(n.query) and (q)", scopeId).totalCount == n.requestCount` for every node kind.

## Plan (implementer, ≤15 lines)
1. Spec F-SITE-1 derivation. 2. Schema + stub + gen. 3. Mock + vitest. 4. Unblock T-020, roadmap.

## Out-of-scope change needed (if any)

## Handoff (implementer)
- What changed: F-SITE-1 derivation rules; `sitemap.graphqls`; `resolvers/sitemap.go` stub; generated code;
  `mocks/sitemap.ts` (+ test) wired into `mockHistoryExchange`; history mock matcher accepts `(<node query>) and (<rest>)`.
- How to verify: `make gen && git diff --exit-code`, `make check`, `cd web && bunx --bun vitest run src/lib/api/mocks`.
- Known limitations: mock ignores `scopeId` (as the history mock does); mock node queries are only recognised as the
  leading `(…)` group, not anywhere in a query.
- Follow-ups: T-020 (backend part, then UI part).

## Review (reviewer)
Reviewed `main...HEAD` at `4a09453` by Codex (2026-11-19).

- Scope: all 13 changed files match the task's `paths`. Read the full diff, F-SITE-1, architecture §6,
  and the connected history schema, mock matcher/deletion flow and HTTPQL parser. The resolver correctly returns
  `apperr.ErrNotImplemented`; the real derivation and performance benchmark belong to T-020.
- Verification: `make check` passed (Go fix/vet/lint, race tests, Svelte/TypeScript, ESLint, formatting,
  34 web test files / 209 tests). The initial sandbox attempt failed with
  `open /home/agus/.cache/go-build/…: read-only file system`; the authorized rerun passed.
- `make gen` passed in a temporary copy of HEAD; byte comparison found no changes to tracked files.
  `git diff --check main...HEAD` passed. A direct mock probe confirmed deleting a host's node query removes
  exactly its badge count and the host disappears. No proxy/store/rawhttp changes, so `make test-int` was not run.

Findings:
- [ ] [major] internal/api/graphql/schema/sitemap.graphqls:59 — The prescribed composition
  `(<node query>) and (<query bar>)` fails for the default empty query bar: the real HTTPQL parser rejects
  `(req.host.eq:"example.test") and ()` with `query: empty parentheses (at 33-35)`. The mock strips the empty
  group and accepts it (probe: badge 11, mock count 11), so the current invariant tests conceal the failure.
  T-020 following this contract would fail to load a selected node's table or delete it with an empty bar.
  → Document using the node expression alone when the bar is empty/whitespace, and appending the grouped
  conjunction only for a non-empty bar; update F-SITE-1 and the T-020 invariant guidance consistently and
  regenerate. Cover empty and non-empty composition against the real parser rather than only the mock matcher.
- [ ] [minor] web/src/lib/api/mocks/sitemap.ts:72 — Query nodes are appended even when `segs` is empty,
  although F-SITE-1 and the QUERY enum describe children of path nodes. With two same-host rows
  `path: "/", query: "x=1"` and `path: "*", query: "x=2"`, the mock returns a HOST → QUERY `?x` with
  badge 2. Its emitted `req.path.regex:"^/*/*$"` excludes `*`; the registry predicate still counts both,
  hiding the mismatch between the returned HTTPQL and the badge. → Keep requests without path segments
  directly on the host as specified; only append query nodes beneath an actual path node. Add a fixture
  for root and non-origin paths so the mock does not teach T-020 a different tree shape.
- [ ] [minor] web/src/lib/api/mocks/sitemap.ts:175 — Parent-ID validation accepts any JSON array.
  Probes with `[]`, `[1]`, and `[true,2,{}]` all return an empty list instead of the documented `BAD_INPUT`.
  This conflates malformed IDs with well-formed nodes that no longer match. → Validate the mock ID's
  tuple shape and element types, with tests distinguishing malformed tuples from a valid absent node.

Verdict: **changes_requested** (1 major, 2 minor). Task status set accordingly. Only this task file changed.
