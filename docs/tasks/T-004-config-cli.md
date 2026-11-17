---
id: T-004
title: Config, CLI flags/env/TOML, app wiring, structured logging
milestone: M0
role: backend
status: changes_requested
depends_on: [T-002]
owner: "opencode/deepseek-v4.1-flash-1017"
branch: task/T-004-config-cli
paths:
  - cmd/ratatoskr/**
  - internal/config/**
  - internal/app/**
  - internal/logging/**
  - go.mod
  - go.sum
spec:
  - docs/01-feature-spec.md#i-cli-f-cli
  - docs/03-architecture.md §2, §9
  - docs/05-go-conventions.md §1, §3
---

## Goal
`ratatoskr` starts from a validated `config.Config` built from flags, `RATATOSKR_*` env vars and `<data-dir>/config.toml` (precedence flags > env > file > defaults),
wires an `app.App` with graceful shutdown, and logs via slog to stderr + an in-memory ring buffer.

## Acceptance criteria
- [x] Flags from F-CLI exist (unimplemented ones return a clear "not implemented yet" error), `ratatoskr version` prints version/commit.
- [x] Precedence covered by table tests.
- [x] SIGINT/SIGTERM triggers graceful shutdown within 5 s (test with `signal.NotifyContext`, synctest where possible).
- [x] Ring buffer keeps last 10k lines; `slog.NewMultiHandler` used.
- [x] Data dir created with 0700 and accessed via `os.Root`.
- [x] `--debug` mounts `/debug/pprof` (incl. goroutineleak) on the UI server only.

## Plan (implementer, ≤15 lines)

- `internal/config`: `Config` + `Load(args)` on `ff/v4` (flags > `RATATOSKR_*` > `<data-dir>/config.toml` > defaults); TOML keys
  accept `-` and `_`; the data dir is re-resolved once when `config.toml` points elsewhere (cycle-capped); `Validate`; redacting `LogValue`.
- Flags: `--listen` (shared proxy+UI listener, default `127.0.0.1:8080`), `--ui-listen`, `--data-dir`, `--no-open`, `--auth-token`,
  `--debug`, `--allow-guests`; `--help` rendered with `ffhelp`.
- `internal/logging`: `New(w, level)` = `slog.NewMultiHandler(text→w, ring)`; `Ring` keeps the last 10k entries inside an 8 MiB
  budget with per-entry truncation and `Entries()` for the Logs page (T-007 wires the API).
- `internal/app`: `New` refuses options that need later tasks (`--auth-token`, `--ui-listen`) with a clear "not implemented yet",
  creates the data dir 0700 and holds an `*os.Root`; `Run`/`Serve(ctx, ln)` serve the embedded SPA (+ `/debug/pprof` incl.
  goroutineleak when `--debug`), warn on a non-loopback bind, drain in-flight requests within 5 s.
- `cmd/ratatoskr`: `version` subcommand (version + build-info commit), `ca`/`project` subcommands return "not implemented yet",
  `signal.NotifyContext` for SIGINT/SIGTERM; the placeholder page goes away (SPA lands in T-003).
- Tests: precedence table, TOML re-resolution, ring capacity/truncation/race, shutdown drain + 5 s cap, signal test, subcommands.

**Plan deviation:** the config file may *not* set `data_dir`. `config.toml` lives inside the data dir, so it cannot
relocate itself; `Load` resolves the dir from `--data-dir`/`RATATOSKR_DATA_DIR`/default first, then reads the file, and
rejects a `data_dir` key with an explicit error instead of silently ignoring it (ff's precedence would otherwise drop it).

## Out-of-scope change needed (if any)

None. `web/node_modules` was missing in this worktree, so `make check`'s web targets failed before any change; I ran
`bun install --frozen-lockfile` (no tracked file changed) to get a full green `make check`.

## Handoff (implementer)

### What changed
- `internal/config` (new): `Config` (Listen, UIListen, DataDir, NoOpen, AuthToken, Debug, AllowGuests), `Load(args)`,
  `Validate`, `Usage(w)`, redacting `LogValue`. Built on `github.com/peterbourgon/ff/v4` (approved list) with `fftoml`;
  precedence flags > `RATATOSKR_*` > `config.toml` > defaults, verified by a table test. Config keys accept `_` for `-`
  (`data_dir` == `data-dir`). Unknown flags, unknown config keys, stray arguments, malformed addresses and an empty data
  dir are errors. `Load` wraps `ff.ErrHelp` so `--help` prints usage.
- `internal/logging` (new): `New(w, level)` returns `slog.NewMultiHandler(text→w, ring)`; `Ring` is a bounded, mutex-guarded
  store of the last `DefaultCapacity` (10k) entries inside an 8 MiB budget, truncating single entries at 4 KiB
  (`Entry.Truncated`), with `Entries()`/`Len()`/`Clear()` for the Logs page and stdlib-identical `With`/`WithGroup` semantics.
- `internal/app` (new): `New(Options)` validates the config, gates unimplemented options, creates the data dir 0700 and keeps
  it as an `*os.Root` (`DataRoot()`); warns when the dir is group/world accessible and when the listener is not loopback.
  `Run(ctx)`/`Serve(ctx, ln)` serve the embedded SPA (`internal/api/static`) plus `/debug/pprof` (all runtime profiles, so
  `goroutineleak` included) only with `--debug`, and drain in-flight requests for at most 5 s (`ErrNotImplemented` sentinel).
- `cmd/ratatoskr`: rewritten around `run(ctx, args, stdout, stderr)`; `notifyContext()` = `signal.NotifyContext(SIGINT, SIGTERM)`;
  `version` subcommand prints `ratatoskr <version> (commit <sha[-dirty]>)` (commit from build info, no Makefile change needed);
  `ca export|regenerate` and `project list|backup|restore` return `app.ErrNotImplemented`. `server.go` (placeholder page) deleted.
- `go.mod`/`go.sum`: added `github.com/peterbourgon/ff/v4 v4.0.0-beta.1` (+ `pelletier/go-toml/v2`, its TOML parser), tidied.

### Decisions worth a reviewer's eye
- `--listen` is the **shared proxy + UI/API listener** (default `127.0.0.1:8080`), matching architecture §4 (origin-form →
  UI on the same listener) and T-011's "listener multiplexing (proxy vs UI)". `--ui-listen` therefore needs that multiplexer
  and is gated as not implemented.
- `--auth-token` is **rejected** rather than ignored: silently accepting it would leave the UI open while the operator
  believes it is protected (§8). `--no-open` and `--allow-guests` are accepted as no-ops because they already describe the
  current behaviour (no browser is opened, no auth exists so every client is a guest).
- The former `--version` flag is gone in favour of the spec'd `ratatoskr version` subcommand.
- `synctest` is not used: every timing test here needs a real `net.Listener`/`http.Client` (an in-memory `httptest` server
  cannot wrap an externally created listener), and no test sleeps — shutdown is driven by channels, the 5 s cap is asserted
  with a shortened grace.

### How to verify
```
make check                                   # go fix diff empty, vet, lint 0 issues, -race tests, web check+test
go test -race ./internal/config/ ./internal/logging/ ./internal/app/ ./cmd/ratatoskr/
go run ./cmd/ratatoskr version
go run ./cmd/ratatoskr --help
go run ./cmd/ratatoskr ca export             # -> ratatoskr: ca export: not implemented yet (exit 1)
go run ./cmd/ratatoskr --data-dir /tmp/rk --listen 127.0.0.1:0 --debug   # pprof on /debug/pprof/
```
Tests: `TestLoad` (precedence table, 20 cases), `TestValidate`, `TestLogValueRedactsAuthToken`, `TestUsage`,
`TestNewFansOutToWriterAndRing`, `TestRingKeepsLastEntries`, `TestRingByteBudgetEvictsOldest`,
`TestRingTruncatesOversizedEntry`, `TestRingAttrsAndGroups`, `TestRingConcurrentWrites`, `TestNewRejectsUnimplementedOptions`,
`TestNewCreatesDataDir`, `TestHandler`, `TestServeGracefulShutdownDrains`, `TestServeShutdownDeadline`,
`TestWarnIfExposed`, `TestRunSubcommands`, `TestNotifyContextOnSIGTERM`.

### Known limitations / follow-ups
- T-007: wire `App.Logs()` into a GraphQL logs query/subscription and add the auth + CSRF middleware (then un-gate
  `--auth-token`/`--allow-guests`); extend `App.buildHandler` with the API routes.
- T-011: replace the single `http.Server` in `App.Serve` with the proxy/UI multiplexer and implement `--ui-listen`.
- T-006/T-008: implement the `project` and `ca` subcommands (currently `app.ErrNotImplemented`).
- Log output is text on stderr only (no file/rotation, no JSON switch); the ring is in-memory and lost on exit, as specified.
- `internal/legacy` still pulls the old Hetty dependencies in `go.mod`; removing them belongs to the task that deletes
  `internal/legacy` (not T-004).

## Review (reviewer)

**Reviewer:** claude-code/reviewer · 2026-11-18 · commits `4f3a8b5..ccff323`

**Scope:** every changed file is inside `paths` (cmd/ratatoskr, internal/{config,app,logging}, go.mod/go.sum, the task file).
`ff/v4` is on the approved list (docs/05 §2); `pelletier/go-toml/v2` comes in as its indirect TOML parser. No schema/generated
files touched. No Caido material. Nothing goes through `net/http` on the proxy path (only the UI server uses it, which is allowed).

**`make check`:** green. go fix diff is empty, vet passes, golangci-lint reports `0 issues.`, `go test -race ./...` passes
(config, app, logging, cmd/ratatoskr), svelte-check/eslint/prettier report 0 errors, vitest 37/37 passed.
`make test-int` not run: proxy/store/rawhttp are untouched.

**Manual probes:**
- `--listen 127.0.0.1:0 --listen 127.0.0.1:0` starts without complaint and the last value wins. See the first major below.
- `env -u HOME -u XDG_CONFIG_HOME ratatoskr --data-dir <dir>` (and even `--help`) fails with
  `config: locate user config dir: neither $XDG_CONFIG_HOME nor $HOME are defined`.
- `debug = "yes"` in config.toml returns a clear parse error. Good.

### Findings
- [ ] [major] internal/config/config.go:224-228 — F-CLI says `ratatoskr [--listen addr]…`, so the flag can be repeated
  (docs/01 §I). Here it is one `string`, and a second `--listen` silently replaces the first. That breaks the AC
  "unimplemented ones return a clear 'not implemented yet' error" and hard rule 2 (no silent product behaviour). → Turn
  `Listen` into `[]string` (`ffval.NewList`, default `[DefaultListen]`, the env/TOML list forms) and validate each entry.
  Then do one of two things: gate `len(cfg.Listen) > 1` in `app.New` with `ErrNotImplemented` ("multiple listeners land
  with T-011"), or serve one `http.Server` per listener. Either way, add a precedence/validation case for the repeated
  flag. If you think the spec doesn't mean repeatable, set `blocked` and ask the lead instead of picking one.
- [ ] [major] internal/logging/ring.go:156-174, 210-212 — `Handle` keeps `slog.Value`s of `KindAny` (and the `LogValuer`
  results) **by reference** inside the ring until they are evicted (possibly 10k records later). This causes two problems:
  (a) the 8 MiB budget isn't really enforced, because `attrSize` counts `len(Value.String())` while the ring keeps the
  whole object graph alive (a struct pointer, a `[]byte` alias, a request record); (b) when the Logs page (T-007) reads
  `Entries()` and formats them, it reads objects that callers may still be changing. That is a data race the current
  tests can't catch, because they only log strings and ints. → Take a snapshot at `Handle` time: for
  `KindAny`/`KindGroup`, store `slog.StringValue(v.String())` (or recursively flatten groups, see the next finding). Then
  compute the size once from the stored value. Add a test that logs a mutable `*struct`/`[]byte`, changes it after the
  call, and checks the stored entry hasn't changed (run with `-race`).
- [ ] [minor] internal/logging/ring.go:24-25, 209-220 — The `Entry` doc says attrs are "resolved and flattened" (`g.key`),
  but group-valued attrs aren't flattened. `slog.Group(...)` and `Config.LogValue()` (logged as `"config"` at
  cmd/ratatoskr/main.go:76 and app.go:128) end up as one `KindGroup` attr. The slog handler rules also aren't followed:
  an empty `Attr{}` should be dropped, and a group with an empty key should be inlined. → Flatten groups recursively in
  `qualify` (prefix `config.listen`, … the same way the text handler already prints them), drop zero attrs, inline
  empty-key groups. Extend `TestRingAttrsAndGroups` to cover these, or use `testing/slogtest` against the ring handler.
- [ ] [minor] internal/config/config.go:122-126, 192-196 — `resolveDataDir` calls `os.UserConfigDir()` even when
  `--data-dir`/`RATATOSKR_DATA_DIR` is set explicitly, so the binary can't start without `$HOME` (containers, systemd
  units, CI) even when it's given a path. → Only use the default when the lookup fails *and* no explicit dir was given.
  For example, use `""` as the flag default and fill in `defaultDataDir()` only when the parsed value is empty. Add a
  test with `t.Setenv("HOME", "")`/`XDG_CONFIG_HOME=""`.
- [ ] [minor] internal/config/config.go:278 — config.toml is read by ff through `os.Open(filepath.Join(dir, ...))`, not
  through `os.Root`. That goes against AC "data dir … accessed via `os.Root`" and docs/05 §1 ("os.Root for every file
  access under the data dir"). → Pass `ff.WithFilesystem(root.FS())` with `ff.WithConfigFile(ConfigFileName)`, opening
  the root with `os.OpenRoot(dir)` (allow `ErrNotExist` for a missing dir). Or move the read into `app` after
  `openDataRoot` and hand the bytes to `fftoml`.
- [ ] [minor] cmd/ratatoskr/main.go:75-78 — The logger from `logging.New` isn't installed as `slog.SetDefault`. Anything
  that logs via `slog.Default()` (stdlib, future packages, and `app.New`'s own nil-logger fallback) skips the ring, so the
  Logs page won't see it. → `slog.SetDefault(logger)` right after `logging.New`.
- [ ] [nit] internal/app/pprof.go:276 — `GET /debug/pprof/symbol` refuses `POST`, which `go tool pprof` uses to resolve
  symbols. → Register it method-less (`/debug/pprof/symbol`), as `net/http/pprof` itself does.
- [ ] [nit] internal/app/pprof.go:275 — `/debug/pprof/cmdline` will expose `--auth-token` from argv once T-007 un-gates
  it, on a server that `--debug` can bind to a non-loopback address. → Add a `TODO(T-007)` saying auth must wrap
  `/debug/pprof`, or leave out `cmdline`.
- [ ] [nit] internal/app/app_test.go:194 — "pprof is off by default" accepts 200 because the SPA fallback serves
  index.html, so the only check is `notBody: "goroutineleak"`. → Also assert that the body is *not* pprof output (for
  example, that it lacks `Types of profiles available`), so an accidental mount can't slip through if the SPA ever
  contains that word.

### What's good
Precedence table is thorough (20 cases incl. explicit-false-beats-env). Rejecting `data_dir` in config.toml is well
reasoned and documented as a plan deviation. Gating `--auth-token` is the right security call. The shutdown path
(`WithoutCancel` + grace + `Close` fallback) is correct and its tests are channel-driven, with no sleeps. The redacting
`LogValue` is a nice touch. The ring's eviction and UTF-8-safe truncation are correct and race-tested.

**Verdict: `changes_requested`.** Fix the two majors (repeatable `--listen`, ring retaining live references). The minors
should go in the same pass, especially `os.Root` for config.toml, because it's part of an AC.
