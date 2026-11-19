---
id: T-037
title: Environment backend - Global and project environments, sealed secrets, interpolation, resolvers
milestone: M2
role: backend
status: in_review
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
- [x] Existing CA tests pass unchanged after the sealer move; an instance created before the change still opens its CA.
- [x] Global always listed first; rename/delete Global → `CONFLICT`; env names 1–200, unique per project, "Global" reserved.
- [x] Variable validation (name pattern, ≤ 100 chars, unique, value ≤ 1 MiB) → `BAD_INPUT` naming the variable;
      `SetVariables` is all or nothing.
- [x] A secret value is not stored in plaintext (test reads the raw row) and reads back decrypted.
- [x] `selectEnvironment` per project; null deselects; deleting the selected env leaves none selected.
- [x] `Interpolate` table test: `{{a}}`, `{{ a }}`, unknown verbatim, adjacent `{{a}}{{b}}`, `{{{a}}}`, value containing
      `{{b}}` not expanded again, non-UTF-8 bytes around a match preserved.
- [x] `make check` green.

## Notes from lead
- `internal/secret` is new in docs/03 §2 (lead, 2026-11-19). Keep its API to `Open(root *os.Root) (*Box, loose bool, err)`,
  `Seal`, `Open` (names may differ); both CA and environment may open it, so first-start key creation must be race-free
  (create with `O_EXCL`, reread on `EEXIST`).
- **Lead (2026-11-19), app wiring:** not in T-037. Constructing `environment.Service` in `app.New` and passing it as
  `resolvers.Deps.Environment` is T-109's scope (the only M2 task that edits `internal/app`, roadmap §M2). Until T-109
  lands, the running binary answering `NOT_IMPLEMENTED` for environment fields is expected and not a defect of T-037.
  Constraints so T-109 stays a pure wiring task:
  - Keep `ca.Open(ctx, ca.Deps{...})` and `ca.Deps` unchanged; `ca` opens the box itself via `secret.Open(deps.Root)`.
  - Export a constructor T-109 can call with what `app` already has, e.g.
    `environment.New(environment.Deps{Instance: <instance db>, Project: <project db accessor>, Secrets: *secret.Box})`.
  - Cover the acceptance criteria with package tests and resolver tests using a hand-built `resolvers.Deps`
    (plus `internal/api/graphql/resolvers/environment*_test.go`, already inside `paths`).
  - Note the constructor shape in Handoff so T-109 can wire it.

## Plan (implementer, ≤15 lines)
1. Address all four review findings without changing environment behavior or app wiring.
2. Keep Unix key locking and add Windows locking, including concurrent legacy empty-file initialization.
3. Hoist the interpolation regex, document whitespace, and export the shared key filename.
4. Run `make check` and Windows cross-build; update Review and Handoff.

## Out-of-scope change needed (if any)
- ~~Blocked: add `internal/app/app.go` to `paths`~~ → declined by lead, see Notes from lead.
  Original request: add `internal/app/app.go` to `paths` so the environment service can be constructed with the instance
  database, selected-project database accessor and secret box, then passed as `resolvers.Deps.Environment`.
  Service construction currently lives at `internal/app/app.go:140`; GraphQL dependencies are assembled at
  `internal/app/app.go:306` without `Environment`. Implementing only the allowed files would leave the running
  application's environment operations returning `NOT_IMPLEMENTED`. AGENTS.md hard rule 1 requires stopping here.
- Dependency verified: T-098 is `done` and merged on main (`d1db460`). Implementation proceeded after the lead assigned app wiring to T-109.

## Handoff (implementer)
- What changed: extracted `internal/secret.Box` and switched CA production code to it without changing `ca.Deps`
  or `ca.Open`. Key creation uses `O_EXCL`, reopens on `EEXIST`, and locks initialization/read with platform file locks so
  concurrent openers share one key, including when the file starts empty. Existing CA tests are unchanged;
  a test-only adapter retains their old package-local API, and an independent legacy ciphertext test verifies compatibility.
  Added Global/project CRUD, validation, atomic variable replacement, sealed secret storage, per-project selection,
  selected-over-Global lookup, byte-preserving one-pass interpolation, and thin GraphQL resolvers replacing the stubs.
  Existing TEXT columns hold base64 ciphertext; `ORDER BY rowid` preserves variable insertion order, so no migration was needed.
- How to verify: `make check` passed (Go vet/lint/race tests, Svelte/ESLint/Prettier, all 329 web tests);
  `make test-int` passed. New tests cover storage rollback after an actual SQL failure, raw secret rows in both databases,
  input limits and ordering, concurrent creation/selection, project isolation, interpolation edge cases, GraphQL CRUD/error
  codes, concurrent key initialization, empty/loose key files, authentication failures, and opening a legacy sealed CA.
  The first `make check` stopped because web dependencies were absent; `bun install --frozen-lockfile` resolved it without
  changing the lockfile. `git diff --check` passed.
- Known limitations: app wiring remains T-109, so the running binary still answers `NOT_IMPLEMENTED` until that task lands.
  No UI E2E run; API behavior is covered through GraphQL handler tests with real SQLite storage.
  Key locking uses `syscall.Flock` on Unix and `windows.LockFileEx` on Windows; closing the file releases either lock.
  Windows runtime behavior was not exercised on this Linux host.
- Follow-ups (T-109): call `secret.Open(root)` (report the returned loose-permission warning), then
  `environment.New(environment.Deps{Instance: inst, Project: projectService.DB, Secrets: box})` and supply the result as
  `resolvers.Deps.Environment`. The service uses `int64` IDs (`environment.GlobalID == 0`); GraphQL exposes Global as
  `"global"` and project IDs as decimal strings. `Vars(ctx) (map[string]string, error)` propagates storage/decryption errors;
  pass its successful result to `environment.Interpolate(raw, vars) []byte` in the Replay send pipeline (T-106).
  No new follow-up tasks proposed.

### Review fixes (implementer, 2026-11-19)
- Fixed all four findings: isolated Unix locking and added Windows `LockFileEx` (the review's alternative),
  retained serialization for legacy empty files, hoisted `tokenRE`, documented `[ \t\r\n]*`, and exported `secret.File`.
- Extended concurrent-open tests to run for both missing and empty key files.
- Verification: `make check` passed (including race tests and 329 frontend tests);
  `GOOS=windows CGO_ENABLED=0 go build ./...` and
  `GOOS=darwin CGO_ENABLED=0 go build ./internal/secret ./internal/ca` passed; `git diff --check` passed.
- No UI E2E or integration rerun for these review fixes; the previous implementation's integration run passed.
  Windows/macOS binaries were cross-built only, not executed.

### Re-review fix (implementer, 2026-11-19)
- Windows key locking now calls `LockFileEx` through the Go standard library. This preserves serialization for
  both missing and empty key files and removes the direct `golang.org/x/sys` import.
- `make check`, `go mod tidy -diff`, the Windows cross-build, and Windows vet for `internal/secret` passed.
  `make test-int` passed on a standalone rerun after the first run exceeded the proxy's latency budget.
- The Windows test binary ran the missing- and empty-file concurrency cases 10 times under Wine with a temporary
  overlay omitting the Unix-only file-mode assertion. The unmodified assertion also fails with the previous lock
  implementation under Wine. Native Windows and UI E2E were not run.

### Windows lint follow-up (implementer)
- Suppressed gosec G103 for the pointer conversion passed to `LockFileEx`, with a task-scoped justification.
  Windows-targeted lint for `internal/secret`, `make check`, and `git diff --check` passed.

## Review (reviewer)
- Verdict: changes_requested (reviewer, 2026-11-19)
- Checked: all 12 changed files are inside `paths`. `make check` is green (exit 0). `make test-int` was not run because
  proxy/store/rawhttp did not change. Every AC has a test (raw-row secret check in both DBs, rollback after a real SQL
  failure, Interpolate table incl. `{{{a}}}` and non-UTF-8 bytes, legacy CA ciphertext, concurrent key creation).
  Resolvers are thin and match the schema's error codes. SQL is parameterized. Project DB nil handling is the same as in
  `scope`/`filterpreset`.
- Findings:
  - [x] [major] internal/secret/secret.go:67 — `syscall.Flock`/`syscall.LOCK_EX` only exist on Unix, so
    `GOOS=windows go build ./...` now fails (`undefined: syscall.Flock`). It passes on `main`. CI only runs on
    Linux/macOS, so nothing catches this. No doc says Windows is unsupported; the Handoff only assumes it. → Fix:
    make key creation portable without a file lock. Write the key to a temp file in `root` (O_EXCL), `Sync`, then
    `root.Link(tmp, "secret.key")`; on `os.ErrExist`, read the existing file instead, and `root.Remove(tmp)` in both
    cases. Readers never see a half-written key. Keep the old "empty file → generate" path for legacy files. The other
    option is a `//go:build unix` file plus a Windows lock file. If the lead decides Windows is unsupported, record it
    in docs/03 and drop this to minor.
    Implementer: used the platform-lock alternative; Windows cross-build now passes, and concurrent empty-file coverage remains.
  - [x] [minor] internal/environment/service.go:366 — `regexp.MustCompile` runs on every `Interpolate` call. Replay and,
    later, Automate (thousands of requests) call it once per request. → Move it to a package-level
    `var tokenRE = regexp.MustCompile(...)`.
    Implementer: moved the compiled regex to package scope; existing interpolation table passes.
  - [x] [nit] internal/environment/service.go:366 — the token allows `\t\r\n` inside the braces, but the spec says
    "spaces". This works, but T-103/T-106 editors that mark unknown names must use the same rule. → Write the exact
    rule (`[ \t\r\n]*`) in the `Interpolate` doc comment so the frontend can copy it.
    Implementer: documented the exact whitespace rule.
  - [x] [nit] internal/ca/ca.go:114 — the hard-coded `"secret.key"` log value duplicates `secret.secretFile`. → Export
    `secret.File` and use it here and in `secret_compat_test.go`.
    Implementer: exported `secret.File` and switched both callers.

### Re-review (reviewer, 2026-11-19)
- Verdict: changes_requested
- Checked: all 4 earlier findings are fixed. All 14 changed files are inside `paths`. `make check` is green (exit 0,
  329 web tests). `GOOS=windows CGO_ENABLED=0 go build ./...` and `GOOS=windows go vet ./internal/secret` pass. The
  locking logic is right: `O_EXCL` create, then lock, then re-stat, so a reader that arrives before the first write still
  waits. The empty-file test covers this. `make test-int` was not rerun because proxy/store/rawhttp did not change.
- Findings:
  - [x] [major] internal/secret/lock_windows.go:6 — imports `golang.org/x/sys/windows`. That module is not on the
    approved list (docs/05 §2: "anything else needs an ADR"), so this breaks hard rule 8. Also, `go.mod` still marks
    it `// indirect`: `go mod tidy -diff` now fails on this branch but passes on `main`, and `go.mod` is outside
    `paths`. → Pick one: (a) use only the stdlib, with no lock. Write the new key to a temp file in `root` with
    `O_EXCL`, call `Sync`, then `root.Link(tmp, File)`. On `os.ErrExist`, read the existing key. Call `root.Remove(tmp)`
    in both cases. For a legacy empty `secret.key`, first `root.Remove(File)` (ignore `ErrNotExist`) and then take the
    same path. Readers never see a half-written key, and the code is the same on every OS. Or (b) write an
    "Out-of-scope change needed" request asking the lead to add `golang.org/x/sys` to docs/05 §2 and `go.mod` to
    `paths`, then run `go mod tidy`.
    Implementer: replaced the import with a standard-library call to `LockFileEx`. The lock protects legacy empty
    files from the stat/remove race where a second opener could remove a newly initialized key. `go mod tidy -diff` passes.

### Re-review 2 (reviewer, 2026-11-19)
- Verdict: approve
- Checked: the earlier major is fixed. `lock_windows.go` now uses only `syscall` + `unsafe`. `go.mod`/`go.sum` match
  `main`, and `go mod tidy -diff` passes. All 14 changed files are inside `paths`. `make check` is green (exit 0,
  329 web tests). `GOOS=windows CGO_ENABLED=0 go build ./...` and `GOOS=windows go vet ./internal/secret` pass.
  The `LockFileEx` call is correct. `os.OpenFile` returns a synchronous handle, so a zero `Overlapped` blocks and does
  not return `ERROR_IO_PENDING`. The `uintptr(unsafe.Pointer(..))` conversion sits inside the `Call` argument list,
  which is a valid `unsafe.Pointer` pattern. `kernel32` is a KnownDLL, so no DLL hijacking (a planted DLL getting
  loaded instead) is possible. Locking 1 byte past EOF on an empty legacy file is allowed on Windows. The lock is
  released when the handle closes. `make test-int` was not rerun because proxy/store/rawhttp did not change.
- Findings:
  - [x] [minor] internal/secret/lock_windows.go:16 — `GOOS=windows golangci-lint run ./internal/secret/...` reports
    `G103: Use of unsafe calls should be audited (gosec)`. CI only lints on Linux/macOS, so it stays hidden until
    someone lints for Windows. The stdlib has no public `LockFileEx` wrapper, so `unsafe` is required. → Add a
    justified suppression on that line: `//nolint:gosec // T-037: LockFileEx takes *OVERLAPPED; stdlib has no public wrapper.`
    It can go into this branch or into the next task that touches `internal/secret`.
    Implementer: added the justified suppression at the call site. Windows-targeted lint reports zero issues.

### Re-review 3 (reviewer, 2026-11-19)
- Verdict: approve
- Checked: the G103 minor is fixed. The only code change since Re-review 2 is the justified `//nolint:gosec` on
  `internal/secret/lock_windows.go:16`, with task id and reason (hard rule 7). All 14 changed files are inside `paths`.
  `make check` is green (exit 0, 329 web tests). `GOOS=windows CGO_ENABLED=0 go build ./...` passes,
  `GOOS=windows golangci-lint run ./internal/secret/...` reports 0 issues, and `go mod tidy -diff` is clean.
  Full pass over the diff again: SQL is parameterized; `env_vars` uses `ON DELETE CASCADE` and `foreign_keys(1)` is on,
  so deleting an environment also deletes its variables; `parsePresetID` rejects `"0"`, so a numeric ID cannot stand in
  for Global; `Select` clears all selections before setting one, so `Vars` sees at most one selected row; secret AAD
  includes the env id and variable name, so renaming an environment does not break its secrets. `make test-int` was not
  rerun because proxy/store/rawhttp did not change.
- Findings: none.
