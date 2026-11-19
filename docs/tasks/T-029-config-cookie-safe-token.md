---
id: T-029
title: Reject auth tokens a cookie cannot carry
milestone: M0
role: backend
status: in_review
depends_on: [T-024]
owner: "claude-code/opus (lead, operator-approved)"
branch: task/T-029-config-cookie-safe-token
paths:
  - internal/config/config.go
  - internal/config/config_test.go
  - internal/api/middleware/auth_login_test.go   # only to drop the pinned-500 case
spec:
  - docs/03-architecture.md §8
---

## Goal
Ratatoskr refuses to start when `--auth-token` holds bytes that cannot go into a cookie. Today such a token makes every
cookie login answer 500, while Bearer auth still works (T-024 review, minor 1).

## Scope
- In: one check in `Config.Validate`, its tests, and removal of the T-024 test that pins the 500.
- Out: any change to how tokens are compared, to the login endpoints, or to token rotation.

## Acceptance criteria
- [x] `Validate` returns an error when `AuthToken` is non-empty and holds any byte outside printable ASCII
      (0x21–0x7E), or any of `"`, `,`, `;`, `\`. These are the cookie-safe bytes listed in docs/03 §8.
      The error names the flag and never contains the token.
- [x] An empty token still means auth is off, so it stays valid.
- [x] Table test: a valid token, one case per forbidden class (space, `"`, `,`, `;`, `\`, a control byte, non-ASCII),
      and a check that the token never appears in the error text.
- [x] The T-024 middleware test that pins the 500 for such a token is removed, because config now makes that state
      unreachable. The 500 path in the handler stays as a defensive branch.
- [x] `make check` green.

## Notes from lead
- Build the check from a byte loop or `strings.ContainsFunc`. Do not use `http.Cookie.Valid`, which would drag a cookie
  into config. No new dependencies.

## Plan (implementer, ≤15 lines)

## Out-of-scope change needed (if any)

## Handoff (implementer)
- What changed: `Config.Validate` rejects a non-empty `AuthToken` containing anything outside 0x21–0x7E or any of
  `" , ; \`. The error names `auth_token` but never includes the token. Added table cases in `config_test.go`, removed
  `TestLoginTokenNotCookieSafe`, and updated the docs/03 §8 sentence (lead).
- How to verify: `go test -race ./internal/config/ ./internal/api/middleware/`; `make check`.
- Known limitations: done by the lead on `main` with operator approval instead of by a backend agent on a task branch.
- Follow-ups (proposed tasks): none.

## Review (reviewer)
- Verdict: approve | changes_requested
- Findings:
  - [ ] [blocker|major|minor|nit] path:line — description
