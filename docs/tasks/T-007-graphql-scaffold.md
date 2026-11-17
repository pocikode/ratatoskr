---
id: T-007
title: GraphQL server scaffold, subscriptions, errors, auth/CSRF, codegen both sides
milestone: M0
role: lead
status: in_review
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
  - cmd/ratatoskr/main.go
  - cmd/ratatoskr/main_test.go
spec:
  - docs/03-architecture.md §6, §8
  - docs/adr/0004-graphql-api.md
---

## Goal
`/api/graphql` (HTTP) and `/api/graphql/ws` (graphql-transport-ws) are served; schema dir with `base.graphqls` (scalars: Time, Bytes(base64), Int64; `Node` interface; error codes),
`project.graphqls` contract; `make gen` regenerates Go and TS; the web app has an urql client with WS subscriptions and a demo `serverInfo` query shown in the status bar.

## Acceptance criteria
- [x] `make gen` is deterministic (running twice yields no diff).
- [x] Error mapping: service errors → `extensions.code`; query errors carry `extensions.span`.
- [x] CSRF: requests without `X-Ratatoskr: 1` or wrong content-type are rejected; `http.CrossOriginProtection` applied.
- [x] A `ping` subscription emits every second; the UI shows connection status in the status bar.
- [x] Contract for projects (types, queries, mutations, `projectChanged` subscription) documented with descriptions.

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
- What changed:
  - Spec: docs/03 §6 (error codes incl. new `BAD_INPUT`/`UNAUTHENTICATED`/`NOT_IMPLEMENTED`, `internal/apperr`, scalar
    wire formats, layout), §8 (CSRF + token auth details); docs/06 (`@urql/core` instead of `@urql/svelte`, one client).
  - `internal/apperr`: `Code`, `Error{Code,Msg,Err,Span}`, `New`, `Query`, `CodeOf`, `ErrNotImplemented`.
  - `gqlgen.yml` → `internal/api/graphql/generated` (exec + models; no gqlgen-managed resolvers). Schema
    `base.graphqls` (Time/Bytes/Int64, `Node`, `ErrorCode`, `serverInfo`, `ping`) + `project.graphqls` (F-WSP-1 contract).
  - `internal/api/graphql`: handler (POST + graphql-transport-ws, keep-alive, query LRU), error presenter, panic recovery,
    WS `connection_init.token` auth (close 4401). `resolvers/`: root, `base.go`, `project.go` stubs (T-006 replaces).
    `scalars/`: marshalers.
  - `internal/api/middleware`: `CrossOrigin`, `RequireAPIRequest` (415/403), `Auth` (Bearer / `ratatoskr_token` cookie /
    guests on loopback with `--allow-guests`). `internal/api/server.go` mounts `/api/**`; `rest/router.go` stub.
  - `internal/app`: mounts the API, `--auth-token` now works, exposure warning only without a token; `Options.Version/Commit`
    passed from `cmd/ratatoskr/main.go`.
  - Web: deps `@urql/core`, `graphql`, `graphql-ws`, dev `@graphql-codegen/cli` + `client-preset`; `codegen.ts`;
    `lib/api/client.ts` (`getApi()`), `connection.svelte.ts` (`ConnectionState`), `status.graphql`; StatusBar shows
    Connecting…/Connected/Disconnected, retrying/Not authorized + server version; Vite dev proxy has `ws: true`.
  - New task T-024 (login/logout endpoints) + roadmap row.
- How to verify: `make gen && git status` (clean, twice); `make check`; `make run`, open the UI → status bar turns
  "Connected" and shows "Ratatoskr <version>"; stop the binary → "Disconnected, retrying", restart → reconnects.
  `curl -XPOST localhost:8080/api/graphql -d '{"query":"{serverInfo{version}}"}'` → 415; add
  `-H 'Content-Type: application/json' -H 'X-Ratatoskr: 1'` → data.
- Known limitations:
  - With `--auth-token`, browsers have no way to obtain the cookie until T-024 + a login page land (API clients use Bearer).
  - Project behaviours decided in `project.graphqls` descriptions (rename saves a temp project, temp cannot be default,
    duplicate name defaults to "<name> (copy)", list order = most recently opened) should be copied into F-WSP-1 (docs/01).
  - A rejected cross-origin WS logs gqlgen's "superfluous WriteHeader" warning (harmless, upstream behaviour).
  - The web codegen emits types only for operations in `src/**/*.graphql`; project types appear once T-009 adds operations.
  - `internal/legacy/api` still has a `go:generate` that re-runs our gqlgen config (same output) until T-002 removes it.
- Follow-ups (proposed tasks):
  - T-025 (frontend): login page shown on `unauthorized`, posting to T-024's endpoint, then recreating the WS client.
  - Lead (docs): copy the project rules above into docs/01 F-WSP-1.

## Review (reviewer)
