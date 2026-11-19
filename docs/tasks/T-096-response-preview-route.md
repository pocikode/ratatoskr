---
id: T-096
title: Serve sandboxed response preview at GET /api/rest/preview/{id} (follow-up of T-017, unblocks T-021)
milestone: M1
role: backend
status: todo
depends_on: [T-016]
owner: ""
branch: task/T-096-response-preview-route
paths:
  - internal/api/rest/**
  - internal/api/server.go
  - internal/api/server_test.go
  - internal/app/**          # only to pass the history service into api.Options
spec:
  - docs/01-feature-spec.md#F-HIST-2 (Preview contract paragraph)
  - docs/03-architecture.md §6 (REST), §8
---

## Goal
A stored response can be opened as a page, under the UI origin, with no scripts and no access to the API.

## Scope
- In: the REST route, its wiring from the history service, tests.
- Out: UI (T-097), GraphQL schema changes (none needed), request-side preview, bodies over 4 MiB.

## Acceptance criteria
- [ ] `GET /api/rest/preview/{id}` answers 200 with the decoded response body (chunked framing and gzip/deflate/br/zstd
      removed), up to 4 MiB, exactly per the F-HIST-2 Preview contract.
- [ ] `Content-Type` comes from the stored response head (`application/octet-stream` when absent). No other stored header
      (for example `Set-Cookie`, `Content-Security-Policy`, `Location`) reaches the client.
- [ ] Every answer, including 404, carries `Content-Security-Policy: sandbox`, `X-Content-Type-Options: nosniff`,
      `Cache-Control: no-store`, `Referrer-Policy: no-referrer`.
- [ ] 404 for a non-numeric id, an unknown id, a request without a response, or no open project. Only GET/HEAD are allowed.
- [ ] Route sits behind the existing `/api/rest/` auth + CSRF chain (with `--auth-token`, no credentials → 401;
      `ratatoskr_token` cookie works).
- [ ] Tests: gzip body decoded; chunked body unframed; stored `Set-Cookie` dropped; the four headers present on 200 and 404;
      a > 4 MiB body truncated to 4 MiB; auth 401.
- [ ] `make check` green.

## Notes from lead
- Use `history.Service.Head(ctx, id, history.PartResponse)` and `Body(ctx, id, history.PartResponse, 0, history.MaxBodyChunk, true)`.
  Do not add a decoder; `Body` already handles coding and chunking. Parse the head with `internal/rawhttp`, not `net/http`.
- Map `apperr` NotFound → 404; any other error → 500 with a generic text body (log the cause at warn).
- Add a `Preview` field to `rest.Deps` (a small interface with the two methods, nil leaves the route 404, like `CA`).
- Request IDs are the decimal row ids the GraphQL layer emits (`strconv`).

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
