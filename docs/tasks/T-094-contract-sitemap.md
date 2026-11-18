---
id: T-094
title: Contract - sitemap
milestone: M1
role: lead
status: in_review
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
