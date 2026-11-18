---
id: T-020
title: Sitemap tree (backend derivation + page)
milestone: M1
role: backend
status: in_progress
depends_on: [T-016, T-094]
owner: "codex (GPT-6)"
branch: task/T-020-sitemap
paths:
  - internal/history/sitemap*.go
  - internal/api/graphql/resolvers/sitemap*.go
  - web/src/pages/sitemap/**
spec:
  - docs/01-feature-spec.md#F-SITE-1
  - docs/02-ui-layout.md §3.2
---

## Notes from lead
This task is split into two commits/branches if two agents are available: backend part (role backend) then UI part (role frontend). Lead adds `sitemap.graphqls` first.

Contract landed in T-094 (`sitemap.graphqls`, stub `resolvers/sitemap.go`, mock `web/src/lib/api/mocks/sitemap.ts`).
Read the expanded F-SITE-1 for the derivation rules. Key points:
- One query: `sitemapNodes(parentId, query, scopeId, recursive)`. No sitemap subscription and no delete mutation: the page
  reuses `requestAdded`/`requestUpdated`/`requestsDeleted` (refetch roots + expanded nodes, ≤ 1/s) and
  `deleteRequests(query: node.query, scopeId)`. `node.query` already includes the bar the tree was fetched with;
  the page never composes queries (use `node.query` unchanged for table and delete).
- Backend: `SitemapNode.query` is server-built HTTPQL (e.g. `req.path.regex` with quoted segments), combined with the
  call's `query`: node expression alone if it parses to empty (`core` Parse → nil: whitespace/comments only), else
  `"(" + node + ") and (" + q + "\n)"`; over `MaxQueryLen` → `BAD_QUERY`. Rule pinned by
  `internal/query/httpql/sitemap_compose_test.go`. Test the invariant
  `requests(query: n.query, scopeId).totalCount == n.requestCount` with `q` empty, comment-only, ending in a `//`
  comment and plain, for HOST/DIRECTORY/FILE/QUERY nodes, including `/a//b/`, `*` paths, IPv6 hosts and regex
  metacharacters in segments. Perf budget: roots for 10k requests / 200 hosts < 500 ms (benchmark).
- Replace the stub in `internal/api/graphql/resolvers/sitemap.go` (inside this task's `paths`).

## Acceptance criteria
- [ ] Tree children loaded lazily per node; counts correct; respects scope + query.
- [ ] 10k requests / 200 hosts: root load < 500 ms.
- [ ] Delete node deletes underlying requests after confirmation.

## Plan (implementer, ≤15 lines)
1. Reuse history query/scope filtering to derive stable, lazy sitemap nodes and exact HTTPQL selectors.
2. Wire the existing GraphQL contract; cover counts, ordering, malformed IDs, deletion and the 10k-row budget.
3. Replace the placeholder with the tree, shared traffic table/viewers, scope/query controls and confirmed node deletion.
4. Reuse history subscriptions with throttled refresh; test expansion, filtering and deletion in the page.
5. Run `make check`, fill Handoff and set `in_review`.

## Out-of-scope change needed (if any)
RESOLVED by lead (T-094): `internal/api/graphql/schema/sitemap.graphqls` did not exist (lead-owned, AGENTS.md rule 5; task notes say lead adds it first). Lead needs to add a contract (e.g. `sitemapRoots(scope, query)`, lazy `sitemapChildren(nodeId)`, `deleteSitemapNode`, live `sitemapUpdated` subscription), with stub resolvers as in T-091. Then run `make gen`, and T-020 can proceed. Also: `history` has no sitemap derivation yet; T-090 note says to move `And`/`Or` helpers into `internal/query/core` when this task adds a third copy.

## Handoff (implementer)

## Review (reviewer)
