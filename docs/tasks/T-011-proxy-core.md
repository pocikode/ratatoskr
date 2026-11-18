---
id: T-011
title: Proxy core - listener multiplexing, CONNECT, MITM, forwarding, capture
milestone: M1
role: backend
status: todo
depends_on: [T-005, T-008, T-010]
owner: ""
branch: task/T-011-proxy-core
paths:
  - internal/proxy/**
  - internal/netx/**
  - internal/history/capture*.go
  - internal/events/**
  - internal/testutil/**
  - internal/app/**
spec:
  - docs/01-feature-spec.md#F-NET-1, #F-NET-2, #F-NET-3
  - docs/03-architecture.md §4, §7
---

## Goal
The :8080 listener routes proxy traffic vs UI traffic, MITMs TLS using `internal/ca`, forwards through `rawhttp` with a keep-alive pool,
and captures every request/response into the project DB (batched) emitting `RequestAdded/RequestUpdated` events.

## Notes from lead
- (2026-11-18, T-006) `app.New` is now `app.New(ctx, opts)`. It opens instance.db plus `project.Service` (`ctx` is wrapped in `context.WithoutCancel`). Rebase on T-006 and keep that signature.
- T-006 also edits `internal/app/app.go` (project service wiring). Start from a `main` that includes T-006, or rebase onto it before review.
- (2026-11-18, T-008) `internal/app/app.go` already opens `ca.Open(...)` and mounts `/ca.crt` + `/ca.der` on the UI listener: rebase, do not re-add it. Call `ca.Service.TLSConfig()` for MITM. F-NET-2's `http://ratatoskr.localhost/ca.crt` arrives through the proxy listener, so answer it there too (and let `ratatoskr.localhost` pass the loopback Host check).

## Acceptance criteria
- [ ] `curl -x` HTTP and HTTPS (with `--cacert`) through the proxy return identical bodies to direct requests (integration test against testutil servers).
- [ ] Malformed requests from the golden corpus are forwarded byte-exact to a raw TCP test server.
- [ ] 200 concurrent clients × 50 requests: no goroutine leak (goroutineleak profile/test), p99 added latency < 5 ms locally.
- [ ] Upstream errors produce a 502 page from Ratatoskr and a stored record with error text.
- [ ] Capture can be paused when no project is open (requests still forwarded).

## Plan (implementer, ≤15 lines)

## Out-of-scope change needed (if any)

## Handoff (implementer)

## Review (reviewer)
