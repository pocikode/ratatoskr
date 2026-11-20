---
id: T-035
title: Match & Replace engine - rule model, sections, operations, matchers, term replacer, body encodings
milestone: M2
role: backend
status: in_progress
depends_on: [T-099]
owner: "codex"
branch: task/T-035-match-replace-engine
paths:
  - internal/matchreplace/**
  - internal/rawhttp/builder.go
  - internal/rawhttp/builder_test.go
  - internal/rawhttp/chunked.go
  - internal/rawhttp/chunked_test.go
spec:
  - docs/01-feature-spec.md#F-MR-1 (incl. M2 contract)
  - docs/03-architecture.md §4 (hook order, buffering)
  - docs/04-query-language.md (evaluator for conditions)
---

## Goal
A pure engine that applies an ordered rule list to raw request/response bytes per F-MR-1, satisfying the `proxy.Rewriter`
shape (T-105), with no storage and no API.

## Scope
- In: `Rule` type mirroring `MatchReplaceRuleInput` (T-099); `Validate`/`Compile` (section × operation table, allowed
  matchers, RE2 compile, term size); `Engine` holding the compiled enabled list behind an atomic pointer (`Set(rules)`),
  with `Active(source, response)`, `Request(ctx, source, raw)`, `Response(ctx, source, req, resp)` exactly as in T-105;
  `Test(rule, kind, raw) (out, conditionMatched, changed, err)` for the tester, owning every tester check in F-MR-1 (see
  Notes from lead); body decode/re-encode (gzip, deflate, br, zstd
  via `klauspost/compress` / `andybalholm/brotli`, already in go.mod) and framing fix (Content-Length, chunked → one chunk +
  last chunk with trailers).
- Out: persistence, resolvers, import/export (T-102); Workflow replacer (T-066); WebSocket sections (T-053).

## Acceptance criteria
- [ ] Golden tests (`testdata/*.txtar`: rule + input + expected output) for every valid section × operation × matcher
      combination listed in T-099, including regex `$1` / `${name}` / `$$` expansion.
- [ ] Invalid combinations, disallowed matchers, bad regex, empty sources, oversized term → validation errors naming the field.
- [ ] Body rules on gzip/deflate/br/zstd responses: decoded match, re-encoded with the same coding, `Content-Length`
      fixed; stacked/unknown coding → body rule skipped, message unchanged; chunked body → one chunk + last chunk, trailers kept.
- [ ] A message no rule changes is returned as the same slice (fidelity); conditions see earlier rules' changes; `resp.*`
      clauses are false on requests; non-matching `sources` never apply.
- [ ] F-MR-1 AC core: `REQUEST_HEADER / ADD / "User-Agent: x"` appends the header.
- [ ] Status update to a non-3-digit result skips the rule (logged).
- [ ] `Test` returns an `apperr.BadInput` error and nil output for: a section for the other kind, `raw` > 12 MiB, each
      tester framing rejection in F-MR-1, and output that would exceed 16 MiB (detected while replacing, before the full
      result is allocated; test with a small input whose replacement crosses the cap); a bad condition is `BAD_QUERY`.
- [ ] Benchmark: 20 enabled header rules on a 2 KiB request; result in Handoff (target ≤ 100 µs/op).
- [ ] `make check` green.

## Notes from lead
- Go structural typing: do not import `internal/proxy`; T-109 adds a compile-time assertion in `internal/app`.
- Tester errors (lead, 2026-11-20, answers the blocked question): the engine owns them. `Test` returns
  `(out []byte, conditionMatched, changed bool, err error)`; `err` is an `*apperr.Error` (`apperr.New(apperr.BadInput, …)`,
  message naming the problem, e.g. "tester output must be at most 16 MiB"). T-102 passes it through unchanged. Proxy
  paths (`Request`/`Response`) are unchanged: no error, oversized results there follow the capture-cap rule.
- Parse with `rawhttp`, edit with `rawhttp.Builder`; never `net/http` (ADR-0003).
- Chunked bodies (lead, 2026-11-20, answers blocked Q1): extend `internal/rawhttp` (paths above; no other open task owns
  it) rather than assembling wire bytes in the engine. Add `Builder.SetChunkedBody(payload, trailer []byte)`: keeps the
  `Transfer-Encoding` field where it is, drops `Content-Length`, writes `hex(len)\r\n payload \r\n 0\r\n trailer \r\n`
  (an empty payload writes only the last chunk). Add the matching exported helper in `chunked.go` that splits a wire
  chunked body (as `Message.Body()` returns it) into payload and raw trailer section. `SetBody` is unchanged. Unit tests
  in rawhttp for both, including trailers and an empty payload.
- Presets (lead, 2026-11-20, answers blocked Q2; F-MR-1 "Presets"): the engine expands, the caller supplies the list.
  `Set(rules, presets []filterpreset.Preset)` and `Test(rule, presets, kind, raw)` expand each condition with
  `filterpreset.Expand` (pure; no store access). In `Set`, a rule whose condition fails to expand is skipped and logged
  at warn; in `Test` and in `Validate`/`Compile` it is the `BAD_QUERY` error with span that `Expand` returns. Add golden
  cases: a `preset:` condition that matches, an unknown preset in `Test`, and a rule skipped by `Set`.
- Condition fields (lead, 2026-11-20, answers blocked Q3; F-MR-1 "Condition fields"): validation rejects every field the
  bytes do not carry, so no zero value is ever guessed and the T-105 interface stays as merged. After `filterpreset.Expand`,
  walk the AST (in `internal/matchreplace`) and return `*core.Error{Span, Msg}` (`BAD_QUERY`) for the first clause outside
  the allowlist; `Set` skips such a rule at warn like a failed expansion. The `query.Message` adapter can return zero values
  for the rejected methods, since they are never reached. Tests: `req.tls.eq:false` and `source:proxy` rejected with span in
  `Validate` and `Test`; a preset containing `row.id` skipped by `Set`; `req.host` from `Host` and from an absolute-form target.
- Overflowing Content-Length (lead, 2026-11-20, answers blocked Q4; F-MR-1 tester): the wire parser wins. A
  `Content-Length` that `rawhttp` rejects (over 2^63−1) is `BAD_INPUT` in `Test`, even on an empty response; the
  empty-body exception covers only lengths the parser accepts. No parser change; `read.go` stays out of scope. Add a
  `Test` case: empty response with `Content-Length: 9223372036854775808` → `BAD_INPUT`, nil output.

- Chunked with Content-Length (lead, 2026-11-20, answers blocked Q5; F-MR-1 tester): the wire parser wins again. On
  HTTP/1.1 with a chunked final coding, `Content-Length` is validated exactly as `rawhttp` already does (decimal, in range,
  duplicates agree; else `BAD_INPUT`) and then ignored: no comparison with the wire bytes or the decoded payload. A final
  coding other than `chunked` is `BAD_INPUT` (rawhttp `ErrFraming`). On HTTP/1.0 `Transfer-Encoding` is ignored and the
  length comparison applies. A body rule writes via `SetChunkedBody` (drops `Content-Length`); a message no rule changes
  keeps the field byte-for-byte. The T-099 mock's "skip when any TE header" shortcut is not the contract. Tests: HTTP/1.1
  chunked + `Content-Length: 999` accepted; same with `Content-Length: abc` → `BAD_INPUT`; HTTP/1.0 chunked +
  mismatched length → `BAD_INPUT`; `Transfer-Encoding: gzip` request → `BAD_INPUT`.

## Plan (implementer, ≤15 lines)

1. Compile and validate rule definitions and HTTPQL conditions.
2. Apply ordered rules to raw HTTP sections and preserve message framing.
3. Verify tester limits, golden cases, and header-rule performance.

Progress: Completed the chunked-body decoding and trailer extraction portion of step 2.

## Out-of-scope change needed (if any)

- Resolved (lead, 2026-11-20; see Notes from lead, Chunked with Content-Length): F-MR-1 requires rejecting a body whose length differs from
  `Content-Length`. For a valid HTTP/1.1 chunked message with a valid decimal `Content-Length`,
  `rawhttp` uses chunked framing and ignores the declared length, and the T-099 mock skips
  the length comparison whenever a Transfer-Encoding header is present. Should the tester
  validate the decimal length fields and then follow chunked framing alone, reject the
  combination, or compare the declared length to the chunked wire bytes or decoded payload?

- Resolved (lead, 2026-11-20; see Notes from lead, Overflowing Content-Length): F-MR-1 says an empty tester response may declare any `Content-Length`,
  and also says the tester follows `rawhttp` framing. `rawhttp` rejects decimal
  lengths greater than `math.MaxInt64` on ordinary responses before the tester
  can apply the empty-body exception. Should the tester accept those empty
  responses or return `BAD_INPUT`? If it should accept them, which parser entry
  point should support that exception (`internal/rawhttp/read.go` is outside
  this task's paths)?

- Resolved (lead, 2026-11-20; see Notes from lead, Condition fields): How should conditions handle HTTPQL fields whose values the rewriter
  and tester cannot know? For example, `req.tls.eq:false` would match an HTTPS
  origin-form request if the engine supplies the zero value for TLS. The T-105
  hook provides source and wire bytes, and the tester provides kind and bytes.
  Should these clauses evaluate false when the field is unavailable, should rule
  validation reject them, or should the caller contract supply metadata? This
  also affects fields such as `resp.roundtrip`, `req.created_at`, and `row.id`.

- Resolved (lead, 2026-11-20; see Notes from lead, Chunked bodies): F-MR-1 requires changed chunked bodies to retain chunked framing and
  trailers. The lead note requires edits through `rawhttp.Builder`, but its only
  body setter always removes Transfer-Encoding and sets Content-Length. May
  T-035 extend `internal/rawhttp/builder.go` and its tests to support explicitly
  framed replacement bodies, or may the engine assemble those wire bytes itself?
- Resolved (lead, 2026-11-20; see Notes from lead, Presets): HTTPQL permits `preset:` clauses and requires caller-side expansion.
  T-035 has no storage, and the tester has no preset input. Should the engine
  accept a preset resolver, or reject preset clauses with a span-bearing
  `BAD_QUERY` until a later task wires preset resolution?

- Resolved (lead, 2026-11-20): "Should `Test` return an error for `BAD_INPUT`?" Yes, the engine owns every tester check; see Notes from lead.

## Handoff (implementer)
- What changed: Commit `c37e05d` added `rawhttp.SplitChunkedBody`. It uses the existing chunk parser, rejects trailing bytes, and returns the trailer fields as a slice of the input without the terminating blank line.
- How to verify: `go test -race ./internal/rawhttp` passed and covers multiple chunks, trailers, an empty body, truncation and trailing bytes. `make check` passed after `cd web && bun install --frozen-lockfile` installed the missing dependencies. All 402 web tests passed.
- Known limitations: The acceptance criteria remain open. The rule engine, offline tester, chunked body writer, golden tests and benchmark are pending. Status stays `in_progress`.
- Follow-ups: Start with rule and HTTPQL condition validation for the offline tester. The live rewrite paths and ambiguous request framing behavior require a scope decision before implementation.

## Review (reviewer)
- Verdict: changes_requested
- Findings:
  - [ ] [blocker] internal/matchreplace/ — package does not exist. Rule type, Validate/Compile, Engine (`Set`/`Active`/`Request`/`Response`), `Test`, body codecs and framing fix are all missing → implement per Scope; nearly every AC is unmet.
  - [ ] [blocker] internal/matchreplace/testdata/*.txtar — no golden tests (section × operation × matcher, regex `$1`/`${name}`/`$$`, presets, condition-field rejection, framing/Content-Length cases from lead notes) → add.
  - [ ] [blocker] (benchmark) — no 20-rule/2 KiB benchmark, none recorded in Handoff → add and report.
  - [ ] [major] internal/rawhttp/builder.go — `Builder.SetChunkedBody(payload, trailer)` (required by lead note) and its builder_test.go cases (trailers, empty payload) are absent → add.
  - [ ] [major] docs/01-feature-spec.md, docs/tasks/M2/T-102-*.md, docs/tasks/M2/T-109-*.md — outside the task `paths`. They are lead-directed spec/task updates, but they sit in this branch under the implementer's commits → confirm the lead owns them (or split them out) before merge.
  - [ ] [minor] internal/rawhttp/chunked.go:28 — `SplitChunkedBody` slices `wire[end-c.trailerBytes:end]` and relies on `c.line` being the unread remainder. This only holds if the trailer lines are contiguous and the final blank line is the last thing read. Add a test with bare-LF trailers and a chunk extension. Also bound `io.ReadAll` (the caller caps at 64 MiB), and doc the aliasing of the result.
  - [ ] [minor] docs/tasks/M2/T-035-*.md — status is `in_progress` with a partial handoff. Correct for now, but set `in_review` only once the ACs are met.
- Checks: `go test -race ./internal/rawhttp` passes. `make check` was not re-run, since the diff holds only the rawhttp splitter and docs. The delivered scope is about 10% of the task (only `SplitChunkedBody`, which is correct for the tested cases).
