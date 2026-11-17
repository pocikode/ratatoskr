---
id: T-023
title: Install Bun and web dependencies in CI so make check runs the web checks
milestone: M0
role: lead
status: in_review
depends_on: [T-001, T-003]
owner: "claude-code (claude-opus-5-5)"
branch: task/T-023-ci-bun
paths:
  - .github/**
  - docs/tasks/T-023-ci-bun.md
  - docs/08-roadmap.md
  - docs/06-frontend-conventions.md
spec:
  - docs/06-frontend-conventions.md §1, §8
  - docs/tasks/T-001-repo-bootstrap.md
---

## Goal
CI on `main` is green again. Since T-003 added `web/package.json`, `make check` runs `web-check`/`web-test`, but the
workflow (T-001) never installs Bun or the web dependencies, so every run fails with `bun: not found`
(run 44075680226, commit 13829fe).

## Scope
- In: `oven-sh/setup-bun` step with a pinned Bun version, `bun install --frozen-lockfile` in `web/`, record the CI Bun
  pin in docs/06.
- Out: Playwright E2E in CI (lands with T-021), dependency caching tweaks, nightly fuzz job.

## Acceptance criteria
- [x] CI workflow installs Bun at a pinned exact version (same pattern as `GOLANGCI_LINT_VERSION`).
- [x] CI installs web dependencies with `bun install --frozen-lockfile` (fails on lockfile drift).
- [x] `make check` passes on ubuntu-latest and macos-latest for this branch's PR.
- [x] docs/06 states the Bun version CI uses and where it is pinned.
- [x] `make check` green locally.

## Notes from lead
- `oven-sh/setup-bun@v2` (official Bun action, v2.2.0 latest) is a CI-only action, not a product dependency; no ADR
  needed, recorded here and in docs/06.
- Pin Bun 1.4.4 (version used to generate `web/bun.lock` locally). Bump in one place: `BUN_VERSION` in `ci.yml`.

## Plan (implementer, ≤15 lines)
1. Add `BUN_VERSION` env, `oven-sh/setup-bun@v2` step and `bun install --frozen-lockfile` (working-directory `web`) before `make check`.
2. Note the pin in docs/06 §1 stack table row for Bun.
3. Add T-023 to the docs/08 M0 index.
4. `make check` locally, push, open PR against pocikode/ratatoskr, confirm both matrix jobs green.

## Out-of-scope change needed (if any)

## Handoff (implementer)
- What changed: `ci.yml` adds `BUN_VERSION: 1.4.4`, `oven-sh/setup-bun@v2`, and `bun install --frozen-lockfile` in `web/`
  before `make check`. docs/06 §1 records the pin; docs/08 M0 index lists T-023.
- How to verify: PR #3 checks green on ubuntu-latest + macos-latest (run 44124158942); locally `make check` green
  (svelte-check 0/0, eslint + prettier clean, vitest 37/37).
- Known limitations: Bun version is pinned in two places in practice (CI `BUN_VERSION` and whatever devs have
  locally); no `.bun-version`/`packageManager` field yet. No bun dependency cache in CI (setup-bun caches the binary only).
- Follow-ups (proposed tasks): T-021 should add a Playwright E2E job (needs `bunx playwright install --with-deps`).

## Review (reviewer)
- Verdict: approve | changes_requested
- Findings:
