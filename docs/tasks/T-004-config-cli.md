---
id: T-004
title: Config, CLI flags/env/TOML, app wiring, structured logging
milestone: M0
role: backend
status: in_progress
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
- [ ] Flags from F-CLI exist (unimplemented ones return a clear "not implemented yet" error), `ratatoskr version` prints version/commit.
- [ ] Precedence covered by table tests.
- [ ] SIGINT/SIGTERM triggers graceful shutdown within 5 s (test with `signal.NotifyContext`, synctest where possible).
- [ ] Ring buffer keeps last 10k lines; `slog.NewMultiHandler` used.
- [ ] Data dir created with 0700 and accessed via `os.Root`.
- [ ] `--debug` mounts `/debug/pprof` (incl. goroutineleak) on the UI server only.

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

## Out-of-scope change needed (if any)

## Handoff (implementer)

## Review (reviewer)
