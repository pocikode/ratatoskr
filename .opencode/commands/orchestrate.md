---
description: "Lead only: run tasks end-to-end with worker agents via Herdr: /orchestrate T-### [T-###…]"
agent: lead
---

You are the lead acting as **orchestrator** for tasks: $ARGUMENTS.
Follow `docs/09-orchestration.md` exactly and route models with `docs/agents/routing.md`. Do not implement worker tasks yourself
(contract tasks with `role: lead` are the exception: do those yourself, in their own worktree).

## 0. Preflight (once per run)
1. You must be inside Herdr (`herdr agent list` works). If not, stop and tell the operator.
2. `git fetch origin && git status` on `main` in the main checkout; it must be clean and up to date.
3. Read `docs/agents/routing.md`. If any model ID you need is still marked ⚠ and you have no verified note in
   `.orchestrator/verified.md`, verify it now (`codex` model list, `opencode models`) or escalate (rule E4).
4. Run `herdr worktree --help`, `herdr agent start --help`, `herdr agent prompt --help`, `herdr agent wait --help`,
   `herdr agent read --help` and `opencode mini --help` once; write the exact syntax you will use to `.orchestrator/herdr.md`. Never guess a flag.
5. Create or resume `.orchestrator/run.md` (state table below). If it exists, resume from it instead of starting over.

## 1. Plan
For each task id: read its file (`docs/tasks/*/T-###-*.md`), get `role`, `status`, `depends_on`, `paths`.
- `status: done` → skip. `depends_on` not all merged on `origin/main` → mark `waiting` (re-check after every merge).
- Classify core / normal / small and pick implementer + reviewer from routing.md. Respect `max_workers` / `max_per_harness`.
- Two ready tasks whose `paths` overlap → run them one after the other.
Write the plan into `.orchestrator/run.md`:

| Task | Role | Class | Implementer | Reviewer | Worktree | Agent names | State | Round | PR | Notes |

## 2. Loop (until every task is merged, or only escalations remain)
For each ready task, without waiting for the others:
1. **Worktree**: `herdr worktree create` for branch `task/T-###-<slug>` from `origin/main` (reuse if it exists). Keep the returned pane id.
2. **Implementer**: `herdr agent start t###-impl --kind <harness> --pane <pane> -- <launch args from routing §5>`;
   then `herdr agent prompt t###-impl "<first prompt from routing §6>" --wait --timeout <prompt_timeout_ms>`.
3. **Judge by the task file, not the screen.** Read `status` from the task file in that worktree.
   - `in_review` → step 4.
   - `blocked` → read its question; apply §3 (answer it yourself, or escalate). If you answered, prompt the implementer: `Question answered in <file/section>. Continue /task T-###.`
   - still `in_progress` and the agent is idle → `herdr agent read t###-impl --lines 80`; if it is asking something in the terminal,
     answer per §3 or tell it: `Autonomous mode: write the question in the task file, set status: blocked, stop.`
   - agent `blocked` on a permission/trust prompt → escalate (E4); never approve unknown prompts yourself.
4. **Review**: start `t###-rev` in a split of the same worktree with the reviewer from routing §4; prompt the review command; wait.
   - verdict `changes_requested` → round += 1. If round < `max_review_rounds`: prompt the implementer `/task T-###`.
     Else: stop that implementer, escalate one tier (routing §3), restart from step 2 with a fresh session (same worktree).
     No tier left → escalate (E3).
   - verdict `approve` → prompt the implementer with the post-approval prompt from routing §6; record the PR number.
5. **CI + merge**: `gh pr checks <n> --repo <OWNER/REPO> --watch`.
   - red → prompt the implementer with the failing check names: `CI failed: <checks>. Fix within your paths, push, keep status done.` (counts as a round).
   - green and `auto_merge: true` → `gh pr merge <n> --repo <OWNER/REPO> --squash --delete-branch`.
   - green and `auto_merge: false` → state `ready_to_merge`; continue other tasks. Escalate (E6) only when a waiting task depends on it, batching all ready PRs in one message.
6. **After merge**: close both agents, `herdr worktree remove` the worktree, `git fetch origin`, re-check `waiting` tasks.
7. Update `.orchestrator/run.md` after every state change.

## 3. Answering vs escalating
You may answer worker questions yourself when the answer is technical and does not change product behaviour:
naming, internal interfaces, test strategy, file layout within the task's `paths`, regenerating code (`make gen`), rebases.
Write every answer into the task file (Notes from lead) or the spec, commit it on the task branch: `docs(T-###): lead answer`.
Escalate to the operator ONLY for E1–E6 in `docs/09-orchestration.md`. Escalation = one message listing every open item
(task, question, your recommended answer, options), then wait. Keep other tasks running while you wait.

## 4. Context hygiene
- Never read whole panes repeatedly; prefer `herdr agent prompt … --wait` / `herdr agent wait` and the task file.
- When your context is getting long: make sure `.orchestrator/run.md` is current, tell the operator
  `Orchestrator checkpoint written; restart me with /orchestrate --resume`, and stop.

## 5. Finish
Print a summary table: task, implementer, reviewer, rounds, PR, state. List anything waiting for the operator.
