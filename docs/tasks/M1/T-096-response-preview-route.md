---
id: T-096
title: Serve sandboxed response preview at GET /api/rest/preview/{id} (follow-up of T-017, unblocks T-021)
milestone: M1
role: backend
status: done
depends_on: [T-016]
owner: Codex (GPT-6)
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
- [x] `GET /api/rest/preview/{id}` answers 200 with the decoded response body (chunked framing and gzip/deflate/br/zstd
      removed), up to 4 MiB, exactly per the F-HIST-2 Preview contract.
- [x] `Content-Type` comes from the stored response head (`application/octet-stream` when absent). No other stored header
      (for example `Set-Cookie`, `Content-Security-Policy`, `Location`) reaches the client.
- [x] Every answer, including 404, carries `Content-Security-Policy: sandbox`, `X-Content-Type-Options: nosniff`,
      `Cache-Control: no-store`, `Referrer-Policy: no-referrer`.
- [x] 404 for a non-numeric id, an unknown id, a request without a response, or no open project. Only GET/HEAD are allowed.
- [x] Route sits behind the existing `/api/rest/` auth + CSRF chain (with `--auth-token`, no credentials → 401;
      `ratatoskr_token` cookie works).
- [x] Tests: gzip body decoded; chunked body unframed; stored `Set-Cookie` dropped; the four headers present on 200 and 404;
      a > 4 MiB body truncated to 4 MiB; auth 401.
- [x] `make check` green.

## Notes from lead
- Use `history.Service.Head(ctx, id, history.PartResponse)` and `Body(ctx, id, history.PartResponse, 0, history.MaxBodyChunk, true)`.
  Do not add a decoder; `Body` already handles coding and chunking. Parse the head with `internal/rawhttp`, not `net/http`.
- Map `apperr` NotFound **and Conflict** → 404; any other error → 500 with a generic text body (log the cause at warn).
  `Head`/`Body` return Conflict only for "no project is open" (`history.Service.db`), so no history change is needed
  (lead, 2026-11-19, answers the blocked question below).
- Add a `Preview` field to `rest.Deps` (a small interface with the two methods, nil leaves the route 404, like `CA`).
- Request IDs are the decimal row ids the GraphQL layer emits (`strconv`).

## Plan (implementer, ≤15 lines)

- Resolve how the preview route should classify the history service's no-project error.
- Mount the sandboxed preview route through the existing REST middleware and wire in history.
- Test the preview contract and auth, then run `make check`.

## Out-of-scope change needed (if any)

- `history.Service.Head` and `Body` return `apperr.Conflict` when no project is open
  (`internal/history/service.go:89`). The acceptance criteria require 404 in this case.
  The lead notes require 500 for errors with any code other than `apperr.NotFound`.
  Should preview map `apperr.Conflict` from these methods to 404, or should the
  history service expose a distinct no-project error for the route to handle?
  - **Lead answer:** map Conflict → 404 in the route; history stays as is. Spec F-HIST-2 updated.

## Handoff (implementer)
- Added `GET` and `HEAD /api/rest/preview/{id}` using the existing history decoder. The route caps the body at 4 MiB and copies only the stored `Content-Type`.
- The four preview headers also apply to auth and CSRF rejections. Missing responses and no open project return 404. Unexpected errors return generic text and log a warning.
- `make check` passed. Run `go test ./internal/api -run '^TestResponsePreview'` for the HTTP tests against SQLite.
- The history service can fall back to stored bytes for undecodable content. Its separate head and body reads can see different projects if the active project changes between them.
- T-097 covers the viewer and browser integration.

## Review (reviewer)
- Verdict: approve
- Checked: diff is inside `paths`; `make check` green (Go race tests + web check/unit); no proxy/store/rawhttp change, so no
  `test-int`. Every AC has a test (gzip + 4 MiB cap, chunked, Set-Cookie/Location/CSP dropped, four headers on 200/404/401/403/405,
  no project → 404, nil dep → 404, HEAD). Stored heads parse with `rawhttp.DefaultLimits`, the same limits the proxy captures with,
  so the 500 on an unparsable head is reachable only with corrupt rows. `a.history` is set before `buildHandler`, so no nil-interface trap.
- Findings:
  - [ ] [minor] docs/01-feature-spec.md:160 — the lead's F-HIST-2 edit ("no open project → 404") is uncommitted in this worktree and
        not on the branch, yet the code relies on it → lead commits it (own PR or onto this branch) before merge.
  - [ ] [nit] internal/api/rest/router.go:31 — `WithPreviewHeaders(mux)` is redundant: `api.NewHandler` already wraps the whole
        `/api/` chain with it (internal/api/server.go:61), which is the only one that also covers 401/403. → return `mux` and keep
        the single outer wrap (-1 line, one place to change the header set).
  - [ ] [nit] internal/api/rest/preview.go:55-73 — `Head` and `Body` are two reads; a project switch between them can mix a head
        from one project with a body from another (already noted in Handoff). Harmless for a sandboxed preview; leave as is.
