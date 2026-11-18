---
id: T-024
title: Login/logout endpoints that set the auth cookie
milestone: M0
role: backend
status: done
depends_on: [T-007]
owner: "DeepSeek V4.1 Flash 1017 (sinarcloud/deepseek-v4.1-flash-1017)"
branch: task/T-024-auth-login-endpoint
paths:
  - internal/api/rest/auth*.go
  - internal/api/rest/router.go
  - internal/api/middleware/auth*.go
  - docs/03-architecture.md   # §8 only; added by lead (AC 2 requires it)
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

## Rework (implementer)
- Major: `loginLimiter.attempt(ip)` reserves under one lock, after the body is parsed; `blocked`/`failed` removed.
  Concurrent test added. Minor 1: test pins the 500; docs/03 §8 lists cookie-safe bytes; config `Validate` follow-up
  proposed (outside paths). Minor 2: auth off returns 401 before limiter/log. Nit docs: lead's call. IPv6 /64 nit skipped.

## Lead decisions (2026-11-18)
- Docs nit: accepted. `docs/03-architecture.md` is now in `paths`. TEMPLATE now says that a doc an AC requires goes in `paths`.
- IPv6 /64 nit: **won't fix**. The spec says "per remote IP". The default listener is loopback, and the token's
  entropy is the real defence. A determined attacker gets around any per-address limit anyway, through the capped map
  or through many IPv4 addresses. Keep the key at /128. Reviewer: tick it as resolved.
- Cookie-unsafe token minor: follow-up **T-029** (config `Validate` rejects such tokens). The 500 that is pinned here stays
  until T-029 lands.
- Login page: T-025 is reserved for it. I will write that task file when M0 finishes.

## Review (reviewer)
Reviewed `ad1cfc2` against `main`. Files: `internal/api/middleware/auth*.go` (in `paths`), `docs/03-architecture.md`
(outside `paths`, but the AC requires it; see the nit below). `make check`: green (exit 0). `make test-int` not needed:
proxy, store and rawhttp are untouched. Hard rules 3–8 and 10–11 are met. AC 1, 2, 4 and 5 are met and tested. AC 3 is
only partly met (finding 1).

- [x] [major] internal/api/middleware/auth_login.go:70-98 — the rate limit can be bypassed by a check-then-act race.
      `blocked(ip)` runs **before** the body is read, and `failed(ip)` runs only after the read and the compare, in a
      separate lock. An attacker opens N connections and sends only the headers on each one, so all N pass `blocked`
      while the count is still 0. Then the attacker sends the N bodies. Result: N guesses per window instead of 5, and
      net/http does not cap N. → Make reserving an attempt atomic, and do it after the body is parsed: one
      `l.attempt(ip) bool` that, under the lock, resets an expired entry, returns false if `count >= limit`, and otherwise
      increments `count` (counting attempts, not failures). Keep `succeeded(ip)` to clear the entry on a match.
      `blocked` and `failed` then go away. Add a test: start `2*loginLimit` goroutines (`sync.WaitGroup.Go`) that call
      `attempt` concurrently, and assert exactly `loginLimit` of them return true.
- [x] [minor] internal/api/middleware/auth_login.go:105-112 — an `--auth-token` that holds `"`, `;`, `\`, a control
      byte or non-ASCII passes `CheckToken`. It then fails `c.Valid()` and returns 500 on every login, so a browser user
      can never log in, while Bearer still works. Nothing in `internal/config` rejects such tokens. → Write a follow-up
      for config `Validate` to reject tokens with bytes that a cookie cannot carry (it is outside `paths`), and say in
      docs/03 §8 which bytes are allowed. Here, a test with such a token pins the 500.
- [x] [minor] internal/api/middleware/auth_login.go:97-99 — when auth is off (`Token == ""`), every login call logs
      `WARN login failed` and fills the shared limiter, though there is no token to guess. A UI that probes login
      (T-025) would spam the log. → Return 401 before you touch the limiter or the logger when `!a.Required()`. The
      existing `TestLoginAuthDisabled` still holds.
- [x] [nit] docs/03-architecture.md — this file is outside `paths`. The AC asks for the change and the Handoff records
      it, so I accept it. → Lead: add `docs/03-architecture.md` to `paths` in tasks whose AC requires a doc change.
- [x] [nit] internal/api/middleware/auth_login.go:143-152 (won't fix, lead decision above) — the IPv6 key is the full /128 address, and one host often
      controls a whole /64. That gives 5 tries per address and lets the host evict its own blocked entries through
      `makeRoom`. The spec says "per remote IP", so this is not a defect. → Optional: key IPv6 peers by
      `netip.PrefixFrom(addr, 64)`.

Checked and fine: the exemption is exact-path and POST-only (tests cover a trailing slash, case and siblings). CSRF:
`RequireAPIRequest` runs inside, and `CrossOrigin` wraps it in server.go. The body is capped at 4 KiB, and 413 is
told apart with `errors.AsType`. Cookie attributes and `Secure` iff TLS. The log never holds the token (test). The
limiter map is bounded, and the mutex is held with no I/O. `Auth.Logger` stays nil in app.go and falls back to
`slog.Default`, which main.go sets. pprof routes still go through `Handler` and are not affected.

Verdict: **changes_requested** (fix the major finding; the minors should be fixed or written up as follow-ups).

### Re-review (round 2)
Reviewed `1e97af8` against `main`. Code files: `internal/api/middleware/auth*.go` and `docs/03-architecture.md`, all
in `paths`. `docs/08-roadmap.md`, `docs/tasks/TEMPLATE.md` and `docs/tasks/T-029-*.md` changed in the lead's commit
`4d5384d`. That is the lead's lane, so it is fine. `make check`: green (exit 0). `go test -race -count=5
./internal/api/middleware/`: ok. `make test-int` not needed: proxy, store and rawhttp are untouched.

All findings from round 1 are resolved. The major one: `attempt` checks, resets and increments under one lock, after
the body is parsed (`auth_login.go:96-103`). `TestLoginLimiterConcurrent` covers it. The minors: auth off answers 401
before the limiter and the log (`TestLoginAuthDisabledQuiet`). The 500 for a token that a cookie cannot carry is pinned
by a test and handed to T-029. I re-checked `makeRoom` with the new reset path. An expired entry for the same IP is
swept, then created again, so the map stays bounded. A reset never evicts a live entry by mistake.

- [ ] [nit] docs/tasks/T-024-auth-login-endpoint.md:64 — the Handoff still says "5 failed logins/min … (429, then the
      window slides)". The limiter now counts attempts, and the window is fixed: it starts at the first attempt. →
      Change it to "5 login attempts/min per TCP peer IP (fixed window from the first attempt; 429 after that)".
- [ ] [nit] internal/api/middleware/auth_login.go:185 — the map is still called `failures`, but it now counts all
      attempts. A reader may think successful attempts are not counted. → Rename it to `attempts`. That is three
      lines in the code and two in the tests.

Verdict: **approve** (only nits are left; fixing them is optional). The status stays `in_review`, and the operator
sets `done` after the merge.
