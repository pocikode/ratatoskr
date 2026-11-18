---
id: T-020
title: Sitemap tree (backend derivation + page)
milestone: M1
role: backend
status: todo
depends_on: [T-016, T-094]
owner: ""
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
  `deleteRequests(query: "(<node.query>) and (<bar>)", scopeId)`.
- Backend: `SitemapNode.query` is server-built HTTPQL (e.g. `req.path.regex` with quoted segments). Test the invariant
  `requests(query: "(n.query) and (q)", scopeId).totalCount == n.requestCount` for HOST/DIRECTORY/FILE/QUERY nodes,
  including `/a//b/`, `*` paths, IPv6 hosts and regex metacharacters in segments. Perf budget: roots for 10k requests /
  200 hosts < 500 ms (benchmark).
- Replace the stub in `internal/api/graphql/resolvers/sitemap.go` (inside this task's `paths`).

## Acceptance criteria
- [ ] Tree children loaded lazily per node; counts correct; respects scope + query.
- [ ] 10k requests / 200 hosts: root load < 500 ms.
- [ ] Delete node deletes underlying requests after confirmation.

## Plan (implementer, ≤15 lines)

## Out-of-scope change needed (if any)
RESOLVED by lead (T-094): `internal/api/graphql/schema/sitemap.graphqls` did not exist (lead-owned, AGENTS.md rule 5; task notes say lead adds it first). Lead needs to add a contract (e.g. `sitemapRoots(scope, query)`, lazy `sitemapChildren(nodeId)`, `deleteSitemapNode`, live `sitemapUpdated` subscription), with stub resolvers as in T-091. Then run `make gen`, and T-020 can proceed. Also: `history` has no sitemap derivation yet; T-090 note says to move `And`/`Or` helpers into `internal/query/core` when this task adds a third copy.

## Handoff (implementer)

## Review (reviewer)
