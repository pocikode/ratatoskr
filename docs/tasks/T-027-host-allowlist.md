---
id: T-027
title: DNS rebinding defence (Host allowlist on loopback) and --allow-guests reverse-proxy warning
milestone: M0
role: backend
status: todo
depends_on: [T-007]
owner: ""
branch: task/T-027-host-allowlist
paths:
  - internal/api/middleware/host*.go
  - internal/api/server.go
  - internal/api/*_test.go
  - internal/app/app.go
  - internal/app/app_test.go
  - internal/config/config.go
  - internal/config/config_test.go
spec:
  - docs/03-architecture.md §8
---

## Goal
Close the two trust gaps found in the T-007 review, as decided in docs/03 §8: a page on a rebinding domain must not
reach the API of an instance bound to loopback, and `--allow-guests` must say that it trusts every loopback peer.

## Scope
- In: Host check middleware, its wiring for `/api/**` and `/debug/pprof/**`, `--allow-guests` help text.
- Out: `X-Forwarded-*` support (not trusted), TLS for the UI.

## Acceptance criteria
- [ ] When the UI listener is bound to a loopback address, a request whose `Host` (port stripped, case-insensitive) is
      not `localhost`, a loopback IP literal (`127.0.0.0/8`, `[::1]`) or the configured listen host gets 421 before
      CSRF/auth. Applies to `/api/**` (HTTP and the WS upgrade) and `/debug/pprof/**`; the SPA static files may stay open.
- [ ] When bound to a non-loopback address the check is off (the token protects the instance; warning of docs/03 §8).
- [ ] Tests: `Host: evil.example:8080` on a loopback-bound handler → 421 for POST `/api/graphql` and the WS endpoint;
      `localhost:8080`, `127.0.0.1:8080`, `[::1]:8080` → pass.
- [ ] `--allow-guests` help text says guests are decided by the TCP peer and that it must not be combined with a
      same-host reverse proxy.
- [ ] `make check` green.

## Notes from lead
- The listen address is known in `internal/app`; pass the allowed hosts into `api.Options`, keep the middleware pure.

## Plan (implementer, ≤15 lines)

## Out-of-scope change needed (if any)

## Handoff (implementer)

## Review (reviewer)
