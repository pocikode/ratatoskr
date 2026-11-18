---
id: T-024
title: Login/logout endpoints that set the auth cookie
milestone: M0
role: backend
status: in_review
depends_on: [T-007]
owner: "DeepSeek V4.1 Flash 1017 (sinarcloud/deepseek-v4.1-flash-1017)"
branch: task/T-024-auth-login-endpoint
paths:
  - internal/api/rest/auth*.go
  - internal/api/rest/router.go
  - internal/api/middleware/auth*.go
spec:
  - docs/01-feature-spec.md#F-WSP-4
  - docs/03-architecture.md §8
---

## Goal
With `--auth-token`, a browser can log in: `POST /api/rest/auth/login` with the token sets the `ratatoskr_token` cookie
that T-007's auth middleware already accepts, and `POST /api/rest/auth/logout` clears it.

## Scope
- In: the two endpoints, cookie attributes, rate limiting of failed logins, tests.
- Out: the login page (proposed follow-up in T-007 Handoff), token rotation from Settings.

## Acceptance criteria
- [x] `POST /api/rest/auth/login` (JSON `{"token": "…"}`, `X-Ratatoskr: 1`) → 204 + cookie on success, 401 otherwise;
      it is reachable without being authenticated (the auth middleware exempts exactly these two paths).
- [x] Cookie: `HttpOnly`, `SameSite=Strict`, `Path=/`, `Secure` when the request came over TLS; value is the token
      (or an opaque session id — decide and document in docs/03 §8).
- [x] Failed logins are rate limited per remote IP (e.g. 5/min) and logged at warn without the submitted value.
- [x] Logout clears the cookie (204).
- [x] `make check` green.

## Notes from lead
- Compare tokens with `middleware.Auth.CheckToken` (constant time). CSRF rules of docs/03 §8 apply to both endpoints.

## Plan (implementer, ≤15 lines)
- Serve both endpoints from `middleware.Auth.Handler`: it is the only place with the token in scope and
  `internal/api/server.go` is outside `paths`, so `rest.Deps` cannot receive the auth config.
  `Handler` dispatches exactly `POST /api/rest/auth/login|logout` before `allows()`; every other path keeps 401.
- Both go through `RequireAPIRequest` (JSON + `X-Ratatoskr: 1`); `CrossOrigin` in `server.go` already wraps them.
- Cookie value = the token: one token per process, compared in constant time, no session table to store,
  expire or leak. `HttpOnly`, `SameSite=Strict`, `Path=/`, `Secure` iff `r.TLS != nil`. Docs: docs/03 §8.
- Login: 204 + cookie, 401 bad token/malformed body, 413 oversized body; logout: 204 + expired cookie.
- `loginLimiter`: fixed window, 5 failures/min per remote IP (no X-Forwarded-For), capped map (1024 IPs),
  `slog` warn with the IP and never the submitted value; 429 once spent, cleared by a successful login.
- Stdlib only: `encoding/json/v2`, `errors.AsType`, `sync.Mutex`, `testing/synctest`.
- Tests in `middleware`: table-driven endpoint/CSRF/cookie tests, synctest rate-limit test, map-bound test,
  narrow-exemption test, log-does-not-leak-token test.

## Out-of-scope change needed (if any)
- docs/03-architecture.md §8 gets the cookie/rate-limit sentence the AC asks for (only non-code path).
- Proposed follow-up: a login page (T-025) must POST JSON, not a form, and read `serverInfo.authRequired`.

## Handoff (implementer)
- What changed:
  - `internal/api/middleware/auth_login.go` (new): `Auth.serveSession` dispatches `POST /api/rest/auth/login` and
    `POST /api/rest/auth/logout`; both pass `RequireAPIRequest` (JSON + `X-Ratatoskr: 1`) and are the only paths that
    skip `allows()`. Login: 204 + cookie, 401 wrong token (or auth disabled), 400 bad JSON, 413 > 4 KiB body.
    Logout: 204 + expired cookie. Cookie value is the token; attributes `HttpOnly`, `SameSite=Strict`, `Path=/`,
    `Secure` iff `r.TLS != nil`. `loginLimiter`: fixed window, 5 failed logins/min per TCP peer IP (429, then the
    window slides), capped at 1024 tracked IPs, cleared by a successful login; failures log
    `Warn("login failed", "component", "auth", "remote_ip", …)` — never the submitted value.
  - `internal/api/middleware/auth.go`: two fields on `Auth` (`Logger *slog.Logger`, unexported `loginLimiter`) and the
    `serveSession` call in `Handler`.
  - `internal/api/middleware/auth_login_test.go` (new): table-driven endpoint tests (status, cookie attributes incl.
    TLS, CSRF header/content-type, narrow exemption, no-credentials, body cap), `synctest` rate-limit tests (block,
    window expiry, per IP, bounded map), and a log test asserting the token never appears.
  - `docs/03-architecture.md` §8: cookie decision and the rate limit (see "Out-of-scope change needed").
- Design decision the paths forced: `internal/api/server.go` is outside `paths`, so `rest.Deps` cannot receive the
  token. The endpoints are therefore served by `middleware.Auth.Handler`, the only place holding it; `rest/router.go`
  is untouched and still 404s those paths if reached directly (it never is).
- How to verify: `make check`. Manually: `./ratatoskr --data-dir ./.dev-data --listen 127.0.0.1:8080 --auth-token s3cret`,
  then `curl -i -XPOST localhost:8080/api/rest/auth/login -H 'Content-Type: application/json' -H 'X-Ratatoskr: 1'
  -d '{"token":"s3cret"}' -c jar` → 204 + `Set-Cookie: ratatoskr_token=s3cret; Path=/; HttpOnly; SameSite=Strict`;
  `-b jar` on `/api/graphql` → 200; 6 wrong tokens in a minute → 429; `POST /api/rest/auth/logout` → 204 +
  `Max-Age=0`.
- Limitations / notes for T-025: the login page must POST JSON with the `X-Ratatoskr: 1` header (a form post gets 415).
  `Secure` follows `r.TLS`, so a TLS-terminating proxy in front of a plain HTTP listener yields a non-Secure cookie
  (documented in docs/03 §8). Failures live in memory: a restart forgets them, and the map evicts the earliest-expiring
  live IP when all 1024 slots are busy.
- Follow-ups (proposed): login page + "log out" UI (T-025); optional `Retry-After` on 429; a browser E2E once T-025 lands.

## Review (reviewer)
