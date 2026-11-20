# Agent Routing

Read by the **lead** when it orchestrates (`/orchestrate`). Humans edit this file to change which model does what.
Model IDs marked ⚠ must be checked once on the operator's machine (`codex` → `/model`, `opencode models`, `claude --help`)
and corrected here; the lead never guesses an ID.

## 1. Settings

| Setting | Value | Meaning |
|---|---|---|
| `max_workers` | 4 | Implementer + reviewer sessions running at the same time (lead not counted) |
| `max_per_harness` | claude: 2, codex: 2, opencode: 2 | Running sessions per harness |
| `max_review_rounds` | 3 | `changes_requested` rounds before the task is escalated one tier |
| `auto_merge` | **false** | `true`: lead squash-merges when reviewer approved + CI green. `false`: lead opens the PR, keeps working on other tasks, and asks the operator to merge only when a pending task depends on it |
| `permission_profile` | **safe** | `safe` or `full` — see §5 |
| `prompt_timeout_ms` | 5400000 | Max wait for one implementer turn (90 min) before the lead reads the pane |

## 2. Harnesses and models

| Key | Harness | Model ID | Family | Tier |
|---|---|---|---|---|
| `opus` | claude | `opus` | anthropic | top |
| `sonnet` | claude | `sonnet` | anthropic | strong |
| `haiku` | claude | `haiku` | anthropic | light |
| `sol` | codex | `gpt-6.2-sol` | openai | strong |
| `terra` | codex | `gpt-6.1-terra` | openai | light |
| `ds-flash` | opencode | `sinarcloud/deepseek-v4.1-flash` | deepseek | flash |
| `ds-flash-1017` | opencode | `sinarcloud/deepseek-v4.1-flash-1017` | deepseek | flash |

Not used: GPT-6.2 Astra (cost), MiniMax M3.1 Flash (too slow). Do not route to them.

## 3. Who implements

A task is **core** when any of its `paths` is under `internal/rawhttp`, `internal/proxy`, `internal/query`,
`internal/store`, `internal/workflow`, `internal/secret`, `internal/ca`, or it is a GraphQL contract task.
A task is **small** when it is frontend or qa, touches ≤ 4 path entries, and is not core.

| Role | core | normal | small | Escalation order when it fails |
|---|---|---|---|---|
| lead (contracts) | `opus` (the orchestrator does it itself) | — | — | escalate to operator (E3) |
| backend | `sol` | `sol` | `sol` | `sol` → `opus` |
| frontend | `sonnet` | `sonnet` | `ds-flash` | `ds-flash` → `sonnet` → `sol` |
| qa | — | `ds-flash-1017` | `ds-flash-1017` | `ds-flash-1017` → `sol` → `sonnet` |
| admin chores (status fixes, Handoff tidy) | — | `haiku` or `terra` | — | — |

If the chosen harness is at its `max_per_harness`, use the next model of the **same tier** from another harness
(`sol` ⇄ `sonnet`; `ds-flash` ⇄ `ds-flash-1017`), never a lower tier. `opus` is never a substitute for capacity, only an escalation.

## 4. Who reviews

The reviewer is **always from a different family** than the implementer.

| Implementer family | core task | other task |
|---|---|---|
| anthropic | `sol` | `sol` |
| openai | `opus` | `sonnet` |
| deepseek | `sonnet` | `sonnet` (or `sol` if `sonnet` is busy) |

Flash models never review. `opus` reviews only core tasks written by `sol` (save quota for orchestration).

## 5. Launch commands

`<role>` = the task's `role`. `<GITDIR>` = absolute path of the main checkout's `.git` directory (worktrees commit into it).
`<wt>` = the worktree's admin dir name: `basename $(git -C <worktree> rev-parse --git-dir)`.

All three harnesses are interactive: pass these after `--` to `herdr agent start <name> --kind <harness> --pane <pane>`.

| Harness | profile `safe` | profile `full` (only on a dedicated VPS/container) |
|---|---|---|
| claude | `--agent <role> --model <id> --permission-mode auto` (deny list in `.claude/settings.json` still applies) | `--agent <role> --model <id> --dangerously-skip-permissions` |
| codex | `-m <id> --sandbox workspace-write --ask-for-approval never --add-dir <GITDIR>/worktrees/<wt> --add-dir <GITDIR>/objects --add-dir <GITDIR>/refs --add-dir <GITDIR>/logs -c sandbox_workspace_write.network_access=true` | `-m <id> --dangerously-bypass-approvals-and-sandbox` |
| opencode | `mini --agent <role> --model <id>` (permissions from `opencode.json`) | same |

Notes:
- **claude**: `auto` mode lets a safety classifier approve routine commands (pipes, `cd`, `git rm`, …) and refuse risky ones
  without stopping on a prompt. A refused command is reported to the worker, which then follows AGENTS.md autonomous mode.
- **codex**: the workspace-write sandbox keeps every `.git` directory read-only, even one passed with `--add-dir`, so add the
  four sub-directories above instead (needed for `git add/commit/rm/push`). Commits may print
  `Unable to create '.../packed-refs.lock'` from git's background maintenance; it is harmless (exit 0).
- **opencode mini**: Herdr's opencode integration (v13) does not report state for opencode 2.3 `mini` sessions, so
  `agent_status` from Herdr is a guess from the screen and is wrong while mini works (`blocked`/`idle`). For opencode:
  - Send prompts with `herdr pane send-text <pane> "<prompt>"` then `herdr pane send-keys <pane> enter`
    (`herdr agent prompt` does not always deliver).
  - Never use `herdr agent prompt --wait` / `herdr agent wait`. The turn is over when the task file `status` leaves
    `in_progress` (implementer) or the `## Review` verdict is filled (reviewer). Wait with a Monitor until-loop on the task
    file in the worktree; if it is quiet for `prompt_timeout_ms`, read the pane: mini is still working while its footer
    shows `esc stop`.
  - An `ask` rule in `opencode.json` stops mini on a prompt; the lead never answers it (E4).

Check each flag once with `<harness> --help` (`opencode mini --help` for opencode); if a flag differs in the installed
version, fix this section, don't improvise.

## 6. First prompt per harness

| Harness | Implementer | Reviewer |
|---|---|---|
| claude | `/task T-###` | `/review T-###` |
| opencode | `Autonomous mode (AGENTS.md). Follow .opencode/commands/task.md for T-###.` | `Autonomous mode (AGENTS.md). Follow .opencode/commands/review.md for T-###.` |
| codex | `Act as the role in .codex/agents/<role>.toml (developer_instructions). Autonomous mode (AGENTS.md). $task T-###` | `Act as the role in .codex/agents/reviewer.toml. Autonomous mode (AGENTS.md). $review T-###` |

After approval: `/task-done T-###` (claude), `$task-done T-###` (codex) or `Follow .opencode/commands/task-done.md for T-###.`
(opencode, as a later turn of the implementer session), sent to the **implementer**.
