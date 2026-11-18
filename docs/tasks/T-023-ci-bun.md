---
id: T-023
title: Install Bun and web dependencies in CI so make check runs the web checks
milestone: M0
role: lead
status: done
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
**Reviewer**: lead (post-merge reconciliation) · 2026-11-18 · `cdb122f` (merged as PR #3, `ccdef3e`)

The task was merged without a recorded verdict: PR #3 landed the code and the task file in the same commit, so the
`Review` section was still the empty template when `main` picked it up. Nothing below is a code change; this records the
verification that should have preceded the merge, run against `main` at `985ca05` and the PR's final CI run.

**Checks**
- PR #3 final run `44124552779`: `make check (ubuntu-latest)` pass (52s), `make check (macos-latest)` pass (2m13s).
  The handoff cites run `44124158942` — that was an earlier push on the same PR; `44124552779` is the run that guarded the merge.
- `main:.github/workflows/ci.yml`: `BUN_VERSION: 1.4.4` in job `env`, `oven-sh/setup-bun@v2` with `bun-version: ${{ env.BUN_VERSION }}`,
  then `bun install --frozen-lockfile` with `working-directory: web`, all before `make check`. Pinned exact version, one place to bump ✔
- `main:docs/06-frontend-conventions.md` §1: the Bun row states the pin location and version and the `--frozen-lockfile` install ✔
- `main:docs/08-roadmap.md`: T-023 present in the M0 index, and T-028 already depends on it ✔
- Scope: `ccdef3e` touched exactly `ci.yml`, docs/06, docs/08 and this task file — all inside `paths` ✔
- Hard rules: no product dependency added (CI action only, no ADR needed), no generated code touched, no secrets ✔

**AC coverage**: all five checkboxes stand on the evidence above (pinned Bun, frozen-lockfile install, both matrix jobs
green on the branch PR, pin documented in docs/06, local `make check` green per handoff).

**Findings**
- [x] [minor] docs/tasks/T-023-ci-bun.md:59-61 — review section was left as the template and `status` stayed `in_review`
  after the merge → closed out here as `done`. Process note for later lead tasks: mark `done` in the handoff commit or in a
  follow-up commit before merging, so `main` never carries a stale `in_review`.
- [x] [minor] docs/tasks/T-023-ci-bun.md:53 — handoff cites the wrong CI run number → corrected in the checks above.
- [ ] [minor] `.github/workflows/ci.yml:32` + `web/package.json` — the Bun version is pinned in CI only; a dev on another
  version can still resolve a different tree (caught by `--frozen-lockfile` at review time, but late). A `packageManager`
  field (corepack-style) or `.bun-version` would make local and CI agree by construction → fold into T-021 or a small follow-up.
- [ ] [minor] `.github/workflows/ci.yml:47-49` — `bun install` has no dependency cache, so each job re-downloads the tree.
  Deliberately out of scope here; T-028 (CI cost trim) is the natural home.

**Verdict**: approve — merged and green on `main`.
