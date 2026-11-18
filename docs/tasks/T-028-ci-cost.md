---
id: T-028
title: Trim CI runs (skip docs-only and draft PRs, macOS only on main)
milestone: M0
role: lead
status: in_review
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
- [x] AC1 — a PR or push to `main` that only changes `docs/**` or `*.md` starts no CI run.
- [x] AC2 — a draft PR runs no job; marking it ready for review (`ready_for_review`) starts the run.
- [x] AC3 — PR runs use `ubuntu-latest` only; pushes to `main` and `workflow_dispatch` runs use `ubuntu-latest` and `macos-latest`.
- [x] AC4 — the policy, and how to run macOS on a branch by hand, are documented in docs/07 §8.
- [ ] `make check` green (CI on this PR, which changes `ci.yml` and so is not skipped)

## Notes from lead
- Operator asked to cut Actions cost. The repo is public, so standard hosted runners are free today. This change still
  matters if the repo goes private, where macOS minutes count 10x Linux.
- Pitfall: if `make check` is later made a **required** status check, a docs-only PR waits forever for it. Add a stub job
  that reports success for skipped paths when branch protection is turned on.

## Plan (implementer, ≤15 lines)
1. `paths-ignore: ['docs/**', '**/*.md']` on `push` and `pull_request`; add `workflow_dispatch`.
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
- Verdict: approve | changes_requested
- Findings:
