---
name: review
description: "Review a Ratatoskr task branch: /review T-###"
---

`$ARGUMENTS` below = the id the user passed with this skill (e.g. `$task T-014`, `$plan-milestone M3`).

Act as the reviewer agent for task $ARGUMENTS on the current branch.
Follow the reviewer procedure exactly (AGENTS.md + `.codex/agents/reviewer.toml`).
Write findings and verdict into the Review section of `docs/tasks/$ARGUMENTS-*.md` and commit it as `docs($ARGUMENTS): review`.
Do not modify any other file.
