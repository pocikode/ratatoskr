---
id: T-037
title: Environment backend - Global and project environments, sealed secrets, interpolation, resolvers
milestone: M2
role: backend
status: blocked
depends_on: [T-098]
owner: "Codex (GPT-6)"
branch: task/T-037-environment-backend
paths:
  - internal/environment/**
  - internal/secret/**
  - internal/ca/secret*.go
  - internal/ca/ca.go
  - internal/ca/root.go
  - internal/ca/row.go
  - internal/store/migrations/instance/0003_env.sql
  - internal/store/migrations/project/0004_env.sql
  - internal/api/graphql/resolvers/environment*.go
  - internal/api/graphql/resolvers/stubs_environment.go
spec:
  - docs/01-feature-spec.md#F-ENV-1 (incl. M2 contract)
  - docs/03-architecture.md §2 (`secret/`), §5.1, §5.2, §8 (secrets at rest)
---

## Goal
Environments with sealed secret values serve the T-098 contract, and `environment` exposes the interpolation used by Replay
(T-106) and later Automate/workflows.

## Scope
- In: move the XChaCha20-Poly1305 sealer from `internal/ca/secret.go` into a new exported `internal/secret` package
  (`secret.key` handling unchanged: same file, 0600, loose-permission tightening) and make `internal/ca` use it;
  `environment.Service`: Global in `env_global` (instance.db), project envs in `environments`/`env_vars`, selection, CRUD,
  `SetVariables` (atomic replace), secret values sealed at rest (base64 in the TEXT column is fine; migrations 0003/0004
  only if a column must change); `Vars(ctx) map[string]string` (selected over Global) and
  `Interpolate(raw []byte, vars map[string]string) []byte`; resolvers; delete `stubs_environment.go`.
- Out: UI (T-103), setting variables from workflows/plugins (M5/M6), keeping secrets out of exports/AI (M3/M6).

## Acceptance criteria
- [ ] Existing CA tests pass unchanged after the sealer move; an instance created before the change still opens its CA.
- [ ] Global always listed first; rename/delete Global → `CONFLICT`; env names 1–200, unique per project, "Global" reserved.
- [ ] Variable validation (name pattern, ≤ 100 chars, unique, value ≤ 1 MiB) → `BAD_INPUT` naming the variable;
      `SetVariables` is all or nothing.
- [ ] A secret value is not stored in plaintext (test reads the raw row) and reads back decrypted.
- [ ] `selectEnvironment` per project; null deselects; deleting the selected env leaves none selected.
- [ ] `Interpolate` table test: `{{a}}`, `{{ a }}`, unknown verbatim, adjacent `{{a}}{{b}}`, `{{{a}}}`, value containing
      `{{b}}` not expanded again, non-UTF-8 bytes around a match preserved.
- [ ] `make check` green.

## Notes from lead
- `internal/secret` is new in docs/03 §2 (lead, 2026-11-19). Keep its API to `Open(root *os.Root) (*Box, loose bool, err)`,
  `Seal`, `Open` (names may differ); both CA and environment may open it, so first-start key creation must be race-free
  (create with `O_EXCL`, reread on `EEXIST`).

## Plan (implementer, ≤15 lines)
1. Extract the shared secret box while preserving existing CA ciphertext and key-file compatibility.
2. Add environment storage, validation, atomic variable replacement, selection and byte-preserving interpolation.
3. Replace GraphQL stubs with thin service adapters; test services, secret storage and GraphQL behavior.
4. Wire the service into the running app, run `make check`, and complete the handoff.

## Out-of-scope change needed (if any)
- Blocked: add `internal/app/app.go` to `paths` so the environment service can be constructed with the instance
  database, selected-project database accessor and secret box, then passed as `resolvers.Deps.Environment`.
  Service construction currently lives at `internal/app/app.go:140`; GraphQL dependencies are assembled at
  `internal/app/app.go:306` without `Environment`. Implementing only the allowed files would leave the running
  application's environment operations returning `NOT_IMPLEMENTED`. AGENTS.md hard rule 1 requires stopping here.
- Dependency verified: T-098 is `done` and merged on main (`d1db460`). No implementation changes made.

## Handoff (implementer)
- What changed:
- How to verify:
- Known limitations:
- Follow-ups (proposed tasks):

## Review (reviewer)
- Verdict: approve | changes_requested
- Findings:
