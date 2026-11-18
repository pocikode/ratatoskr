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
  - internal/query/httpql/sitemap_compose_test.go
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
- Round 2 (review fixes): composition rule is now "node query alone for an empty/whitespace bar, else
  `(<node>) and (<bar>)`" in F-SITE-1, `SitemapNode.query` docs and T-020 notes (invariant tested with empty and
  non-empty bar). Mock accepts the bare node query and returns `BAD_QUERY` for `(<node>) and ()` like the parser;
  `internal/query/httpql/sitemap_compose_test.go` pins the rule against the real parser (path added to this task).
  Mock QUERY nodes only under path nodes (fixture: `/`, `*`, `/a`); parent ids validated as
  `[tls, host, port(, segs(, names))]` tuples, malformed → `BAD_INPUT`, well-formed absent → `[]`.
- Round 3 (review fixes): composition moved server-side. `SitemapNode.query` now already includes the call's `query`:
  node expression alone if the bar parses to empty (whitespace/comments), else `(<node>) and (<bar>\n)` (newline ends
  a trailing `//` comment); over `MaxQueryLen` → `BAD_QUERY`. Clients pass it unchanged to `requests`/`deleteRequests`
  and never compose. Updated schema docs, F-SITE-1, T-020 notes; `sitemap_compose_test.go` covers empty, whitespace,
  `/* */`-only, `//`-only, trailing `//` and plain bars against the real parser. Mock composes the same way
  (shortcut: only whitespace counts as empty there).

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
- [x] [major] internal/api/graphql/schema/sitemap.graphqls:59 — The prescribed composition
  `(<node query>) and (<query bar>)` fails for the default empty query bar: the real HTTPQL parser rejects
  `(req.host.eq:"example.test") and ()` with `query: empty parentheses (at 33-35)`. The mock strips the empty
  group and accepts it (probe: badge 11, mock count 11), so the current invariant tests conceal the failure.
  T-020 following this contract would fail to load a selected node's table or delete it with an empty bar.
  → Document using the node expression alone when the bar is empty/whitespace, and appending the grouped
  conjunction only for a non-empty bar; update F-SITE-1 and the T-020 invariant guidance consistently and
  regenerate. Cover empty and non-empty composition against the real parser rather than only the mock matcher.
- [x] [minor] web/src/lib/api/mocks/sitemap.ts:72 — Query nodes are appended even when `segs` is empty,
  although F-SITE-1 and the QUERY enum describe children of path nodes. With two same-host rows
  `path: "/", query: "x=1"` and `path: "*", query: "x=2"`, the mock returns a HOST → QUERY `?x` with
  badge 2. Its emitted `req.path.regex:"^/*/*$"` excludes `*`; the registry predicate still counts both,
  hiding the mismatch between the returned HTTPQL and the badge. → Keep requests without path segments
  directly on the host as specified; only append query nodes beneath an actual path node. Add a fixture
  for root and non-origin paths so the mock does not teach T-020 a different tree shape.
- [x] [minor] web/src/lib/api/mocks/sitemap.ts:175 — Parent-ID validation accepts any JSON array.
  Probes with `[]`, `[1]`, and `[true,2,{}]` all return an empty list instead of the documented `BAD_INPUT`.
  This conflates malformed IDs with well-formed nodes that no longer match. → Validate the mock ID's
  tuple shape and element types, with tests distinguishing malformed tuples from a valid absent node.

Verdict: **changes_requested** (1 major, 2 minor). Task status set accordingly. Only this task file changed.

### Round 2 — review of `375f021`

Reviewed `main...HEAD` by Codex (2026-11-19).

- Scope: all 14 changed files match the task's `paths`. Read the full diff, linked spec sections,
  history query/deletion/subscription callers, HTTPQL parser and lexer, and query-bar submission path.
  Assumed one local proxy process; mock checks use 300 seeded requests. Production tree derivation and
  the 10k-request / 200-host performance budget remain T-020 work.
- Previous findings: all three are resolved. Empty/whitespace bars now use the node query alone;
  the real parser regression covers empty-group rejection; query nodes only appear under path nodes;
  malformed ID tuples are rejected while valid absent nodes return an empty list.
- Verification: `make check` passed, including Go race tests and 34 web test files / 211 tests.
  The first sandbox run failed with `context loading failed: no go files to analyze` (trimmed);
  the authorized rerun passed without source changes.
  `make gen` passed in a temporary HEAD copy and byte comparison found no tracked-file differences.
  Its sandbox attempt failed with `open /home/agus/.cache/go-build/…: read-only file system`;
  the authorized rerun passed. `git diff --check main...HEAD` passed.
- Direct mock probe: all 142 nodes' query counts equal their badges; deleting a host query removes
  exactly its 11 requests and the host disappears. Temporary real-parser probes confirmed the finding
  below; adding a newline before the closing parenthesis fixes the trailing-line-comment case.

Findings:
- [x] [major] internal/api/graphql/schema/sitemap.graphqls:61 — The revised composition rule still
  rejects valid query bars containing comments. `req.method.eq:"GET" // note` parses on its own,
  but `(req.host.eq:"example.test") and (req.method.eq:"GET" // note)` fails with
  `query: expected ')' or AND/OR, found end of query (at 62-62)` because the comment consumes `)`.
  A comment-only bar such as `/* note */` is also valid and means no filter, but grouping it produces
  `query: empty parentheses (at 33-45)`. The existing query bar submits this text unchanged.
  → Treat comment-only bars as empty and terminate trailing line comments before the appended `)`.
  Update the schema, F-SITE-1 and T-020 guidance consistently, regenerate, and add both cases to the
  real-parser composition regression. Otherwise a T-020 client following this contract would load
  the tree successfully but fail to load a selected node's requests or delete that node.

Verdict: **changes_requested** (1 major). Task status set accordingly. Only this task file changed.
Not checked: E2E and production sitemap performance (backend/page are T-020); `make test-int`
was not required because no proxy/store/rawhttp files changed.

### Round 3 — review of `5ad0bda`

Reviewed `main...HEAD` by Codex (2026-11-19).

- Scope: all 14 changed files match the task's `paths`. Read the full diff, F-SITE-1,
  architecture §6, Go/frontend conventions, and the connected history resolver, query/filter,
  deletion, subscription and mock flows. Assumed one local proxy process; the mock uses
  300 seeded requests. Production derivation and the 10k-request / 200-host budget belong to T-020.
- Previous finding resolved: `SitemapNode.query` now includes the call's filter on the server;
  clients pass it unchanged. The schema, feature spec and T-020 guidance agree. The real-parser
  regression covers empty, whitespace-only, block/line-comment-only, trailing-line-comment,
  ordinary and OR filters. The resolver remains the intended `ErrNotImplemented` contract stub.
- Verification: `make check` passed (Go fix/vet/lint, race tests, Svelte/TypeScript, ESLint,
  formatting, 34 web test files / 211 tests). `make gen` passed in a temporary HEAD copy;
  byte comparison found no tracked-file differences. `git diff --check main...HEAD` passed.
- Additional mock probe: 354 node/filter combinations returned history counts equal to their
  badges, across empty, whitespace, ordinary and trailing-line-comment bars. Deleting one host
  using its returned query removed exactly its 11 requests and the host disappeared.
- Documented mock limits remain: scope filtering is ignored, general HTTPQL matching is simplified,
  and comment-only bars are not treated as empty. These do not change the production contract.

Findings: none remaining.

Verdict: **approve**. Task remains `in_review` for the operator; this supersedes the previous verdicts.
Only this task file changed.

Not checked: production sitemap E2E and performance (backend/page are T-020); `make test-int`
was not required because no proxy/store/rawhttp files changed.
