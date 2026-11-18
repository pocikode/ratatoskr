---
id: T-027
title: DNS rebinding defence (Host allowlist on loopback) and --allow-guests reverse-proxy warning
milestone: M0
role: backend
status: changes_requested
depends_on: [T-007]
owner: opencode/deepseek-v4.1-flash-1017
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
- [x] When the UI listener is bound to a loopback address, a request whose `Host` (port stripped, case-insensitive) is
      not `localhost`, a loopback IP literal (`127.0.0.0/8`, `[::1]`) or the configured listen host gets 421 before
      CSRF/auth. Applies to `/api/**` (HTTP and the WS upgrade) and `/debug/pprof/**`; the SPA static files may stay open.
- [x] When bound to a non-loopback address the check is off (the token protects the instance; warning of docs/03 §8).
- [x] Tests: `Host: evil.example:8080` on a loopback-bound handler → 421 for POST `/api/graphql` and the WS endpoint;
      `localhost:8080`, `127.0.0.1:8080`, `[::1]:8080` → pass.
- [x] `--allow-guests` help text says guests are decided by the TCP peer and that it must not be combined with a
      same-host reverse proxy.
- [x] `make check` green.

## Notes from lead
- (2026-11-18, T-006) `app.New` is now `app.New(ctx, opts)`. It opens instance.db plus `project.Service` (`ctx` is wrapped in `context.WithoutCancel`). Rebase on T-006 and keep that signature.
- T-006 also edits `internal/app/app.go` (project service wiring). Start from a `main` that includes T-006, or rebase onto it before review.
- The listen address is known in `internal/app`; pass the allowed hosts into `api.Options`, keep the middleware pure.

## Plan (implementer, ≤15 lines)
1. `middleware/host.go`: `Hosts(allowed []string, next http.Handler)`; 421 unless Host (port stripped, case-folded)
   is `localhost`, a loopback IP literal (netip, 127/8 and ::1) or one of `allowed`; empty `allowed` = check off.
2. `app.go`: `hostAllowlist()` = nil when `cfg.Listen[0]` is not loopback, else {listen addr, proxy.UIHost}
   (`ratatoskr.localhost` must keep working, T-008 handoff). Wrap the UI mux in `buildHandler`, so `/api/**`,
   the WS upgrade and `/debug/pprof/**` are guarded before CSRF/auth. `/debug/pprof` is mounted on that same
   mux (not under `/api`), so one wrap covers both; static files are guarded too, which docs/03 §8 allows.
3. `config.go`: `--allow-guests` help = loopback TCP peer decides it, never with a same-host reverse proxy.
4. Tests: middleware allow/deny/off table; app handler table (evil Host → 421 on graphql, WS, pprof;
   `localhost`, `127.0.0.1`, `[::1]`, `ratatoskr.localhost` pass; non-loopback listen off); usage text.
5. Existing app tests that call `a.handler` get `Host: localhost` (the guard is on for a 127.0.0.1 listen).

## Out-of-scope change needed (if any)

## Handoff (implementer)
### What changed
- `internal/api/middleware/host.go`: pure `Hosts(allowed []string, next http.Handler)`. Rejects with 421
  (`http.StatusMisdirectedRequest`) unless the Host (port stripped, lower-cased, IPv6 brackets trimmed) is `localhost`,
  a `netip` loopback literal (127/8, `::1`, unmapped `::ffff:127.0.0.1`) or one of `allowed`. Empty `allowed` = off.
  `allowed` entries may be `host:port`; only the host is kept.
- `internal/app/app.go`: `hostAllowlist()` returns nil unless `cfg.Listen[0]` is loopback; otherwise the configured
  listen address plus `proxy.UIHost` (`ratatoskr.localhost` — F-NET-2's CA download and the T-008 handoff require it).
  `buildHandler` returns `middleware.Hosts(hostAllowlist(), mux)`, so the check is the outermost handler: 421 before
  `CrossOrigin`, auth and body limits, on `/api/**`, the WS upgrade, `/debug/pprof/**` and the SPA.
- `internal/config/config.go`: `--allow-guests` says guests are decided by the loopback TCP peer and that it must not be
  combined with a same-host reverse proxy.
- Tests: `TestHosts` (allow/deny/off table incl. `127.0.0.2`, mapped v6, bracketless v6, empty Host, lookalike hosts),
  `TestHandlerHostAllowlist` (evil Host → 421 on graphql/WS/pprof/static; localhost, 127.0.0.1, `[::1]`,
  ratatoskr.localhost pass; the app has `--auth-token`, so the 421 also proves it runs before auth),
  `TestHandlerHostAllowlistOffWhenExposed` (0.0.0.0 listen), `TestUsage` (new wording). `TestHandler` and
  `TestProjectsAreWired` now send `Host: localhost`, since the guard is on for their 127.0.0.1 listen.

### Deviation from the lead note
The note suggested `api.Options.AllowedHosts`. `/debug/pprof` is mounted on the app mux, not under `api.NewHandler`, so
one wrap in `buildHandler` covers both without duplicating the middleware (or editing `pprof.go`, which is outside my
`paths`). `api.NewHandler` is unchanged and stays unguarded on its own; every mount of it goes through `app`.

### How to verify
`make check` (green here; a fresh worktree needs `cd web && bun install --frozen-lockfile` first).
Manual, with `--listen 127.0.0.1:8080 --debug`:
- `curl -si -x http://127.0.0.1:8080 http://ratatoskr.localhost/api/graphql -H 'Host: evil.example'` → the guard's 421
  ("unexpected Host header"). The shared listener's dispatch already answers a *foreign origin-form* Host with its own
  421 (T-011); the guard is what protects the UI handler itself, e.g. once `--ui-listen` serves it directly.
- `curl -si -X POST http://127.0.0.1:8080/api/graphql` → 415 (guard passed, CSRF next).
- `curl -si -x http://127.0.0.1:8080 http://ratatoskr.localhost/ca.crt` → 200 (F-NET-2 still works).

### Limitations / follow-ups
- Enabled from the *configured* listen address: `--listen` on a hostname other than `localhost` that resolves to loopback
  leaves the check off (it is treated as a network listener).
- `--ui-listen` is still rejected by `app.New` (T-011 multiplexer). When it lands, its address must join the allowlist too.
- The SPA and `/ca.crt` are guarded as well (docs/03 §8 allows the SPA to stay open; guarding it is harmless). A custom
  `/etc/hosts` alias for a loopback instance now gets 421 — use `localhost`, a loopback IP or `ratatoskr.localhost`.

## Review (reviewer)
Reviewer: claude-code/opus-5.5 · 2026-11-18 · verdict: **changes_requested**

Checks: `make check` green (go vet, golangci-lint, `go test -race`, web check + vitest). `make test-int` not run: no
proxy/store/rawhttp change. Every AC is met and tested; the guard's logic is correct. The only reason this is not an
approve is hard rule 1, and fixing it means moving one file.

- [ ] [major] internal/api/middleware/middleware_test.go:97 — `TestHosts` is added to a file outside the task `paths`
      (`internal/api/*_test.go` does not match the `middleware/` subdirectory). Hard rule 1. → Move `TestHosts` into a new
      `internal/api/middleware/host_test.go`; it matches the existing `host*.go` path and can reuse `okHandler` from the
      same package. Another fix: the lead adds the file to `paths`.
- [ ] [minor] internal/app/app.go:328 — `ratatoskr.localhost` is allowed, but docs/03 §8 only lists `localhost`, loopback
      literals and the configured listen host. The choice is right: F-NET-2's `/ca.crt` through the proxy needs it, and
      `.localhost` cannot be rebound (RFC 6761). But the code goes beyond the spec. → Lead: add "and `ratatoskr.localhost`
      (F-NET-2)" to the DNS-rebinding bullet in docs/03 §8. No code change.
- [ ] [nit] internal/app/app.go:372 vs internal/api/middleware/host.go:57 — `isLoopback` does not `Unmap`, but the guard
      does. So `--listen [::ffff:127.0.0.1]:8080` turns the guard off (and `warnIfExposed` fires wrongly). This is a rare
      edge and fails open only to the documented token-protected mode. → `ip.Unmap().IsLoopback()` in `isLoopback`, if
      you touch the file anyway.

Verified and fine: the guard is outermost (421 before `CrossOrigin`/auth; the app test uses `--auth-token`, which proves it).
Case folding, port stripping, bracketed and bare IPv6, mapped v4, empty Host and lookalike hosts (`127.0.0.1.evil…`,
`localhost.evil…`) are all handled and tested. The check is off for a non-loopback listen. `Listen[0]` alone is safe
because `app.New` rejects multiple listeners and `--ui-listen` (handoff records the follow-up). The shared proxy listener
already 421s foreign origin-form Hosts (`proxy/server.go:376`), so this guard is defence in depth there. The handoff
explains the deviation from `api.Options` (one wrap covers `/debug/pprof`) well, and I accept it.
