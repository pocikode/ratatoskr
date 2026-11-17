---
id: T-024
title: Login/logout endpoints that set the auth cookie
milestone: M0
role: backend
status: todo
depends_on: [T-007]
owner: ""
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
- [ ] `POST /api/rest/auth/login` (JSON `{"token": "…"}`, `X-Ratatoskr: 1`) → 204 + cookie on success, 401 otherwise;
      it is reachable without being authenticated (the auth middleware exempts exactly these two paths).
- [ ] Cookie: `HttpOnly`, `SameSite=Strict`, `Path=/`, `Secure` when the request came over TLS; value is the token
      (or an opaque session id — decide and document in docs/03 §8).
- [ ] Failed logins are rate limited per remote IP (e.g. 5/min) and logged at warn without the submitted value.
- [ ] Logout clears the cookie (204).
- [ ] `make check` green.

## Notes from lead
- Compare tokens with `middleware.Auth.CheckToken` (constant time). CSRF rules of docs/03 §8 apply to both endpoints.

## Plan (implementer, ≤15 lines)

## Out-of-scope change needed (if any)

## Handoff (implementer)

## Review (reviewer)
