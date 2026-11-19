---
id: T-028
title: Trim CI runs (skip docs-only and draft PRs, macOS only on main)
milestone: M0
role: lead
status: done
depends_on: [T-023]
owner: "claude-code (claude-opus-5-5)"
branch: task/T-028-ci-cost
paths:
  - .github/workflows/ci.yml
  - docs/tasks/T-028-ci-cost.md
  - docs/07-agent-workflow.md
  - docs/08-roadmap.md
spec:
  - docs/07-agent-workflow.md §8
---

## Goal
CI stops spending runner minutes on changes that cannot break the build, while every code PR is still checked before merge.

## Scope
- In: `on:` triggers, a draft-PR guard, and the OS matrix in `ci.yml`; a note on the policy in docs/07 §8.
- Out: required status checks / branch protection, nightly fuzz job (docs/05), caching.

## Acceptance criteria
- [x] AC1 — a PR or push to `main` that only changes `docs/**` or root `*.md` starts no CI run.
- [x] AC2 — a draft PR runs no job; marking it ready for review (`ready_for_review`) starts the run.
- [x] AC3 — PR runs use `ubuntu-latest` only; pushes to `main` and `workflow_dispatch` runs use `ubuntu-latest` and `macos-latest`.
- [x] AC4 — the policy, and how to run macOS on a branch by hand, are documented in docs/07 §8.
- [x] `make check` green (CI on this PR, which changes `ci.yml` and so is not skipped)

## Notes from lead
- Operator asked to cut Actions cost. The repo is public, so standard hosted runners are free today. This change still
  matters if the repo goes private, where macOS minutes count 10x Linux.
- Pitfall: if `make check` is later made a **required** status check, a docs-only PR waits forever for it. Add a stub job
  that reports success for skipped paths when branch protection is turned on.

## Plan (implementer, ≤15 lines)
1. `paths-ignore: ['docs/**', '*.md']` on `push` and `pull_request`; add `workflow_dispatch`.
2. `pull_request.types` adds `ready_for_review`; job `if:` skips draft PRs.
3. Matrix `os` comes from `fromJSON(...)`: Linux on `pull_request`, Linux + macOS otherwise.
4. Document the policy in docs/07 §8 and add a roadmap row.

## Out-of-scope change needed (if any)

## Handoff (implementer)
- What changed: `.github/workflows/ci.yml` triggers/guard/matrix; docs/07 §8 CI policy line; roadmap row T-028.
- How to verify: this PR shows one `make check (ubuntu-latest)` check. After merge, the `main` run shows both OSes.
  A docs-only PR shows no CI run. `gh workflow run CI --repo pocikode/ratatoskr --ref <branch>` runs both OSes.
- Known limitations: macOS-only regressions are caught after merge (or by a manual run), not on the PR. Workflow
  checked by YAML parse and actionlint v1.7.7 (no findings).
- Follow-ups (proposed tasks): stub "skipped" job if `make check` becomes a required check.

## Review (reviewer)
- Verdict: approve
- Checked: diff is limited to the 4 files in `paths`. PR #9 CI is green with exactly one check, `make check (ubuntu-latest)`,
  which proves the PR side of AC3. Local `make check`: all Go steps pass (fix-check, vet, lint, `go test -race`); `web-check`
  fails with `svelte-check: command not found` because this checkout has no `web/node_modules`. That is an environment
  problem, not a regression, since the diff touches nothing under `web/`. actionlint is not installed locally, so I took
  the handoff's actionlint result on trust.
- AC1: `paths-ignore` skips a run only when *every* changed file matches. Per GitHub filter semantics, `**/*.md` also
  matches root `README.md`/`AGENTS.md`. Nothing that builds embeds or reads `docs/**` or `*.md`: the only `go:embed`s
  are `web/dist` and the SQL migrations. AC2: `ready_for_review` is in `types`, and the job `if:` skips draft PRs. A
  skipped job reports as "Skipped", which branch protection treats as passing. AC3: `fromJSON` matrix is correct, and
  the job-level `if:` does not depend on `matrix`. AC4: documented in docs/07 §8.
- `concurrency: ci-${{ github.ref }}`: a `workflow_dispatch` run on a branch (`refs/heads/…`) and the PR run
  (`refs/pull/N/merge`) are in separate groups, so a manual macOS run does not cancel the PR check. Good.
- Findings:
  - [x] [minor] .github/workflows/ci.yml:8,11 — `'**/*.md'` also ignores Markdown under `web/`, but `web` `bun run check`
    runs `prettier --check .`, which formats `.md` files (`web/.prettierignore` does not exclude them). No `web/**/*.md`
    exists today. Once one is added, a PR that changes only that file skips CI and could turn `main` red. → either add
    `'!web/**/*.md'` after `'**/*.md'`, or add `*.md` to `web/.prettierignore` (needs a frontend task). Alternatively,
    record it under Known limitations. *Lead: fixed by narrowing to root `'*.md'`; `!` negation is not supported in
    `paths-ignore` (only in `paths`), so the first option would not work.*
  - [x] [nit] docs/07-agent-workflow.md:65 — the new bullet line is ~230 chars, while the rest of §8 wraps near 130.
    → reflow to match the neighbouring lines.
  - [x] [nit] docs/tasks/T-028-ci-cost.md AC list — the `make check` box is still unchecked even though PR #9 CI is
    green. → tick it when the operator merges.
