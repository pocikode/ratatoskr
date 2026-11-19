---
id: T-105
title: Proxy hooks - intercept hold, drop, Match & Replace rewrite, edited pairs
milestone: M2
role: backend
status: todo
depends_on: [T-100, T-031]
owner: ""
branch: task/T-105-proxy-intercept-mr-hooks
paths:
  - internal/proxy/**
spec:
  - docs/03-architecture.md §4 (pipeline, buffering rule, hook order)
  - docs/01-feature-spec.md#F-INT-1, #F-HIST-4, #F-MR-1 (M2 contracts)
  - docs/adr/0003-own-raw-http-engine.md
---

## Goal
The proxy calls Match & Replace and Intercept at the points fixed in docs/03 §4, sends drops, and stores edited pairs,
without changing the streaming path for traffic that neither feature touches.

## Scope
- In: two interfaces in `internal/proxy`, both optional in `proxy.Deps` (nil = feature off):
  - `Interceptor` — matches the hold API of `intercept.Service` (T-031): would-hold check per kind, hold request, hold response.
  - `Rewriter` — implemented by `matchreplace` (T-035) with exactly this shape:
    ```go
    type Rewriter interface {
        // Active reports whether an enabled rule exists for source and side (decides full buffering).
        Active(source string, response bool) bool
        // Request returns the rewritten request; the same slice when nothing changed.
        Request(ctx context.Context, source string, raw []byte) ([]byte, error)
        // Response returns the rewritten response; req is the request as sent upstream.
        Response(ctx context.Context, source string, req, resp []byte) ([]byte, error)
    }
    ```
  - Full buffering (≤ capture cap) only when one of them is active for that side; over the cap → stream unchanged.
  - Order: M&R `proxy` → hold → after a Forward that changed bytes, M&R `intercept` → send upstream; same on the response side.
  - Drop request / response per F-INT-1 (status from the hold decision, our own short HTML body, `isDropped`).
  - Edited pair per F-HIST-4 via the T-100 capture API (source `intercept` when the user's edit changed bytes).
  - Destination always from the original connection, never from edited `Host`/target.
- Out: the M&R engine, intercept service, app wiring (T-109).

## Acceptance criteria
- [ ] Integration (testutil raw server, fake Interceptor): request held → client blocks; Forward with edits → server gets
      the edited bytes; history has the original (client bytes, server response) and the edited record (`editedFrom`,
      source `intercept`, response as delivered).
- [ ] Drop request → client gets the configured status, the server sees nothing, record `isDropped`.
- [ ] Drop response → client gets the configured status, record keeps the server response and `isDropped`.
- [ ] Response held and edited → client gets the edited response; pair stored with identical request bytes.
- [ ] Editing `Host` in a held request does not change where it is sent.
- [ ] Body over the capture cap with an active Interceptor/Rewriter passes through unheld and unchanged (test with a small cap).
- [ ] Fake Rewriter adding a header → server sees it, pair stored with the original's source; `intercept` rules run only
      after a Forward that changed bytes.
- [ ] With both deps nil, existing proxy tests pass unchanged and the proxy benchmark stays within 5% of `main` (numbers in Handoff).
- [ ] `make check` and `make test-int` green.

## Notes from lead
- Plan mode required. Byte fidelity: no `net/http` on this path (ADR-0003); unchanged messages must go out as the same bytes.
- Edits to a held request may change its framing; re-read the edited bytes with `rawhttp` before writing them upstream.

## Plan (implementer, ≤15 lines)

## Out-of-scope change needed (if any)

## Handoff (implementer)
- What changed:
- How to verify:
- Known limitations:
- Follow-ups (proposed tasks):

## Review (reviewer)
- Verdict: approve | changes_requested
- Findings:
