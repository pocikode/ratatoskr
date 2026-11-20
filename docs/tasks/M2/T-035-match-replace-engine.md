---
id: T-035
title: Match & Replace engine - rule model, sections, operations, matchers, term replacer, body encodings
milestone: M2
role: backend
status: in_review
depends_on: [T-099]
owner: "claude-backend"
branch: task/T-035-match-replace-engine
paths:
  - internal/matchreplace/**
  - internal/rawhttp/builder.go
  - internal/rawhttp/builder_test.go
  - internal/rawhttp/chunked.go
  - internal/rawhttp/chunked_test.go
  - internal/rawhttp/read.go
  - internal/rawhttp/read_test.go
  - internal/rawhttp/bench_test.go
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
- [x] Golden tests (`testdata/*.txtar`: rule + input + expected output) for every valid section × operation × matcher
      combination listed in T-099, including regex `$1` / `${name}` / `$$` expansion.
- [x] Invalid combinations, disallowed matchers, bad regex, empty sources, oversized term → validation errors naming the field.
- [x] Body rules on gzip/deflate/br/zstd responses: decoded match, re-encoded with the same coding, `Content-Length`
      fixed; stacked/unknown coding → body rule skipped, message unchanged; chunked body → one chunk + last chunk, trailers kept.
- [x] A message no rule changes is returned as the same slice (fidelity); conditions see earlier rules' changes; `resp.*`
      clauses are false on requests; non-matching `sources` never apply.
- [x] F-MR-1 AC core: `REQUEST_HEADER / ADD / "User-Agent: x"` appends the header.
- [x] Status update to a non-3-digit result skips the rule (logged).
- [x] `Test` returns an `apperr.BadInput` error and nil output for: a section for the other kind, `raw` > 12 MiB, each
      tester framing rejection in F-MR-1, and output that would exceed 16 MiB (detected while replacing, before the full
      result is allocated; test with a small input whose replacement crosses the cap); a bad condition is `BAD_QUERY`.
- [x] Benchmark: 20 enabled header rules on a 2 KiB request; result in Handoff (target ≤ 100 µs/op).
- [x] `make check` green.

## Notes from lead
- Go structural typing: do not import `internal/proxy`; T-109 adds a compile-time assertion in `internal/app`.
- Tester errors (lead, 2026-11-20, answers the blocked question): the engine owns them. `Test` returns
  `(out []byte, conditionMatched, changed bool, err error)`; `err` is an `*apperr.Error` (`apperr.New(apperr.BadInput, …)`,
  message naming the problem, e.g. "tester output must be at most 16 MiB"). T-102 passes it through unchanged. Proxy
  paths (`Request`/`Response`) never return an `apperr`; oversized results there follow the capture-cap rule.
- Hook errors and invalid results (lead, 2026-11-20, answers blocked Q6; F-MR-1 "Invalid results"): implement the merged
  T-105 signatures exactly (`([]byte, error)`). Return a non-nil error only when the input does not parse with `rawhttp`
  or `ctx` is done; the proxy then keeps the original bytes. Every per-rule failure is handled inside the engine: a
  result that `rawhttp` cannot parse as the same kind with its framed body complete in the buffer is discarded (warn,
  rule name) and the next rule sees the previous output; same for a non-3-digit status. Edits to framing header fields
  are kept when the result parses (e.g. a smaller `Content-Length`: the extra bytes stay, not body). `Test` uses the
  same rule: a skipped rule gives `changed=false` and the input as output, not an error. Golden cases: a first-line
  update that breaks the request line is skipped while a later rule still applies; a `Content-Length` raw edit larger
  than the body is skipped; one smaller than the body is kept with the trailing bytes.
- Trailing bytes and smuggling (lead, 2026-11-20, answers blocked Q7; F-MR-1 "Invalid results"): the contract stands.
  Forwarding a message whose edited framing leaves extra bytes is a feature (ADR-0003: desync and smuggling testing),
  and the merged T-105 proxy already contains it. It sends a changed request that `wholeRequest` rejects
  (`x.split`, `hooks.go:552`) once on a fresh upstream connection and closes that connection after
  (`forward.go:174`, `release`). It closes the client connection after a changed response that is not whole
  (`respondHooked`). Do not add a framing-safety check or refuse such results in the engine. Implement the golden
  cases as written.
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

- Reassignment (lead + operator, 2026-11-20, answers the second Q7 block): the framing contract in F-MR-1 "Invalid
  results" is final and will not be revisited. It is a product decision for an authorized-testing tool (ADR-0003), and
  the engine needs no smuggling-specific code: it only must not add a framing-safety check. The task is reassigned
  (owner cleared, status `todo`). Next implementer: continue from the merged rawhttp chunked work (`SplitChunkedBody`,
  `SetChunkedBody`, added in 47f3f3e after the review; re-check the open Review findings) and deliver the rest of Scope.

- Review round 2 (lead, 2026-11-20):
  - Out-of-path docs (finding 6): the edits to `docs/01-feature-spec.md`, `docs/tasks/M2/T-102-*.md` and
    `docs/tasks/M2/T-109-*.md` on this branch are lead commits (6dc761f, b9224b7, 4e95269, 01b9529, 39e6d17, bc6a63a),
    each answering a blocked question in this file. The lead owns them and they merge with T-035. No implementer change.
  - Benchmark (finding 5): the 100 µs/op target and the re-parse-every-changed-result contract both stand. `paths` now
    include `internal/rawhttp/read.go` (+ `read_test.go`, `bench_test.go`; no other open task owns them) so parse
    allocations can be cut, e.g. a parse entry point over a `[]byte` that avoids a fresh `bufio.Reader` and copying the
    head. Behaviour of the existing `ReadRequest`/`ReadResponse` must not change (rawhttp goldens and fuzz stay green).
    Engine-side savings are welcome too, as long as every changed result is still re-parsed.

## Plan (implementer, ≤15 lines)

1. rawhttp: `Builder.SetHeaders` (whole header list, Raw kept per field) and `Builder.Head` (head only); bound and
   document `SplitChunkedBody` (payload ≤ input, trailer aliases wire), bare-LF/extension test.
2. `internal/matchreplace/rule.go`: `Rule` (MatchReplaceRuleInput shape, schema enum strings), `Validate`, `compile`
   (section × operation, matchers, RE2, sizes, ADD header term, condition via `filterpreset.Expand` + field allowlist).
3. `message.go`: parse raw bytes with rawhttp into head/body/trailing bytes; `condition.go`: query.Message view, eval
   where clauses about a missing side are false.
4. `apply.go`: per-section operations with a bounded replacer (`$1`/`${name}`/`$$` via `Regexp.Expand`, output cap
   checked while replacing); `body.go`: gzip/deflate/br/zstd decode (bounded) + re-encode, Content-Length / chunked.
5. `engine.go`: atomic rule list, `Set/Active/Request/Response`; each changed result is re-parsed, invalid → skip+warn.
6. `tester.go`: `(*Engine).Test` with all F-MR-1 tester checks as `apperr`.
7. Tests: txtar goldens (all section × operation × matcher, regex expansion, framing, presets), table tests for
   validation/tester/encodings/fidelity, benchmark (20 header rules, 2 KiB).

## Out-of-scope change needed (if any)

- Resolved (lead, 2026-11-20; see Notes from lead, Trailing bytes and smuggling): the live rewrite contract forwards a message whose edited Content-Length leaves bytes beyond its declared body, which can enable request smuggling. A framing-safe contract is needed.

- Resolved (lead, 2026-11-20; see Notes from lead, Hook errors and invalid results): T-035 calls for the exact T-105 `Rewriter` shape, but the lead's tester note says the proxy paths have “no error”. The merged interface returns `([]byte, error)` for both hooks, and the proxy uses a non-nil error to keep the original bytes. Should the engine implement the merged signatures and use that fallback for invalid rewrite results, or always return a nil error and handle invalid results inside the engine? F-MR-1 specifies skipping invalid status-code results but leaves the handling of other invalid start-line and framing edits open.

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
- Completed (claude-backend, 2026-11-20):
  - `internal/rawhttp`: `Builder.SetHeaders(hs)` (whole header block; a field keeps its `Raw` bytes unless edited) and
    `Builder.Head()` (head without reading the body). `SplitChunkedBody` now reads into a buffer sized and limited
    to `len(wire)` (the payload can never be longer), documents that the trailer aliases `wire`, and rejects a body
    without the last chunk. New tests: bare-LF trailers with chunk extensions on data and last chunk, empty input,
    and `SetHeaders`/`Head`. This addresses the open minor review finding on `chunked.go`.
  - `internal/matchreplace` (new):
    - `rule.go`: `Rule`/`Matcher` in the `MatchReplaceRuleInput` shape (schema enum names, JSON tags as in export) and
      `Validate` (name, sources, then the rest). Unexported `compile` checks the section x operation table and the
      matchers allowed per operation (ADD ignores the matcher), non-empty values, RE2, ADD header term, 1 MiB
      term/value and 64 KiB condition. The condition goes through `filterpreset.Expand` and the condition-field allowlist.
    - `condition.go`: field allowlist (`BAD_QUERY` with span, "<field> is not available in Match & Replace
      conditions", `source:` too) and an evaluator where clauses about a missing side are false (`NOT` of one is true).
    - `message.go`/`apply.go`: parse with rawhttp; the body and any bytes after it are located in the input buffer.
      Each section x operation edits through `rawhttp.Builder`, except UPDATE_RAW on the first line, which splices
      the raw line and is re-parsed. One bounded replacer serves all matchers: `$1`/`${name}`/`$$` via
      `Regexp.Expand`, and the output cap is checked while replacing.
    - `body.go`: one gzip/deflate (zlib or raw, re-encoded the same way)/br/zstd coding is decoded (bounded at 64 MiB)
      and re-encoded. Content-Length is set in place. Chunked bodies use `SplitChunkedBody` + `SetChunkedBody`. Stacked,
      unknown or corrupt codings skip the rule. Body rules leave a response that cannot carry a body (HEAD/1xx/204/304)
      unchanged. A body over 64 MiB gets no rule (info log).
    - `engine.go`: `New(log)`, `Set(rules, presets)` (atomic list; a rule that fails to compile is skipped at warn),
      `Active`, `Request`, `Response`, matching the T-105 `Rewriter` structurally. Every changed result is re-parsed.
      An unparsable result or a non-3-digit status skips the rule (warn with the rule name), and the next rule gets the
      previous output. No framing-safety check. Errors only for unparsable input or a done ctx. Unchanged output is the
      input slice. A 2xx answer to CONNECT is never rewritten.
    - `tester.go`: `(*Engine).Test(rule, presets, kind, raw) (out, conditionMatched, changed, err)` does every F-MR-1
      tester check and returns `*apperr.Error` with nil output. A skipped rule gives the input with `changed=false`.
- Verification: `make check` is green. Run `go test ./internal/matchreplace/ ./internal/rawhttp/` (goldens:
  `internal/matchreplace/testdata/engine.txtar`, 83 cases, `-update` rewrites `out`) and
  `go test ./internal/matchreplace/ -run '^$' -bench HeaderRules -benchmem`.
- Benchmark (Apple M4, GOMAXPROCS 10, machine under load avg ~12): `BenchmarkHeaderRules` 68-70 µs/op over 6 runs,
  249 KB/op, 360 allocs/op (was ~115 µs, 3081 allocs). Target ≤ 100 µs met. Every changed result is still re-parsed
  (21 parses). `go test ./internal/rawhttp -run '^$' -bench 'ReadRequest|ParseRequest' -benchmem`: the 2 KiB head
  costs 3.4 µs / 51 allocs with `ReadRequest` and 1.9 µs / 9 allocs with `ParseRequest`.
- Review round 2 (claude-backend, 2026-11-20): all six findings addressed (see Review). Changes:
  - `message.go` `splitTarget`: absolute-form only when `scheme://` leads.
  - `rule.go` `compile`: `Sources` cloned; `resume` regexp `(?s:.)(re)` compiled with the rule.
  - `apply.go` `replace`/`findAt`: incremental regex replacement with stdlib FindAll semantics.
  - `body.go`: decoded work bounded by the capture cap, encoded bytes capped as written (`capped`).
  - `internal/rawhttp/read.go`: `ParseRequest(b, limits) (*Request, n, error)` and `ParseResponse(b, method, limits)`;
    `Head` aliases `b`, the body is read from `b[n:]`. `ReadRequest`/`ReadResponse` share `init`; header
    names, values, `Raw`, method, target, version and reason are now substrings of one head string (same values).
- Review round 3 (claude-backend, 2026-11-20): three findings fixed (see Review): bounded single-match expansion
  via a precompiled term template, budget counts decided output only, status zero-padded. `make check` green;
  benchmark 69-74 µs/op, 360 allocs/op.
- Limitations:
  - A REGEX replacement allocates one small index slice per match (garbage, not held): live memory is bounded by the
    output cap, but a very dense body costs time proportional to its matches.
  - On proxy paths a rule result over 256 MiB is skipped. This is a memory bound only (`shortcut:` comment in
    `engine.go`); F-MR-1 sets no proxy output cap.
  - Edits rebuild the start line from parsed fields (Builder `SetMethod`/`SetTarget`/`SetStatus`), so odd spacing
    in an edited request or status line is normalised. Untouched lines keep their bytes.
  - A header line emptied by UPDATE_RAW is dropped, not written as a blank line.
  - A REQUEST_QUERY param edit that removes the last param also drops the `?`.
  - `identity` in Content-Encoding counts as an "other" coding, so body rules skip it (spec wording).
- Follow-ups: T-109 adds the `proxy.Rewriter` compile-time assertion. T-102 maps API enums onto `Rule` (string values
  are the schema names; hook sources match case-insensitively).

## Review (reviewer)

- Verdict: changes_requested (round 3)
- Round 2 findings:
  - [x] [major] internal/matchreplace/message.go:104 — An origin-form target with a URL in its query is mistaken for absolute-form. The `next` parameter in `/login?next=https://example.invalid/home` disappears from query matching, and the parsed path is also wrong → identify the target form from its prefix before splitting path and query, and add a golden for updating this parameter.
    Fixed: `splitTarget` treats the target as absolute-form only when `scheme://` comes before any `/?#`. Goldens `req-query-value-url-in-origin-form` (updates `next`) and `req-path-url-in-origin-form-query`.
  - [x] [major] internal/matchreplace/rule.go:196 — The atomic snapshot retains the caller's `Sources` backing array. Reusing that array after `Set` changes live rule selection without a reload and can race with a hook → copy the slice before publishing and test mutation after `Set`.
    Fixed: `compile` stores `slices.Clone(r.Sources)`. Test `TestSetCopiesSources`.
  - [x] [major] internal/matchreplace/apply.go:252 — A body regex collects every match and capture index before checking the output budget. This can allocate far beyond the message and tester caps even when the result is small → process matches with bounded memory while preserving anchor and boundary semantics, and cover a dense-match body.
    Fixed: `replace` runs regexp's FindAll loop one match at a time (`findAt`); after the start it runs `(?s:.)(re)` from the rune before the position, so `^`, `\b`, `\B` see the real context. Output cap checked per match. Tests: `TestRegexReplaceMatchesStdlib` (anchors, boundaries, empty matches, invalid UTF-8 vs `regexp.ReplaceAll`), `FuzzRegexReplace`, `TestRegexDenseMatches`.
  - [x] [major] internal/matchreplace/body.go:50 — The tester's wire-output budget is applied to decoded growth, so a compressible replacement can be rejected even when the re-encoded message fits. The final wire-size check also runs after encoding and assembling the full message → bound decoded work independently and enforce the tester budget on encoded bytes as they are written. Cover both sides of the wire limit.
    Fixed: decoded replacement is bounded by the 64 MiB capture cap; `encode` writes into a capped buffer that fails with the output cap once the encoded bytes would cross it. Tests: `TestTesterWireCap` (20 MiB decoded, a few KiB gzip → accepted; 9 MiB random doubled → BAD_INPUT, nil output), `TestEncodeCap` (every coding).
  - [x] [major] internal/matchreplace/engine_test.go:366 — The required 20-rule, 2 KiB benchmark measured 111.7–115.1 µs/op on Apple M4 after the other checks finished, above the 100 µs/op target → reduce rewrite overhead and rerun the same benchmark.
    Fixed: new `rawhttp.ParseRequest`/`ParseResponse` parse from the buffer (no bufio.Reader, no head copy; same results and errors as `ReadRequest`/`ReadResponse`, checked by `TestParseMatchesRead` and `FuzzParseMatchesRead`); head fields are substrings of one head string. Every changed result is still re-parsed. Now ~68-70 µs/op, 360 allocs/op.
  - [x] [major] docs/01-feature-spec.md:293 — This file and the T-102 and T-109 task files are outside T-035's declared paths. The branch history labels the edits as lead decisions → record the lead's ownership of the companion commits or land them separately before merging T-035.
    Settled by the lead (Notes from lead, "Review round 2", 2026-11-20): lead commits, owned by the lead, merge with T-035.

- Checks: `make check` and `make test-int` passed. Two temporary overlay regressions failed as expected on this branch: `TestReviewQueryURL` reported `changed=false err=<nil>`, and `TestReviewRuleSnapshot` returned the original request after the caller changed its sources slice. The standalone benchmark used `go test ./internal/matchreplace -run '^$' -bench HeaderRules -benchmem -count=3` and reported 3081 allocs/op.

### Round 3

- [x] [major] internal/matchreplace/apply.go:274 — The regex replacer expands a whole match before checking the cap. A capture-heavy replacement can allocate far beyond the limit in a single expansion → enforce the remaining budget before appending expansion bytes and cover an oversized single-match expansion.
  Fixed: the REGEX term is parsed once at compile (`parseTemplate`, regexp.Expand's rules) so `expandLen` gives the size before `expand` appends. Test `TestRegexBudget` (1 GiB expansion fails having allocated < 1 MiB); stdlib parity table gained `$01`, `${1x`, `$9999999999`, numeric names; fuzz 60 s green.
- [x] [major] internal/matchreplace/apply.go:276 — The budget check counts the unprocessed suffix as final output. An early match that grows can reject a rewrite whose later matches shrink the result below the cap → count only output already determined, and check the unchanged suffix when appending it. Add a regression with growth followed by shrinkage.
  Fixed: per match only decided output (prefix + expansion) is counted; the tail is checked when appended. `TestRegexBudget` grow-then-shrink case.
- [x] [minor] internal/rawhttp/builder.go:214 — The builder serializes a status below 100 without zero padding. The engine accepts the three-digit replacement `099`, but the builder emits `99` and re-parsing drops the edit → preserve the validated three-digit token when serializing the status and add a regression.
  Fixed: `startLine` zero-pads 0–99 to three digits. `TestBuilderStatusThreeDigits`, golden `resp-status-leading-zero`.

`make check`, `make test-int`, and `git diff --check main...HEAD` passed. After the suites finished, the standalone 20-rule benchmark measured 69.3–74.7 µs/op and 360 allocs/op over three runs on Apple M4, within the 100 µs/op target.

Two temporary overlay regressions failed. `TestReviewFinalReplacementBudget` reported `out="" err=matchreplace: result over the output cap` for a two-byte final result under a five-byte limit. `TestReviewThreeDigitStatus` reported `changed=false err=<nil>` and returned the original status. The single-match allocation issue was assessed by code inspection.
