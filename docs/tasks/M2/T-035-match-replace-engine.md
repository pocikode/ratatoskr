---
id: T-035
title: Match & Replace engine - rule model, sections, operations, matchers, term replacer, body encodings
milestone: M2
role: backend
status: blocked
depends_on: [T-099]
owner: "codex"
branch: task/T-035-match-replace-engine
paths:
  - internal/matchreplace/**
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

## Plan (implementer, ≤15 lines)

1. Compile and validate rule definitions and HTTPQL conditions.
2. Apply ordered rules to raw HTTP sections and preserve message framing.
3. Verify tester limits, golden cases, and header-rule performance.

## Out-of-scope change needed (if any)

- Open: F-MR-1 requires changed chunked bodies to retain chunked framing and
  trailers. The lead note requires edits through `rawhttp.Builder`, but its only
  body setter always removes Transfer-Encoding and sets Content-Length. May
  T-035 extend `internal/rawhttp/builder.go` and its tests to support explicitly
  framed replacement bodies, or may the engine assemble those wire bytes itself?
- Open: HTTPQL permits `preset:` clauses and requires caller-side expansion.
  T-035 has no storage, and the tester has no preset input. Should the engine
  accept a preset resolver, or reject preset clauses with a span-bearing
  `BAD_QUERY` until a later task wires preset resolution?

- Resolved (lead, 2026-11-20): "Should `Test` return an error for `BAD_INPUT`?" Yes, the engine owns every tester check; see Notes from lead.

## Handoff (implementer)
- What changed:
- How to verify:
- Known limitations:
- Follow-ups (proposed tasks):

## Review (reviewer)
- Verdict: approve | changes_requested
- Findings:
