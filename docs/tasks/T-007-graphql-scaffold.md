---
id: T-007
title: GraphQL server scaffold, subscriptions, errors, auth/CSRF, codegen both sides
milestone: M0
role: lead
status: in_progress
depends_on: [T-004, T-003]
owner: "claude-code (claude-opus-5-5)"
branch: task/T-007-graphql-scaffold
paths:
  - internal/api/graphql/**
  - internal/api/server.go
  - internal/api/middleware/**
  - internal/api/rest/router.go
  - internal/app/**
  - gqlgen.yml
  - web/codegen.ts
  - web/src/lib/api/**
  - web/package.json
  - web/bun.lock
  - Makefile
  - go.mod
  - go.sum
  # added at start with operator approval (scaffold needs them; no open task owns them):
  - internal/apperr/**
  - web/vite.config.ts
  - web/src/lib/components/StatusBar*.svelte
  - web/src/lib/components/StatusBar*.test.ts
  - web/eslint.config.js
  - web/.prettierignore
  - docs/03-architecture.md
  - docs/06-frontend-conventions.md
  - docs/tasks/T-007-graphql-scaffold.md
  - docs/tasks/T-024-*.md
  - docs/08-roadmap.md
spec:
  - docs/03-architecture.md §6, §8
  - docs/adr/0004-graphql-api.md
---

## Goal
`/api/graphql` (HTTP) and `/api/graphql/ws` (graphql-transport-ws) are served; schema dir with `base.graphqls` (scalars: Time, Bytes(base64), Int64; `Node` interface; error codes),
`project.graphqls` contract; `make gen` regenerates Go and TS; the web app has an urql client with WS subscriptions and a demo `serverInfo` query shown in the status bar.

## Acceptance criteria
- [ ] `make gen` is deterministic (running twice yields no diff).
- [ ] Error mapping: service errors → `extensions.code`; query errors carry `extensions.span`.
- [ ] CSRF: requests without `X-Ratatoskr: 1` or wrong content-type are rejected; `http.CrossOriginProtection` applied.
- [ ] A `ping` subscription emits every second; the UI shows connection status in the status bar.
- [ ] Contract for projects (types, queries, mutations, `projectChanged` subscription) documented with descriptions.

## Plan (implementer, ≤15 lines)
1. Spec first: §6 error codes (+`BAD_INPUT`, `NOT_IMPLEMENTED`, `UNAUTHENTICATED`), `internal/apperr` (code + span) as the one
   way services signal codes; §8 auth details (bearer/cookie, guests on loopback, WS `connection_init.token`); docs/06 urql choice.
2. `gqlgen.yml` → schema `internal/api/graphql/schema/*.graphqls`, exec+models → `internal/api/graphql/generated`,
   no gqlgen-managed resolvers: hand-written `internal/api/graphql/resolvers` (root + `base.go`, `project.go` stubs).
3. Schema: `base.graphqls` (Time, Bytes, Int64 scalars; `Node`; `ErrorCode` enum; `serverInfo`; `ping`), `project.graphqls` (F-WSP-1).
4. `internal/api/graphql`: handler (POST + WS graphql-transport-ws), error presenter, panic recovery.
5. `internal/api/middleware`: CSRF (content-type + `X-Ratatoskr: 1`, `http.CrossOriginProtection`), token auth.
6. `internal/api/server.go` mounts `/api/graphql`, `/api/graphql/ws`, `/api/rest/` (router stub); `internal/app` uses it, --auth-token enabled.
7. Web: deps (`@urql/core`, `graphql-ws`, `graphql`, codegen), `codegen.ts`, `lib/api/client.ts` + status store, StatusBar shows
   connection + server version; `make gen` deterministic (run twice, diff empty); tests both sides; follow-up T-024 (login page).

## Out-of-scope change needed (if any)

## Handoff (implementer)

## Review (reviewer)
