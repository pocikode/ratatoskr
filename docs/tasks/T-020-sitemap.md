---
id: T-020
title: Sitemap tree (backend derivation + page)
milestone: M1
role: backend
status: in_review
depends_on: [T-016, T-094]
owner: "codex (GPT-6)"
branch: task/T-020-sitemap
paths:
  - internal/history/sitemap*.go
  - internal/api/graphql/resolvers/sitemap*.go
  - web/src/pages/sitemap/**
  # Added 2026-11-19 (lead, operator-approved): codegen output for pages/sitemap/*.graphql; `make gen` only, never hand-edited.
  - web/src/lib/api/generated/**
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
- [x] Tree children loaded lazily per node; counts correct; respects scope + query. Service/GraphQL tests and page/state tests.
- [x] 10k requests / 200 hosts: root load < 500 ms. `BenchmarkSitemapRoots`: ~54.8 ms/op (3 iterations, Ryzen 7 8845HS).
- [x] Delete node deletes underlying requests after confirmation. Page test checks cancel/confirm and unchanged node selector; GraphQL test verifies history deletion.

## Plan (implementer, ≤15 lines)
1. Reuse history query/scope filtering to derive stable, lazy sitemap nodes and exact HTTPQL selectors.
2. Wire the existing GraphQL contract; cover counts, ordering, malformed IDs, deletion and the 10k-row budget.
3. Replace the placeholder with the tree, shared traffic table/viewers, scope/query controls and confirmed node deletion.
4. Reuse history subscriptions with throttled refresh; test expansion, filtering and deletion in the page.
5. Run `make check`, fill Handoff and set `in_review`.

## Out-of-scope change needed (if any)
2026-11-19 (Codex): UI implementation needs `web/src/lib/api/generated/**` added to `paths` for
codegen output. The new page operation belongs in `web/src/pages/sitemap/*.graphql`, but its typed
document/types are emitted outside the allowed paths. Frontend conventions require page-local
`.graphql` operations with generated typed documents; using handwritten operation types would
bypass that convention. Per AGENTS.md rule 1, stopped before adding the operation or changing generated files.
Question for lead/operator: may T-020 regenerate `web/src/lib/api/generated/**` with `make gen`?

RESOLVED by lead (2026-11-19, operator-approved): yes. `web/src/lib/api/generated/**` added to `paths`, same as
T-009/T-017/T-019. Regenerate only (`make gen` / `bun run codegen`), never hand-edit (AGENTS.md rule 4). No schema
change: the page operation goes in `web/src/pages/sitemap/sitemap.graphql` against the T-094 contract.
Status back to `in_progress`.

RESOLVED by lead (T-094): `internal/api/graphql/schema/sitemap.graphqls` did not exist (lead-owned, AGENTS.md rule 5; task notes say lead adds it first). Lead needs to add a contract (e.g. `sitemapRoots(scope, query)`, lazy `sitemapChildren(nodeId)`, `deleteSitemapNode`, live `sitemapUpdated` subscription), with stub resolvers as in T-091. Then run `make gen`, and T-020 can proceed. Also: `history` has no sitemap derivation yet; T-090 note says to move `And`/`Or` helpers into `internal/query/core` when this task adds a third copy.

## Handoff (implementer)
- Status: `in_review`; generated-file scope blocker resolved by the lead. Dependencies T-016/T-094 merged on main.
- Backend: history-derived nodes with stable opaque IDs, deterministic lazy/recursive children, scope/query filtering,
  exact server-composed selectors, query-length checks and the 5000-node recursive cap; GraphQL resolver wired.
- UI: virtualized keyboard-accessible tree, lazy expansion and Shift-click subtree expansion; query/scope controls;
  shared TrafficTable and request/response MessageViewers; cursor paging beyond 1000 rows; command-registry context
  menu for copy URL, add host to active scope and confirmed delete using the unchanged node query and captured scope.
- Live: reuse the three history subscriptions; throttle root/expanded-node refetches to once per second, restart ended
  streams, invalidate stale results and clear selections/confirmations on project switches. No new dependencies/schema changes.
- Tests: service coverage for all node kinds, scope/filter/count agreement, comments, special paths/hosts, malformed IDs,
  ordering, deletion and recursive cap; real GraphQL integration for count agreement, lazy children, deletion and errors;
  eight page/state tests for selection/viewers, expansion, filter arguments, confirmation/cancel, paging, live throttling,
  stale expansion results, cleanup and project reset. jsdom Range geometry shim is local to the page tests.
- Verified: `make gen`, `make check` (Go modernization/vet/lint/race tests; web types/lint/format; 36 files / 219 tests),
  `make build`, `go test ./internal/history -run '^$' -bench BenchmarkSitemapRoots -benchtime=3x -count=1`
  (~54.8 ms/op for 10k rows / 200 hosts; benchmark asserts each load < 500 ms), and `git diff --check`.
- Verification caveat: an earlier full race run caught an intermittent existing SQLite function-registration race:
  `internal/store/funcs.go:124` vs direct driver open in `internal/scope/service_test.go:142`. These files are unchanged
  from main. A focused `go test -race ./internal/scope -count=10` and the final unmodified `make check` passed.
  Lead/store owner should track that race separately; no test settings were weakened.
- Not run: browser E2E/visual checks (Playwright is not installed/configured in this checkout) or `make test-int`;
  the added real-handler GraphQL integration and component tests cover this task's behavior. No proxy/raw-traffic changes.
- Review/approval remains pending; no PR opened.

## Review (reviewer)
