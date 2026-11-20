---
id: T-112
title: Make CI green - race-aware time bound in matchreplace, no port reuse race in proxy 502 test
milestone: M2
role: qa
status: in_review
depends_on: []
owner: "opencode (deepseek-v4.1-flash-1017)"
branch: task/T-112-ci-test-timing
paths:
  - internal/matchreplace/replace_test.go
  - internal/matchreplace/race_on_test.go
  - internal/matchreplace/race_off_test.go
  - internal/proxy/proxy_test.go
spec:
  - docs/05-go-conventions.md (testing)
  - AGENTS.md hard rule 7 (never weaken tests)
---

## Goal
`make check` is green on `main` in CI again. Two tests fail there for timing reasons, not product bugs.

## Scope
- In:
  - `TestRegexSharedGroupsPerMatch` (`internal/matchreplace/replace_test.go:155`): its 10 s wall-clock bound fails on
    the CI runner under `-race` (22-26 s; 0.33 s without race, 5.4 s with race on a dev machine). Keep the bound as a
    performance guard when the race detector is off; under `-race` keep the correctness assertions and skip only the
    time check. Use a `raceEnabled` const from a `//go:build race` / `//go:build !race` pair of `_test.go` files.
  - `TestUpstreamErrorIs502AndRecorded` (`internal/proxy/proxy_test.go:155`): it closes a listener and dials its port,
    but parallel tests can reuse that port, so the recorded entry is sometimes not the 502. Make the "dead upstream"
    deterministic (e.g. an address that can never be bound by the test process) without changing what is asserted.
- Out: any non-test code; other tests.

## Acceptance criteria
- [x] `go test -race -count=20 -run 'TestRegexSharedGroupsPerMatch|TestUpstreamErrorIs502AndRecorded' ./internal/matchreplace/ ./internal/proxy/` passes.
- [x] Without `-race`, `TestRegexSharedGroupsPerMatch` still fails if `replace` takes over 10 s (bound unchanged).
- [x] All existing assertions of both tests are kept (rule 7: no weakening beyond the race-only time check).
- [x] `make check` green.

## Notes from lead
- Created by the orchestrator: `main` went red after #52/#53/#54 merged (all three CI runs failed on
  `TestRegexSharedGroupsPerMatch`; #52 also on the proxy test). Not caused by those PRs.
- Shell rules (autonomous mode): one simple allowlisted command per call, no pipes/chains/`cd`.

## Plan (implementer, ≤15 lines)
1. `race_on_test.go` (`//go:build race`) + `race_off_test.go` (`//go:build !race`): `const raceEnabled`.
2. `TestRegexSharedGroupsPerMatch`: keep both cases and the 10 s bound, apply the bound only when `!raceEnabled`.
3. `TestUpstreamErrorIs502AndRecorded`: dial `127.0.0.1:1` instead of a closed listener — a privileged port the
   unprivileged test process can never bind and the kernel never hands out for `:0`, so no parallel test can take it.
   Assertions untouched.
4. Verify `-race -count=20` on both packages, then `make check`.

## Out-of-scope change needed (if any)

## Handoff (implementer)
- What changed:
  - `internal/matchreplace/race_on_test.go` (`//go:build race`) and `race_off_test.go` (`//go:build !race`) define
    `const raceEnabled`; `TestRegexSharedGroupsPerMatch` keeps the 10 s bound but applies it only when `!raceEnabled`.
  - `TestUpstreamErrorIs502AndRecorded` now targets the fixed `127.0.0.1:1` instead of a listener it opened and closed.
    Port 1 is privileged, so the unprivileged test process can never bind it, and the kernel never hands it out for
    `:0`, so no parallel test can take it over. Assertions and the 502 body/raw checks are unchanged.
- How to verify:
  - `go test -race -count=20 -run 'TestRegexSharedGroupsPerMatch|TestUpstreamErrorIs502AndRecorded' ./internal/matchreplace/ ./internal/proxy/`
  - `go test -count=1 -run TestRegexSharedGroupsPerMatch ./internal/matchreplace/` (plain build, bound active)
  - `go list -race -f '{{.TestGoFiles}}' ./internal/matchreplace/` picks `race_on_test.go`; without `-race` `race_off_test.go`.
  - `make check` green (needs `bun install --frozen-lockfile` in `web/` first in a fresh worktree).
- Known limitations:
  - The plain-build time bound stays machine-dependent: a very slow (non-CI) machine without `-race` could still trip it.
  - `127.0.0.1:1` assumes the test process is unprivileged (true on GH runners and local dev); a root-run suite could in
    principle have something listening on port 1.
- Follow-ups (proposed tasks):
  - None.

## Review (reviewer)
- Verdict: approve
- Reviewer: sonnet (round 1, 4e1c6ac; written by lead, reviewer has no edit tool)
- Findings:
  - [ ] [nit] internal/proxy/proxy_test.go:158 — `127.0.0.1:1` assumes an unprivileged test process (documented in
    Handoff); a root-run suite could in principle bind it. No change needed.
  - [ ] [nit] internal/matchreplace/replace_test.go:169 — under `-race` the case still runs fully (~5 s/case); only the
    time check is skipped, as scoped. A future task could shrink the input under race if CI time matters.
- Checks: scope inside paths, no non-test code; rule 7 kept (10 s bound unchanged when `!raceEnabled`, correctness
  assertions run in every build); build-tag pair gives one `raceEnabled` const per build; proxy assertions unchanged.
- Verification: `go test -race -count=20 -run 'TestRegexSharedGroupsPerMatch|TestUpstreamErrorIs502AndRecorded'
  ./internal/matchreplace/ ./internal/proxy/` passed; `make check` green (first `web-test` run had 2 unrelated 5 s
  timeouts under load, rerun 461/461). `make test-int` not needed.
