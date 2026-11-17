---
id: T-004
title: Config, CLI flags/env/TOML, app wiring, structured logging
milestone: M0
role: backend
status: in_review
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

## Plan (round 2, addressing review)

- [major] `--listen` becomes repeatable: `Config.Listen []string`, a `listenValue` (flag.Value) that replaces the default on the
  first set, appends afterwards and accepts a comma-separated list for `RATATOSKR_LISTEN`; each address validated. `app.New`
  gates `len > 1` with `ErrNotImplemented` ("multiplexer lands with T-011"); `Run` uses the first address. Table cases added.
- [major] Ring snapshots every value in `Handle`/`WithAttrs`: `KindAny` becomes `slog.StringValue(v.String())`, so no caller
  object stays referenced and the byte budget is honest. Test logs a mutable `*struct`/`[]byte`, mutates it, checks the entry.
- [minor] `qualify` → `flatten`: groups are inlined with dotted keys, empty-key groups are inlined without a prefix, zero attrs
  are dropped. Tests for nested groups, empty-key groups and `slog.LogValuer` groups.
- [minor] `--data-dir` default is filled lazily: only when the flag/env stayed unset is `os.UserConfigDir()` consulted, so an
  explicit dir starts without `$HOME`. Test with `HOME=""`/`XDG_CONFIG_HOME=""`.
- [minor] `config.toml` is read through `os.Root`: `ff.WithFilesystem(root.FS())` + `WithConfigFile(ConfigFileName)`; a missing
  data dir means no file. Test: a `config.toml` symlink escaping the dir is refused.
- [minor] `slog.SetDefault(logger)` in `cmd/ratatoskr`; [nit] `/debug/pprof/symbol` registered method-less, `cmdline` gets a
  `TODO(T-007)`; [nit] the "pprof off" case also asserts the body is not pprof output.

## Out-of-scope change needed (if any)

None. `web/node_modules` was missing in this worktree, so `make check`'s web targets failed before any change; I ran
`bun install --frozen-lockfile` (no tracked file changed) to get a full green `make check`.

## Handoff (implementer)

### What changed
- `internal/config` (new): `Config` (Listen `[]string`, UIListen, DataDir, NoOpen, AuthToken, Debug, AllowGuests),
  `Load(args)`, `Validate`, `Usage(w)`, redacting `LogValue`. Built on `github.com/peterbourgon/ff/v4` (approved list) with
  `fftoml`; precedence flags > `RATATOSKR_*` > `config.toml` > defaults, verified by a table test. Config keys accept `_`
  for `-` (`data_dir` == `data-dir`). Unknown flags, unknown config keys, stray arguments, malformed addresses and an
  empty data dir are errors. `Load` wraps `ff.ErrHelp` so `--help` prints usage. `config.toml` is read through the data
  dir's `os.Root`; the `--data-dir` default is only looked up when neither flag nor env names a dir.
- `internal/config` — `--listen` is repeatable (`listenValue`): the first value replaces the default, later values append,
  a comma-separated value names several addresses (that is how `RATATOSKR_LISTEN` does it) and each address is validated;
  it implements `ff.Resetter` so ff's reset cannot append duplicates. TOML accepts a string or an array.
- `internal/logging` (new): `New(w, level)` returns `slog.NewMultiHandler(text→w, ring)`; `Ring` is a bounded, mutex-guarded
  store of the last `DefaultCapacity` (10k) entries inside an 8 MiB budget, truncating single entries at 4 KiB
  (`Entry.Truncated`), with `Entries()`/`Len()`/`Clear()` for the Logs page and stdlib-identical `With`/`WithGroup`
  semantics. Attrs are flattened and snapshotted when the record is handled (see `flatten`), so the ring holds no
  reference a caller can still mutate and the byte budget measures what is actually stored.
- `internal/app` (new): `New(Options)` validates the config, gates unimplemented options (`--auth-token`, `--ui-listen`,
  more than one `--listen`) with `ErrNotImplemented`, creates the data dir 0700 and keeps it as an `*os.Root`
  (`DataRoot()`); warns when the dir is group/world accessible and when the listener is not loopback. `Run(ctx)`/
  `Serve(ctx, ln)` serve the embedded SPA (`internal/api/static`) plus `/debug/pprof` (all runtime profiles, so
  `goroutineleak` included) only with `--debug`, and drain in-flight requests for at most 5 s.
- `cmd/ratatoskr`: rewritten around `run(ctx, args, stdout, stderr)`; `notifyContext()` = `signal.NotifyContext(SIGINT, SIGTERM)`;
  `version` subcommand prints `ratatoskr <version> (commit <sha[-dirty]>)` (commit from build info, no Makefile change needed);
  `ca export|regenerate` and `project list|backup|restore` return `app.ErrNotImplemented`. The process logger is installed
  with `slog.SetDefault` so anything logging through `slog.Default()` lands in the ring. `server.go` (placeholder page) deleted.
- `go.mod`/`go.sum`: added `github.com/peterbourgon/ff/v4 v4.0.0-beta.1` (+ `pelletier/go-toml/v2`, its TOML parser), tidied.

### Decisions worth a reviewer's eye
- `--listen` is the **shared proxy + UI/API listener** (default `127.0.0.1:8080`), matching architecture §4 (origin-form →
  UI on the same listener) and T-011's "listener multiplexing (proxy vs UI)". It is repeatable per §I, but more than one
  address needs the multiplexer, so `app.New` gates it instead of silently serving only the first.
  `--ui-listen` needs the same multiplexer and is gated as not implemented.
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
go run ./cmd/ratatoskr --listen 127.0.0.1:0 --listen 127.0.0.1:1   # -> not implemented yet (multiplexer = T-011)
env -u HOME -u XDG_CONFIG_HOME go run ./cmd/ratatoskr --data-dir /tmp/rk --listen 127.0.0.1:0   # starts
go run ./cmd/ratatoskr --data-dir /tmp/rk --listen 127.0.0.1:0 --debug   # pprof on /debug/pprof/
```
Tests: `TestLoad` (precedence table, 24 cases incl. repeated `--listen`, comma-separated env and a TOML array),
`TestLoadHelp`, `TestLoadWithoutHome`, `TestLoadConfigFileThroughRoot`, `TestListenValueReset`, `TestValidate`,
`TestLogValueRedactsAuthToken`, `TestUsage`, `TestNewFansOutToWriterAndRing`, `TestRingKeepsLastEntries`,
`TestRingByteBudgetEvictsOldest`, `TestRingTruncatesOversizedEntry`, `TestRingAttrsAndGroups`, `TestRingFlattensGroups`,
`TestRingFlattensLogValuer`, `TestRingSnapshotsValues`, `TestRingConcurrentWrites`, `TestNewRejectsUnimplementedOptions`,
`TestNewCreatesDataDir`, `TestHandler` (incl. POST to `/debug/pprof/symbol`), `TestServeGracefulShutdownDrains`,
`TestServeShutdownDeadline`, `TestWarnIfExposed`, `TestRunSubcommands`, `TestRunRejectsSeveralListeners`,
`TestNotifyContextOnSIGTERM`.

### Known limitations / follow-ups
- T-007: wire `App.Logs()` into a GraphQL logs query/subscription and add the auth + CSRF middleware (then un-gate
  `--auth-token`/`--allow-guests`); extend `App.buildHandler` with the API routes. `/debug/pprof/cmdline` carries a
  `TODO(T-007)` to be wrapped in that middleware (it exposes `--auth-token` from argv).
- T-011: replace the single `http.Server` in `App.Serve` with the proxy/UI multiplexer, implement `--ui-listen` and serve
  the extra `--listen` addresses (both are gated with `ErrNotImplemented` today).
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
- [x] [major] internal/config/config.go:224-228 — F-CLI says `ratatoskr [--listen addr]…`, so the flag can be repeated
  (docs/01 §I). Here it is one `string`, and a second `--listen` silently replaces the first. That breaks the AC
  "unimplemented ones return a clear 'not implemented yet' error" and hard rule 2 (no silent product behaviour). → Turn
  `Listen` into `[]string` (`ffval.NewList`, default `[DefaultListen]`, the env/TOML list forms) and validate each entry.
  Then do one of two things: gate `len(cfg.Listen) > 1` in `app.New` with `ErrNotImplemented` ("multiple listeners land
  with T-011"), or serve one `http.Server` per listener. Either way, add a precedence/validation case for the repeated
  flag. If you think the spec doesn't mean repeatable, set `blocked` and ask the lead instead of picking one.
  → **Fixed.** `Config.Listen` is `[]string`; `listenValue` (a `flag.Value`) replaces the default on the first value,
  appends afterwards, accepts a comma-separated list so one env var names several addresses, validates each entry and
  implements `ff.Resetter` (ff would otherwise append the default back). TOML takes a string or an array. `app.New` gates
  `len > 1` with `ErrNotImplemented` (multiplexer = T-011) and `Run` uses the first address. Cases added: repeated flag,
  comma-separated env, TOML array, a bad address among several, `ui-listen` equal to a repeated `listen`, `app.New` gate,
  `run` gate, `Validate` with several listeners, `TestListenValueReset`.
- [x] [major] internal/logging/ring.go:156-174, 210-212 — `Handle` keeps `slog.Value`s of `KindAny` (and the `LogValuer`
  results) **by reference** inside the ring until they are evicted (possibly 10k records later). This causes two problems:
  (a) the 8 MiB budget isn't really enforced, because `attrSize` counts `len(Value.String())` while the ring keeps the
  whole object graph alive (a struct pointer, a `[]byte` alias, a request record); (b) when the Logs page (T-007) reads
  `Entries()` and formats them, it reads objects that callers may still be changing. That is a data race the current
  tests can't catch, because they only log strings and ints. → Take a snapshot at `Handle` time: for
  `KindAny`/`KindGroup`, store `slog.StringValue(v.String())` (or recursively flatten groups, see the next finding). Then
  compute the size once from the stored value. Add a test that logs a mutable `*struct`/`[]byte`, changes it after the
  call, and checks the stored entry hasn't changed (run with `-race`).
  → **Fixed.** `flatten` renders every `KindAny` value to `slog.StringValue(v.String())` at Handle/WithAttrs time, so the
  ring never holds a caller's object (groups are recursed into, so their values are snapshotted too). `attrSize` now
  measures exactly what is stored. `TestRingSnapshotsValues` logs a `*struct` with a `[]byte` field, mutates both after the
  call, asserts the entry is unchanged and that no attr is left as `KindAny` (runs under `-race`).
- [x] [minor] internal/logging/ring.go:24-25, 209-220 — The `Entry` doc says attrs are "resolved and flattened" (`g.key`),
  but group-valued attrs aren't flattened. `slog.Group(...)` and `Config.LogValue()` (logged as `"config"` at
  cmd/ratatoskr/main.go:76 and app.go:128) end up as one `KindGroup` attr. The slog handler rules also aren't followed:
  an empty `Attr{}` should be dropped, and a group with an empty key should be inlined. → Flatten groups recursively in
  `qualify` (prefix `config.listen`, … the same way the text handler already prints them), drop zero attrs, inline
  empty-key groups. Extend `TestRingAttrsAndGroups` to cover these, or use `testing/slogtest` against the ring handler.
  → **Fixed.** `qualify` became `flatten`: groups are inlined recursively with dotted keys (`config.listen`, `req.hdr.host`),
  an empty-key group is inlined without a prefix, a zero attr is dropped and an empty group contributes nothing.
  `TestRingFlattensGroups` covers nesting, empty-key/empty groups and a `WithGroup`-scoped group; `TestRingFlattensLogValuer`
  covers a `slog.LogValuer` returning a group (what `Config.LogValue` does).
- [x] [minor] internal/config/config.go:122-126, 192-196 — `resolveDataDir` calls `os.UserConfigDir()` even when
  `--data-dir`/`RATATOSKR_DATA_DIR` is set explicitly, so the binary can't start without `$HOME` (containers, systemd
  units, CI) even when it's given a path. → Only use the default when the lookup fails *and* no explicit dir was given.
  For example, use `""` as the flag default and fill in `defaultDataDir()` only when the parsed value is empty. Add a
  test with `t.Setenv("HOME", "")`/`XDG_CONFIG_HOME=""`.
  → **Fixed.** `resolveDataDir` parses with an empty `--data-dir` default and calls `defaultDataDir()` only when the flag
  and the env stayed unset (`ff.Flag.IsSet`), so `--data-dir ""` still errors instead of falling back.
  `TestLoadWithoutHome` runs with `HOME=""`/`XDG_CONFIG_HOME=""` and covers: no dir at all (lookup error), `--help`
  (works), `--data-dir` and `RATATOSKR_DATA_DIR` (both start).
- [x] [minor] internal/config/config.go:278 — config.toml is read by ff through `os.Open(filepath.Join(dir, ...))`, not
  through `os.Root`. That goes against AC "data dir … accessed via `os.Root`" and docs/05 §1 ("os.Root for every file
  access under the data dir"). → Pass `ff.WithFilesystem(root.FS())` with `ff.WithConfigFile(ConfigFileName)`, opening
  the root with `os.OpenRoot(dir)` (allow `ErrNotExist` for a missing dir). Or move the read into `app` after
  `openDataRoot` and hand the bytes to `fftoml`.
  → **Fixed.** `Load` opens the data dir with `os.OpenRoot` (`openConfigRoot`; a missing dir means no config file) and
  passes `ff.WithFilesystem(root.FS())` + `ff.WithConfigFile(ConfigFileName)`. `TestLoadConfigFileThroughRoot` makes
  `config.toml` a symlink out of the dir and asserts the read is refused
  (`config: openat config.toml: path escapes from parent`).
- [x] [minor] cmd/ratatoskr/main.go:75-78 — The logger from `logging.New` isn't installed as `slog.SetDefault`. Anything
  that logs via `slog.Default()` (stdlib, future packages, and `app.New`'s own nil-logger fallback) skips the ring, so the
  Logs page won't see it. → `slog.SetDefault(logger)` right after `logging.New`.
  → **Fixed.** `run` calls `slog.SetDefault(logger)` right after `logging.New`;
  `TestRunServesAndStopsOnContextCancel` asserts a record logged through `slog.Default()` reaches the log writer.
- [x] [nit] internal/app/pprof.go:276 — `GET /debug/pprof/symbol` refuses `POST`, which `go tool pprof` uses to resolve
  symbols. → Register it method-less (`/debug/pprof/symbol`), as `net/http/pprof` itself does.
  → **Fixed, with a deviation.** A method-less `/debug/pprof/symbol` panics at registration: it matches more methods than
  `GET /debug/pprof/` while being a more specific path, which `ServeMux` reports as a conflict. Making *every* pprof route
  method-less (as `net/http/pprof` does on `DefaultServeMux`) would widen the other profiles to all methods. So `symbol`
  is registered twice, `GET` and `POST`, which keeps the rest GET-only. `TestHandler` posts to it.
- [x] [nit] internal/app/pprof.go:275 — `/debug/pprof/cmdline` will expose `--auth-token` from argv once T-007 un-gates
  it, on a server that `--debug` can bind to a non-loopback address. → Add a `TODO(T-007)` saying auth must wrap
  `/debug/pprof`, or leave out `cmdline`.
  → **Fixed.** `mountDebug` carries `TODO(T-007): wrap /debug/pprof in the auth middleware (or drop cmdline)` next to the
  `cmdline` route.
- [x] [nit] internal/app/app_test.go:194 — "pprof is off by default" accepts 200 because the SPA fallback serves
  index.html, so the only check is `notBody: "goroutineleak"`. → Also assert that the body is *not* pprof output (for
  example, that it lacks `Types of profiles available`), so an accidental mount can't slip through if the SPA ever
  contains that word.
  → **Fixed.** `notBody` is a list and the default case rejects both `goroutineleak` and `Types of profiles available`.

### What's good
Precedence table is thorough (20 cases incl. explicit-false-beats-env). Rejecting `data_dir` in config.toml is well
reasoned and documented as a plan deviation. Gating `--auth-token` is the right security call. The shutdown path
(`WithoutCancel` + grace + `Close` fallback) is correct and its tests are channel-driven, with no sleeps. The redacting
`LogValue` is a nice touch. The ring's eviction and UTF-8-safe truncation are correct and race-tested.

**Verdict: `changes_requested`.** Fix the two majors (repeatable `--listen`, ring retaining live references). The minors
should go in the same pass, especially `os.Root` for config.toml, because it's part of an AC.

### Round 2

**Reviewer:** claude-code/reviewer · 2026-11-18 · commits `2528596..f1e2edd`

**Scope:** every changed file is still inside `paths`. No dependency changes in this round. No schema or generated files
touched. No Caido material.

**`make check`:** green. go fix diff is empty, vet passes, golangci-lint reports `0 issues.`, `go test -race ./...` passes
(cmd/ratatoskr, app, config, logging), svelte-check/eslint/prettier report 0 errors, vitest 37/37 passed.
`make test-int` not run: proxy/store/rawhttp are untouched.

**Round-1 findings:** all 9 verified as fixed, both in the code and with a test for each:
- Repeatable `--listen`: `listenValue` plus the `ff.Resetter` handling. `app.New` gates more than one address.
- Ring snapshot: `flatten` turns `KindAny` into a string. `TestRingSnapshotsValues` mutates the logged value and runs
  under `-race`.
- Group flattening follows the slog rules: zero attrs are dropped and empty-key groups are inlined.
- The data-dir default is now looked up lazily.
- `config.toml` is read through `os.Root`.
- `slog.SetDefault` is called.
- pprof `symbol` answers both GET and POST. Registering it method-less would conflict with `GET /debug/pprof/` under
  the Go 1.22+ mux rules, which I checked, so this deviation is correct.
- `cmdline` has its `TODO(T-007)`.
- The "pprof off" test asserts more strongly.

**Manual probes (built binary):**
- `--listen a --listen b` and `RATATOSKR_LISTEN="a, b"` both exit 1 with `2 --listen addresses: not implemented yet (… T-011)`.
- `env -u HOME -u XDG_CONFIG_HOME` with `--data-dir` starts and stops cleanly. `--help` also works without `$HOME`.
- `data_dir` in config.toml gives a clear refusal.
- A TOML `listen = [..]` array works.
- `--listen` beats a TOML `listen`.
- `RATATOSKR_DATA_DIR=` gives `data_dir must not be empty`.
- `--listen foo` and `--listen "127.0.0.1:0,"` give clear errors.

### Findings (round 2)
- [ ] [minor] cmd/ratatoskr/main_test.go:200-201 — `previous := slog.Default()` runs *after* `run` has already called
  `slog.SetDefault`. The cleanup therefore restores the test's own logger, and the process default stays pointed at
  this test's `bytes.Buffer` for the rest of the package. The parallel `run` tests (`TestRunRejectsUnimplementedAuthToken`,
  `TestRunRejectsSeveralListeners`) also call `SetDefault` with their own unsynchronized buffers. As soon as any
  goroutine logs through `slog.Default()` or the stdlib `log` package, two tests can write into one `bytes.Buffer` and
  race. → Capture `previous` before calling `run` and restore it in `t.Cleanup`. Give every test that reaches
  `logging.New` (all `run` calls that get past `config.Load`) the same save/restore through a small helper. Since the
  default is process-global, also drop `t.Parallel()` from those tests, or give `run` an option that skips `SetDefault`.
- [ ] [nit] internal/config/config.go:335 — An error from `listenValue.Set` carries the `config:` prefix twice:
  `config: parse args: --listen: set "foo": config: listen: address foo: …`. → Let `Set` return the error from
  `validateAddr` without the `config:` prefix. For example, split `validateAddr` into a prefix-free check, and have
  `Validate` wrap it.
- [ ] [nit] internal/config/config_test.go:396-399 — `TestLoadConfigFileThroughRoot` passes on *any* error, including
  one unrelated to `os.Root`. → Also assert `strings.Contains(err.Error(), "escapes")` (the `os.Root` message is
  `path escapes from parent`).
- [ ] [nit] internal/logging/ring.go:245-246 — `a.Value.String()` renders a `KindAny` value in full before `fit`
  truncates it to 4 KiB. A large `[]byte` or struct therefore costs a full-size temporary allocation on every log call.
  The text handler does the same, so this is not a regression. → Optional: render into a writer capped at
  `maxEntryBytes` (`fmt.Fprint` into a bounded writer). Or leave it and document it on `Entry`.

### What's good
- The fixes are well targeted.
- `listenValue` handles ff's reset semantics, which is the subtle part.
- `resolveDataDir` uses `IsSet`, so `--data-dir ""` still errors instead of falling back.
- The handoff documents the `symbol` deviation with the correct reason.
- The tests are still channel-driven, with no sleeps.

**Verdict: `approve`.** No blocker or major findings remain. Fix the minor (test hygiene around the global
`slog.Default`) before merge, or in an early follow-up; the nits are optional. Status stays `in_review` for the operator
to merge and set `done`.
