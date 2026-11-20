# 09 — Autonomous Orchestration (lead + Herdr)

The operator gives the lead a list of tasks (`/orchestrate T-035 T-106 T-107`). The lead starts worker agents in Herdr,
routes each to a harness/model (`docs/agents/routing.md`), runs cross-family review, opens PRs and (optionally) merges.
The operator is interrupted only for the escalations in §3.

This document extends `docs/07-agent-workflow.md`; everything there still applies (task files, paths ownership,
Definition of Done, cross-model review).

## 1. Roles in this mode

| Who | Does | Never does |
|---|---|---|
| Operator (human) | Starts runs, answers escalations, merges when `auto_merge: false`, edits `routing.md` | — |
| Lead / orchestrator (Claude Code, `opus`) | Plans, starts/stops workers, answers technical questions, writes lead answers into task files/spec, opens PRs via workers, merges if allowed | Implements worker tasks itself; approves unknown permission prompts; force-pushes; touches `dstotijn/hetty` |
| Worker (implementer / reviewer / qa) | Its task, inside its worktree and `paths` | Asks questions in the terminal (see §2) |

## 2. Autonomous mode (workers)

Workers started by the orchestrator run in **autonomous mode** (also stated in `AGENTS.md`):
- Never ask the operator anything in the terminal and never wait for a reply.
- Unsure about product behaviour or scope → write the question under `## Plan` → `Question for lead:` in the task file,
  set `status: blocked`, commit, stop.
- Unsure about a technical detail inside your `paths` → choose the simplest option that meets the spec, note it in Handoff.
- A tool or permission is missing → set `status: blocked` with the exact command that was refused, stop.
- Finish every turn with the task file updated; the orchestrator reads the file, not your screen.

## 3. Escalations (the only reasons to interrupt the operator)

| Code | When | What the lead sends |
|---|---|---|
| E1 | A product/behaviour decision the spec does not cover, or a scope change | The question, 2–3 options, recommended option |
| E2 | A new dependency, a new ADR, or a change to an accepted ADR | The proposal and why |
| E3 | `max_review_rounds` exhausted on every tier in the escalation order | Task, last review findings, recommendation (split task / take over / drop) |
| E4 | Login, credentials, quota exhausted, unverified model ID, unknown permission/trust prompt | Exact prompt or error, which harness |
| E5 | Any destructive or history-changing git action (force-push, delete a branch it did not create, rewrite history) | The command and why |
| E6 | `auto_merge: false` and a waiting task depends on a PR that is ready | All ready PRs in one list |

The lead batches open escalations into **one** message, keeps unaffected tasks running, and waits. In Herdr the lead's
pane then shows as blocked — that is the operator's notification.

## 4. State and recovery
- `.orchestrator/` (git-ignored, in the main checkout): `run.md` (state table), `herdr.md` (verified CLI syntax),
  `verified.md` (verified model IDs), `escalations.md` (open and answered escalations).
- The task files on their branches are the source of truth for task state; `run.md` is the orchestrator's cache.
- Restart after a crash or a full context: `/orchestrate --resume` — the lead rebuilds state from `run.md`, `herdr agent list`
  and the task files, and continues.

## 5. One-time setup (operator)
1. Run Herdr on a machine dedicated to this project (VPS or container recommended when `permission_profile: full`).
2. Install integrations: `herdr integration install claude`, `herdr integration install codex`, `herdr integration install opencode` (opencode workers run as `opencode mini`; see `docs/agents/routing.md` §5).
   Codex asks you to trust the new hook once.
3. Log in to every harness (`claude`, `codex login`, `opencode auth login`) on that machine.
4. Verify the ⚠ model IDs in `docs/agents/routing.md` and fix them in a commit.
5. `gh auth status` works and `gh repo set-default pocikode/ratatoskr` is set.
6. GitHub: branch protection on `main` (PR required, CI required). This is the last safety net in autonomous mode.
7. Start the lead in Herdr from the main checkout: `claude --agent lead --model opus`, then `/orchestrate T-… T-…`.

## 6. Safety rules (non-negotiable)
- Never `--force` push; never push to `upstream`; never merge with failing or pending checks.
- Never approve a permission or trust prompt whose command is not on the allowlist — escalate (E4).
- Never edit `docs/agents/routing.md` or this file during a run; propose changes to the operator instead.
- Never let two running tasks own overlapping `paths`.
