---
id: T-007
title: GraphQL server scaffold, subscriptions, errors, auth/CSRF, codegen both sides
milestone: M0
role: lead
status: done
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
  # added in review round 1 with operator approval:
  - internal/api/*_test.go
  - docs/05-go-conventions.md
  - docs/tasks/T-026-*.md
  - docs/tasks/T-027-*.md
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
  - T-026 (lead, created): copy the project rules above into docs/01 F-WSP-1 before T-006.
  - T-027 (backend, created): Host allowlist against DNS rebinding + `--allow-guests` help text (docs/03 §8).
  - T-024 note: `Auth.Handler` is now strict everywhere, so the login endpoint needs an explicit exemption there.
  - Carried from review round 2 (not fixed here): T-006 — `apperr` messages reach clients, so don't `%w` raw OS/SQL
    errors into `Msg` (doc in docs/03 §6 or add `apperr.Wrap`), and handle multiple `%w` causes; T-024 — WS test with
    HTTP credentials and no `connection_init.token` → `connection_ack`; any backend task — resolver panic log with
    stack trace, logged once.
- Review round 1 (changes): `Auth.Handler` strict + `Auth.WebSocket` for the WS endpoint only; pprof behind auth;
  case-insensitive Bearer; error presenter classifies by cause (plain resolver errors → `INTERNAL`), scalar errors are
  `apperr` `BAD_INPUT`; 32 MiB POST body cap (413); docs/03 §6/§8/§9, docs/05 §2 (`coder/websocket`), roadmap; tests
  through the real handler for auth bypass, error codes and body limit.

## Review (reviewer)
Reviewer: claude-code (claude-opus-5-5), 2026-11-18, branch `task/T-007-graphql-scaffold` @ 048b22b.

**Checks run**
- `make check`: green (go fix diff, vet, golangci-lint, `go test -race`, svelte-check/eslint/prettier, vitest 50/50).
- `make gen` twice: exit 0, `git status` clean both times. AC 1 holds.
- `make test-int`: not run (no proxy/store/rawhttp changes).
- Manual probe: built the binary and ran it with `--auth-token s3cret`. `GET /api/rest/files/1` returns 401, but with
  `-H 'Upgrade: websocket'` it returns 404. `/api/nope` behaves the same way (see the first major).
- Paths: every changed file matches `paths` except `internal/api/server_test.go` (see the third major).

**AC coverage**: make gen deterministic ✔ (manual) · error mapping ✔ in the unit test of `presentError`, but the real
gqlgen path is not tested and is wrong (see the second major) · CSRF + `CrossOriginProtection` ✔ (`TestCSRF`,
`TestCrossOrigin`, `TestRequireAPIRequest`) · ping every second ✔ (synctest) + status bar states ✔ (vitest) · project
contract with descriptions ✔.

**Findings**
- [x] [major] internal/api/middleware/auth.go:49 — Auth bypass on every GET under `/api/`. `isWebSocketUpgrade` only
  checks `GET` + `Upgrade: websocket`, so any GET route lets an unauthenticated request through when it carries that
  header. I confirmed this on the running binary: 401 becomes 404 on `/api/rest/files/1`. No GET route exposes data
  yet, but T-008 (`GET /api/rest/ca.crt`) and the files, exports, backups and preview downloads will all be readable
  without the token. → Keep `Auth.Handler` strict everywhere. Add a separate wrapper (e.g. `Auth.WebSocket(next)`)
  that passes unauthenticated upgrades through with `Authenticated()==false`, and use it only on
  `GET /api/graphql/ws` (internal/api/server.go:40), e.g. by giving the WS route its own handler chain. Add an
  integration test: token on, `GET /api/rest/x` with `Upgrade: websocket` and no credentials → 401.
  **Fixed** (c1613b4): `Auth.Handler` is strict; `Auth.WebSocket` passes unauthenticated upgrades and is used only for
  `GET /api/graphql/ws` (own chain in server.go). Tests: `TestAuth` (upgrade on a strict route → 401, WS route without
  upgrade → 401), `TestAuthUpgradeHeaderDoesNotBypass` (`/api/rest/files/1`, `/api/nope` with `Upgrade` → 401).
- [x] [major] internal/api/graphql/graphql.go:104,124 — In production, unclassified resolver errors come out as
  `BAD_INPUT` with their raw message, when they should be `INTERNAL` with a generic message plus a log line. gqlgen's
  `graphql.AddError` calls `ErrorOnPath`, which wraps every plain error in `*gqlerror.Error` (`gqlerror.WrapPath`)
  before the presenter sees it. So `isGQL` is always true for a resolver returning e.g. `fmt.Errorf("open %s: %w",
  path, err)`. The message (paths, SQL, etc.) leaks to the client, nothing is logged, and docs/03 §6 ("INTERNAL …
  message replaced by 'internal error' and the cause logged") is violated. The unit test misses this because it passes
  the raw `io.ErrUnexpectedEOF` (graphql_test.go:45), and the "argument error" case (graphql_test.go:57) has the same
  shape as a wrapped resolver error. → Classify by cause, not by wrapper. A `*gqlerror.Error` with `Err == nil`
  (gqlgen-built messages) → `BAD_INPUT`. A `*gqlerror.Error` wrapping a non-apperr cause → `INTERNAL`. Make the
  scalar unmarshalers in `scalars/scalars.go` return `apperr.New(apperr.BadInput, …)` so argument errors keep
  `BAD_INPUT` through `apperr`. Add a test through the real handler: a resolver returning a plain error must give
  `INTERNAL` / "internal error" and a log line, and a bad `Int64` variable must give `BAD_INPUT`.
  **Fixed** (1268c8e): a gqlerror is `BAD_INPUT` only when it has no cause; scalar unmarshalers return
  `apperr` `BAD_INPUT`. Tests: `TestHandlerErrorCodes` through the real handler (plain resolver error → `INTERNAL`,
  generic message, cause logged; panic → `INTERNAL`; missing variable keeps gqlgen's code), `TestPresentError` with
  `graphql.ErrorOnPath` (what generated code does) for a bad `Int64` → `BAD_INPUT`. No schema field takes an `Int64`
  argument yet, so a bad `Int64` variable cannot be sent through the handler; checked that the tests fail on the old code.
- [x] [major] internal/api/server_test.go — This file is outside the task's `paths` (only `internal/api/server.go` is
  listed). Hard rule 1. → No code change: the operator adds `internal/api/server_test.go` (or
  `internal/api/*_test.go`) to `paths`, with the same "operator approval" note as the other additions.
  **Fixed**: operator approved; `internal/api/*_test.go` added to `paths`.
- [x] [minor] docs/03-architecture.md:149 — The spec says services use "`apperr.NotFound(...)` & co.", but
  `apperr.NotFound` is a `Code` constant, not a constructor. The real API is `apperr.New(apperr.NotFound, …)`. →
  Change the doc to `apperr.New(code, …)` / `apperr.Query(span, …)`, or add the constructors.
  **Fixed**: docs/03 §6 now names `apperr.New(code, …)` / `apperr.Query(span, …)`.
- [x] [minor] internal/api/graphql/graphql.go:66 — `transport.POST` reads the body with an unbounded `io.ReadAll`
  (gqlgen http_post.go:48). The WS side is capped at 1 MiB by gqlgen. → Wrap the POST route in
  `http.MaxBytesHandler` (server.go, `POST /api/graphql`) with a named constant (e.g. 32 MiB, large enough for
  base64 Replay bodies). Record the limit in docs/03 §6.
  **Fixed**: `POST /api/graphql` capped at `api.MaxGraphQLRequestBytes` (32 MiB). A declared larger `Content-Length`
  gets 413, and a longer chunked body fails as a GraphQL error (`TestGraphQLBodyLimit`). Recorded in docs/03 §6.
- [x] [minor] go.mod:10 — `github.com/coder/websocket` became a direct dependency (used only by server_test.go). It
  is not on the docs/05 §2 approved list, even though gqlgen already pulls it in as its default WS implementation. →
  Lead adds it to docs/05 §2 (Testing, or "GraphQL server" as the gqlgen WS implementation), or the tests talk to
  the socket another way.
  **Fixed**: operator approved; added to docs/05 §2 next to gqlgen.
- [x] [minor] internal/app/app.go:219 — With `--debug --auth-token`, `/debug/pprof/*` is mounted outside the API auth
  middleware and stays unauthenticated. This exposes heap and goroutine dumps on a host the operator thinks is
  protected. → Wrap `mountDebug` routes with the same `middleware.Auth.Handler`, or document in docs/03 §8/§9 that
  `--debug` is unauthenticated.
  **Fixed**: `mountDebug` wraps every pprof route in `Auth.Handler` (removes the T-004 `TODO(T-007)`). Tests in
  `TestHandler` (no token → 401, bearer → 200). docs/03 §9 updated.
- [x] [minor] internal/api/middleware/auth.go:65 — Two spec gaps around trust in loopback/Host, with no current spec
  violation: (a) `--allow-guests` trusts every loopback peer, so a same-host reverse proxy turns every remote client
  into a guest; (b) DNS rebinding: when bound to loopback without a token, CrossOriginProtection and the WS origin
  check both pass for a rebinding origin, because Origin host == Host. → Lead decides and records in docs/03 §8,
  e.g. a Host allowlist (`localhost`, `127.0.0.1`, `[::1]`, configured listen host) when bound to loopback.
  Suggested follow-up task.
  **Decided** in docs/03 §8: no `X-Forwarded-For` trust, `--allow-guests` documented as incompatible with a same-host
  reverse proxy; Host allowlist (421) when bound to loopback. Implementation: new task T-027 (backend).
- [x] [minor] internal/api/graphql/schema/project.graphqls / docs/01 F-WSP-1 — The contract makes product decisions
  that are not in F-WSP-1: renaming saves a temp project, temp projects cannot be default, duplicates default to
  " (copy)", list order is most recently opened. The Handoff flags this (not silent), but T-006 will implement from
  the schema. → Lead copies these into docs/01 F-WSP-1 before T-006 starts. Track it as a task, not only a Handoff
  bullet.
  **Tracked**: new task T-026 (lead, docs/01 F-WSP-1). It must land before T-006; the operator should add it to
  T-006's `depends_on`.
- [x] [nit] internal/app/app_test.go:208-210 — The two new API cases sit between the comment "go tool pprof resolves
  addresses with POST …" and the pprof POST case it describes. → Move them above the comment.
  **Fixed**: the API cases sit after the pprof cases; the comment is next to its case.
- [x] [nit] internal/api/middleware/auth.go:69 — `CutPrefix(…, "Bearer ")` is case-sensitive, but the auth scheme
  is case-insensitive (RFC 9110 §11.1). → Split on the first space and compare the scheme with `strings.EqualFold`.
  **Fixed**: `bearerToken` splits on the first space and compares the scheme with `strings.EqualFold` (+ tests).
- [x] [nit] web/src/lib/components/StatusBar.svelte:8 — The default `start` uses the module singleton `connection`
  rather than the `conn` prop, so a caller passing only `conn` would start a different object. → `start = () =>
  conn.start(getApi())` (read inside the closure).
  **Fixed**: `start = () => conn.start(getApi())`.
- [x] [nit] internal/api/graphql/schema/project.graphqls:48 — `ProjectChange.kind` has no description; every other
  field does. → Add one ("What happened.").
  **Fixed**: "What happened."

**What's good**: clean split between `apperr` and the presenter, hand-written resolvers with contract stubs,
constant-time token compare, the WS 4401 close plus client `shouldRetry`, the synctest ping test, a thin API layer,
deterministic codegen with strict scalars on the TS side, and docs/03 §6/§8 and docs/06 updated with the decisions.

**Verdict: changes_requested.** Fix the two security/correctness majors (auth bypass, error classification) with
tests, and get the operator to amend `paths` for `server_test.go`. The minors can be fixed here or tracked as
follow-ups.

### Review round 2
Reviewer: claude-code (claude-opus-5-5), 2026-11-18, branch `task/T-007-graphql-scaffold` @ 5a984fa.

**Checks run**
- `make check`: green (go fix diff, vet, golangci-lint, `go test -race`, svelte-check 0 errors/0 warnings, eslint,
  prettier, vitest 7 files / 50 tests).
- `make gen` twice: exit 0, `git status` clean both times. AC 1 holds.
- `make test-int`: not run (no proxy/store/rawhttp changes).
- Manual probe: built binary with `--auth-token s3cret --debug`. `GET /api/rest/files/1`, `/api/nope` and
  `/debug/pprof/cmdline` with `Upgrade: websocket` all return 401 (round 1 gave 404). `GET /api/graphql/ws` without
  upgrade or credentials returns 401. `POST /api/graphql` with `Authorization: bearer s3cret` (lower-case scheme)
  returns `NOT_IMPLEMENTED` for `projects`.
- Paths: every changed file matches `paths`, including the round-1 additions (`internal/api/*_test.go`, docs/05,
  T-026, T-027).

**Round-1 findings re-verified**: all eleven are resolved as described above. The auth split (`Handler` strict,
`WebSocket` only on `GET /api/graphql/ws` with its own mux) is correct. The presenter now classifies by cause, and I
checked gqlgen v0.17.98: `AddErrorf`, `InvalidNullError` and resolver errors all reach the presenter as
`gqlerror.WrapPath` with `Err` set, so they become `INTERNAL`. Only `gqlerror.Errorf` messages about the request
become `BAD_INPUT`. `TestHandlerErrorCodes` exercises the real handler, the body cap returns 413, and pprof sits
behind `Auth.Handler`.

**AC coverage**: make gen deterministic ✔ (manual) · error mapping ✔ (`TestPresentError`, `TestHandlerErrorCodes`,
`TestErrorCodes`; span ✔) · CSRF + `CrossOriginProtection` ✔ (`TestCSRF`, `TestCrossOrigin`, `TestRequireAPIRequest`,
`TestWebSocketRejectsForeignOrigin`) · ping every second ✔ (synctest + `TestPingSubscription`) + status bar states ✔
(vitest) · project contract with descriptions ✔.

**Findings (round 2)**
- [ ] [minor] internal/apperr/apperr.go:61-65 — `New` copies the text of a `%w` cause into `Msg`, and `Msg` reaches
  clients for every code except `INTERNAL`. A future service writing `apperr.New(apperr.NotFound, "open project: %w",
  err)` around an `*fs.PathError` or SQLite error leaks paths or SQL. Round 1 closed that same leak for plain errors.
  The doc comment states this, but docs/03 §6 does not warn about it. → Add a sentence to docs/03 §6 ("messages of
  non-INTERNAL `apperr` errors are shown to clients; do not `%w` raw OS/SQL errors into them, set `Err` instead"), or
  add `apperr.Wrap(code, cause, format, args…)` that keeps the cause out of `Msg`. Either is fine here or in T-006.
- [ ] [nit] internal/apperr/apperr.go:64 — With two `%w` verbs, `fmt.Errorf` returns an `Unwrap() []error` value,
  so `errors.Unwrap` gives nil and both causes are dropped from `Err`. → Store `err` itself as `Err` when
  `errors.Unwrap(err) == nil` but `err` wraps more than one error, or document "at most one %w".
- [ ] [nit] internal/api/graphql/graphql.go:72-76 — The panic log has no stack trace, so a resolver panic in
  production cannot be located. The same panic is also logged twice: once here, and again by the presenter as
  "graphql internal error". → Add `"stack", string(debug.Stack())` to the recover log, and return an error the
  presenter recognises as already logged (e.g. a package-private sentinel cause) so it does not log again.
- [ ] [nit] internal/api/server_test.go:314 — Nothing exercises the `middleware.Authenticated(ctx)` branch of `wsInit`
  (graphql.go:85) end to end: a WebSocket that carries HTTP credentials (Bearer or `ratatoskr_token` cookie) but no
  `connection_init.token` must be acknowledged. Browsers will take this path after T-024. → Add a `TestAuthWebSocket`
  case: `websocket.DialOptions{HTTPHeader: {"Authorization": {"Bearer " + token}}}` with a nil payload →
  `connection_ack`.

**Verdict: approve.** No blocker or major findings remain. The minor and the nits can be fixed on this branch or
carried into T-006 (apperr usage) and T-024 (WS cookie test). Status stays `in_review`; the operator sets `done`
after merge.
