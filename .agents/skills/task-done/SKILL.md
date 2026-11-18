---
name: task-done
description: "Mark an approved task done and open its PR"
---

`$ARGUMENTS` below = the id the user passed with this skill (e.g. `$task T-014`, `$plan-milestone M3`).

Mark the task done and open its PR. Task: $ARGUMENTS (if empty, read the id from the current branch `task/T-###-*`).

1. Read `docs/tasks/T-###-*.md`. Look at the **last** reviewer verdict in `## Review`. If it is not **approve**, stop and say why. Do not change anything.
2. Make sure the working tree is clean and you are on the task branch, not `main`.
3. Set `status: done` in the front matter. Commit only that file: `docs(T-###): mark done`.
4. Push the branch: `git push -u origin HEAD`.
5. Take OWNER/REPO from `git remote get-url origin`. It must not be `dstotijn/hetty` (AGENTS.md rule 11); if it is, stop and tell the operator.
6. If `gh pr view --repo OWNER/REPO <branch>` already finds a PR, print its URL and stop. Otherwise run
   `gh pr create --repo OWNER/REPO --base main --head <branch> --title "feat(T-###): <task title>" --body …`
   (use `fix`/`docs`/`chore` instead of `feat` if that fits the task better). Build the body from the task file:
   - a link to the task file and the spec sections it lists
   - what changed (from Handoff)
   - anything left out or filed as out of scope
   - review findings that are still open (`- [ ]` items), with their severity
   - how to verify
7. Print the PR URL and the open review items in 3 lines or fewer.

Do not edit code or run `make check` again. The reviewer already approved this commit.
