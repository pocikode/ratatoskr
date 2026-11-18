---
id: T-020
title: Sitemap tree (backend derivation + page)
milestone: M1
role: backend
status: done
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
1. Batch live refreshes for recursively expanded subtrees, including overlapping expansion roots.
2. Preserve loaded request pages when selected-node counts change; avoid unnecessary table reloads.
3. Bracket IPv6 scope rules, refresh pending deletion counts, and use a plain Set for local deduplication.
4. Add state regressions for the review fixes; record the shared-filter parse nit's scope limitation inline.
5. Run `make check`, update Handoff/Review, commit scoped changes and set `in_review`.

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
- Review revision: refresh each Shift-expanded subtree once, omit covered/hidden expansions and retain correct parent
  grouping; preserve loaded request pages on count changes or request updates, leave unchanged rows untouched otherwise;
  bracket IPv6 scope rules and refresh pending deletion counts without changing the captured scope.
- Tests: service coverage for all node kinds, scope/filter/count agreement, comments, special paths/hosts, malformed IDs,
  ordering, deletion and recursive cap; real GraphQL integration for count agreement, lazy children, deletion and errors;
  ten page/state tests for selection/viewers, expansion, filter arguments, confirmation/cancel, paging, live throttling,
  overlapping recursive roots, collapsed branches, live count/note changes, IPv6 scope rules, stale expansion results,
  cleanup and project reset. jsdom Range geometry shim is local to the page tests.
- Verified for the review revision: `make check` (Go modernization/vet/lint/race tests; web types/lint/format;
  36 files / 221 tests), targeted `bun run test src/pages/sitemap` (10 tests), and `git diff --check`.
  Original implementation also verified: `make gen`, `make build`,
  `go test ./internal/history -run '^$' -bench BenchmarkSitemapRoots -benchtime=3x -count=1`
  (~54.8 ms/op for 10k rows / 200 hosts; benchmark asserts each load < 500 ms), and `git diff --check`.
- Verification caveat: an earlier full race run caught an intermittent existing SQLite function-registration race:
  `internal/store/funcs.go:124` vs direct driver open in `internal/scope/service_test.go:142`. These files are unchanged
  from main. A focused `go test -race ./internal/scope -count=10` and the final unmodified `make check` passed.
  Lead/store owner should track that race separately; no test settings were weakened.
- Not run: browser E2E/visual checks (Playwright is not installed/configured in this checkout) or `make test-int`;
  the added real-handler GraphQL integration and component tests cover this task's behavior. No proxy/raw-traffic changes.
- Review/approval remains pending; no PR opened.
- Follow-ups outside this task: the History page's existing IPv6 scope-rule bug; optional shared-filter optimization
  to expose whether the original bar parsed empty and avoid the harmless second parse. No backend changes in this revision.

## Review (reviewer)
Reviewed 2026-11-19 (Claude Code, reviewer). Scope: `git diff main...HEAD` (15 files). All changed files are inside `paths`;
generated files match `bun run codegen` (no diff). `make check`: green (Go vet/lint/race tests, svelte-check, eslint,
prettier, vitest 36 files / 219 tests). `make test-int` not run: no proxy/store/rawhttp changes.
Spec check (F-SITE-1): host/path/query derivation, directory/file kind, child order, `/a//b/`, `*`/absolute-form paths,
IPv6 labels, query-bar composition (`(node) and (bar⏎)`, empty/comment-only bar), 5000-node cap, `BAD_QUERY` over
`MaxQueryLen` and malformed IDs all match and are tested; the count == `requests(query: n.query)` invariant is tested
across bars, scopes and all four kinds. Preset references in the bar still work (parsed, expanded later by `List`/`Delete`).

- [x] [major] web/src/pages/sitemap/sitemap.svelte.ts:196-199,283-305 — after a Shift-click, every descendant with
  children is added to `expanded` (up to ~5000), and `refresh()` sends one `sitemapNodes` query per expanded node via
  `Promise.all`. Each call rebuilds the whole tree server-side (full `GROUP BY` over filtered history,
  internal/history/sitemap.go:110-112; ~55 ms at 10k rows). Shift-click a host with 1000 directories while the proxy is
  capturing → ~1000 full scans every second, server CPU far above 1 s/s, UI never catches up. → Remember the nodes
  expanded with Shift (`recursiveRoots`); in `refresh()` fetch each of them once with `recursive: true` (rebuild
  `children` like `toggle` does) and skip expanded nodes that sit under one. Add a state test: Shift-expand a subtree
  with N directories, emit `requestAdded`, assert ≤ (1 + recursive roots + other expanded) `SitemapNodes` calls.
  - Implementer: fixed. Track recursive expansion roots, omit covered descendants and hidden expansions from refresh
    calls, and regroup flat recursive results by parent. The live regression checks exactly three calls for roots,
    overlapping recursive expansions, and one unrelated expanded host; it also checks collapse and filter reset.
- [x] [minor] web/src/pages/sitemap/sitemap.svelte.ts:318-321 — each live refresh calls `loadRows()` with `more=false`,
  so rows paged in beyond the first 1000 are dropped and the table jumps back once per second during capture. →
  Reload only when the refreshed `selected.requestCount` changed, and then request `first: rows.length` (or keep the
  current pages) instead of resetting to page one.
  - Implementer: fixed by preserving the loaded page count and reloading through existing 1000-row cursor pages
    (the API's maximum). Unchanged counts leave the row array untouched, except after `requestUpdated`: those events
    must still refresh notes/responses with unchanged counts. Tests cover unchanged counts, a same-count note update,
    and deletion after loading beyond page one. Failed table loads remain eligible for retry on the next refresh.
- [x] [minor] web/src/pages/sitemap/sitemap.svelte.ts:383-389 — "Add host to scope" appends `target.host` raw. For an
  IPv6 host (`::1`, which the tree supports) `scope.ParseRule` (internal/scope/match.go:45-54) finds `:` at index 0 →
  "empty host" error, the action always fails. → Send `` `[${host}]` `` when the host contains `:`. Same bug exists in
  web/src/pages/http-history/history.svelte.ts:119 (out of lane: lead to file a follow-up / shared helper).
  - Implementer: fixed in Sitemap, with coverage for `[::1]` and duplicate prevention. The History page remains a
    lead follow-up outside this task's paths.
- [x] [nit] web/src/pages/sitemap/sitemap.svelte.ts:284 — `new SvelteSet` used only to dedupe a local array; reactivity
  is not needed → `new Set`.
  - Implementer: fixed. The local Set has a T-020 justification for the Svelte reactivity lint rule, which otherwise
    rejects it even though the worklist has no reactive consumers.
- [x] [nit] web/src/pages/sitemap/sitemap.svelte.ts:41,317 — the confirm dialog shows the count captured when it opened;
  refreshes update `target` but not `confirmation.node`, so with live traffic the dialog under-reports what `Delete`
  removes. → In `refresh()`, also swap `confirmation.node` for its refreshed copy (keep `scopeId`).
  - Implementer: fixed. Refetch the confirmation node's parent when needed, replace its node while retaining the
    captured scope, and dismiss the dialog if the node disappeared. The live test checks the refreshed count/scope.
- [x] [nit] internal/history/sitemap.go:97-105 — the bar is parsed a second time after `s.filter` already parsed it;
  harmless, but `filter` could return whether the parsed node is nil to avoid the duplicate parse.
  - Implementer: not changed; recommend retaining this harmless parse in T-020. `s.filter` combines the bar with the
    scope and expands preset references, so its resulting AST cannot identify an empty original bar. Exposing that
    information requires changing shared `internal/history/service.go`, outside this task's paths. The current parse
    preserves the tested whitespace/comment-only composition semantics; a shared-filter optimization can follow separately.

Verdict: **changes_requested** — fix the major (refresh fan-out after Shift-expand); minors recommended in the same pass.

Implementer revision (2026-11-19, Codex): five findings fixed, the harmless shared-filter parse nit retained with the
scope rationale above. Final `make check`: green (36 web test files / 221 tests). Ready for re-review.

### Re-review 2026-11-19 (Claude Code, reviewer)
Scope: fix commit 2ba597b plus the full `git diff main...HEAD`; all files still inside `paths`. `make check`: green
(Go vet/lint/race; web 36 files / 221 tests). `make test-int` not run: no proxy/store/rawhttp changes.
All five earlier fixes are confirmed. Refresh now sends one `recursive: true` call per Shift-expanded root and skips covered
or hidden descendants (test at sitemap.svelte.test.ts:70). Loaded pages are kept. IPv6 rules are bracketed. The confirm
dialog count updates. Keeping the extra parse nit is accepted, since fixing it needs `service.go`, which is outside `paths`.

- [ ] [minor] web/src/pages/sitemap/sitemap.svelte.ts:342-346 — refresh is all-or-nothing. If one live update pushes a
  Shift-expanded subtree past the 5000-node cap (internal/history/sitemap.go:204), that branch returns `BAD_INPUT` on every
  refresh. Then nothing updates: no roots, no other branches, no table. This lasts until the user collapses the node, and
  the message does not tell them to do that. → Apply the results that succeeded and report only the failed branch. For a
  failed recursive branch, also run `recursiveRoots.delete(parentId)` and add " Expand level by level." to the message.
- [ ] [minor] web/src/pages/sitemap/sitemap.svelte.ts:393-401,360-363 — `rowsDirty` is set only by `requestUpdated`. Suppose
  one request under the selected node is deleted and another is added in the same 1 s window. `requestCount === total`, so
  the table is not reloaded: it keeps showing the deleted row and misses the new one. → Also set `rowsDirty` in a `tap` on
  the `requestsDeleted` stream. Deletes are rare, so the extra reload costs little.
- [ ] [nit] web/src/pages/sitemap/sitemap.svelte.ts:347-351 — directories that appear inside a Shift-expanded subtree
  during a live refresh are loaded into `children` but are not added to `expanded`. They show as collapsed next to
  expanded siblings. → In the recursive branch of refresh, add `hasChildren` nodes to `expanded` the same way `toggle` does.

Verdict: **approve** — no blocker/major left. The minors can go in a follow-up or in this branch before merge.
