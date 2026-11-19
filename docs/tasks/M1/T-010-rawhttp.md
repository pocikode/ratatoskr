---
id: T-010
title: rawhttp - lenient byte-preserving HTTP/1.x reader and writer
milestone: M1
role: backend
status: done
depends_on: [T-001]
owner: opencode/deepseek-v4.1-flash-1017
branch: task/T-010-rawhttp
paths:
  - internal/rawhttp/**
spec:
  - docs/01-feature-spec.md#F-NET-3
  - docs/adr/0003-own-raw-http-engine.md
  - docs/03-architecture.md §4
---

## Goal
`internal/rawhttp` reads requests/responses from a `*bufio.Reader` preserving exact bytes, exposes a lazy structured view
(method, target, version, headers in original order/case incl. duplicates, body framing), and writes messages byte-exact.

## Notes from lead
- Framing rules: RFC 9112 for well-formed input, but never "fix" input; if framing is ambiguous (CL+TE, duplicate CL),
  follow the same choice as `net/http` *for reading on the proxy side* and record `framing_warning`.
- Body handling: streaming reader with capture cap; chunked decoder exposes both raw and decoded views.
- Responses to HEAD and 1xx/204/304 have no body. Support `Connection: close` and keep-alive reuse; pipelining read-ahead tolerated.
- API sketch: `ReadRequest(br, Limits) (*Request, error)`, `(*Request).Raw() []byte`, `Headers() iter.Seq2[int, Header]`,
  `Body() io.Reader`, `WriteTo(w)`; `Builder` for editing that re-serializes only changed parts.

## Acceptance criteria
- [x] ≥ 30 golden txtar cases round-trip byte-exact (malformed + normal), incl. obs-fold, bare LF, lowercase method, absolute-form, `*` target, huge headers (limit error).
- [x] `FuzzReadRequest` / `FuzzReadResponse` run 60 s without panics; corpus committed.
- [x] Zero allocations per header iteration in benchmark (after initial parse).
- [x] No use of `net/http` types in the public API.

## Plan (implementer, ≤15 lines)
1. `message.go`: `Header{Name,Value}`, `Message` (head bytes + header spans + framing), `Raw/Head/Headers/Header/Body/DecodedBody/Framing/ContentLength/Close/Warnings/Truncated/WriteTo`.
2. `read.go`: `Limits` (head 1 MiB, body capture 64 MiB), `ReadRequest`, `ReadResponse(br, method, limits)`, lenient start-line/header parsing, warnings instead of fixes.
3. Framing follows net/http where input is ambiguous: chunked wins over CL (warn), identical duplicate CL deduped (warn), conflicting CL is an error.
4. `chunked.go`: one lenient state machine, raw mode (framing passed through) and decode mode; `body.go`: capture-capped reader + `truncated`.
5. `builder.go`: edits method/target/status/headers/body and reuses every untouched raw line.
6. `testdata/*.txtar` golden round-trips (≥30, escaped wire bytes, `-update`), chunked/builder unit tests, `FuzzReadRequest`/`FuzzReadResponse` + seed corpus, benchmark + 0-alloc header iteration test.

## Out-of-scope change needed (if any)

## Handoff (implementer)
- What changed: new `internal/rawhttp` package.
  - `ReadRequest(br, Limits)`, `ReadResponse(br, method, Limits)` (method drives the HEAD/1xx/204/304
    no-body rule). `Limits{MaxHeadBytes, MaxBodyBytes}`; zero value = `DefaultLimits` (1 MiB head,
    64 MiB body capture), negative `MaxBodyBytes` disables capture.
  - `Message`: `Head`, `Raw`, `Headers() iter.Seq2[int, Header]`, `Header`, `HeaderValues`, `Framing`,
    `ContentLength`, `Close`, `Warnings`, `Truncated`, `Body`, `DecodedBody`, `WriteTo`. `Request` and
    `Response` add `Method`/`Target`/`Version` and `Status`/`Reason`. No `net/http` types anywhere.
  - Body streams: `Body()` is the on-wire framing (chunked kept chunked), `DecodedBody()` strips chunk
    framing. `Raw()` is head + captured body, so it is complete once the body has been read;
    `WriteTo` streams the rest and captures on the way.
  - Framing follows net/http on ambiguous input: chunked wins over Content-Length, identical duplicate
    Content-Length values are deduped, conflicting ones are refused (`ErrFraming`). Every deviation is
    recorded as a `Warn*` string so the capture layer can persist it as `framing_warning`.
  - `Builder` (`Message.Builder`) edits method/target/status/headers/body and keeps the original bytes for
    everything untouched. `SetBody` re-frames with Content-Length and drops Transfer-Encoding.
- How to verify: `make check`; `go test ./internal/rawhttp -run TestGolden -v`; `go test ./internal/rawhttp
  -run '^$' -fuzz '^FuzzReadRequest$' -fuzztime 60s` (same for `FuzzReadResponse`); `go test
  ./internal/rawhttp -run XXX -bench HeaderIteration -benchmem` (0 allocs/op).
- Review round 1: every finding is fixed and marked in the Review section below; new regression tests are
  `TestObsFoldLarge`, `BenchmarkObsFold`, `TestShortLengthBody`, `TestHeaderCountLimit`,
  `TestStatusLineReason`, `TestSpaceBeforeColon`, `TestLeadingBlankLine`, `TestBuilderWrongKindStartLine`,
  `TestBuilderSynthHeadersKeepBareLF`, golden `039`/`040`/`041`, and the fuzz invariant now allows the
  skipped leading blank lines. Corpus is 59 golden cases.
- Known limitations:
  - Conflicting duplicate Content-Length is an error, not a forwarded message (net/http parity). If the
    proxy must forward such smuggling probes, a `Limits` opt-in is needed first.
  - Trailer fields stay in the raw body bytes but are not parsed into the header view.
  - Bodies beyond `Limits.MaxBodyBytes` are forwarded but `Raw()` keeps only the first slice
    (`Truncated()` true).
  - `DecodedBody()` must be the only reader of a chunked message; mixing it with `Body()` splits the stream.
  - `Builder` drops header lines that could not be parsed at all (no colon) when headers are edited; the
    unedited head keeps them.
  - A body shorter than its Content-Length fails with `io.ErrUnexpectedEOF` from `Body()`/`WriteTo`; the
    caller must drop the connection (the message is truncated, not complete).
  - Blank lines before the start line are consumed and left out of `Raw()`, and the header view is capped at
    1000 fields (`ErrHeadTooLarge`): a message cannot exceed either budget without an error.
  - A header name with whitespace before the colon is trimmed in the view (`WarnSpaceBeforeColon`); `Raw()`
    keeps the original spacing.
  - `Header.Name`/`Value` are copies, not offsets into `Raw()`. Making them offsets is the follow-up if the
    header budget ever needs to grow.
- Follow-ups (proposed tasks): T-016 should map `Warnings()` to the `framing_warning` column; T-011 owns
  the 1xx/`Connection: close` connection loop around these readers, and the `CONNECT` 2xx tunnel (a 2xx to
  CONNECT is close-delimited here, so the loop must not read a body for it).


## Review (reviewer)

**Verdict: changes_requested.** `make check` green (Go + web). Diff is inside `paths` (`go.mod`/`go.sum` only gain `x/tools`, needed by the txtar goldens). AC covered by tests, but three real bugs below were reproduced with throwaway tests (since removed).

- [x] [blocker] internal/rawhttp/read.go:198 — obs-fold CPU DoS. Each fold line does `Value += ...` and re-copies `Raw` (`string(m.raw[curStart:next])`), so it is O(n²). Reproduced: one header + 300 000 lines of `" a
"` (~0.9 MB, under the 1 MiB head limit) took **9.2 s** in `ReadRequest`. → Accumulate folds in a `[]byte`/`strings.Builder` per header and build `Value`/`Raw` once when the header ends (Raw can simply be `m.raw[curStart:next]` taken at the end). Add a test/benchmark with a large fold block.
  Fixed: folds accumulate into one `[]byte`; `endHeader` builds `Value` and `Raw` once per field. `TestObsFoldLarge` (200 000 folds, still O(n)) + `BenchmarkObsFold` (50 000 folds ≈ 4 ms/op).
- [x] [major] internal/rawhttp/message.go:214 — a short `Content-Length` body is not an error. `io.LimitReader` returns clean EOF when the peer closes early. Reproduced: `Content-Length: 10` with 3 bytes sent → `WriteTo` and `Body()` return `nil` error. The proxy would forward a truncated message as complete and reuse the connection. Chunked already returns `io.ErrUnexpectedEOF`. → Wrap the length reader so EOF before `length` bytes yields `io.ErrUnexpectedEOF`; add a test.
  Fixed: `lengthReader` (body.go) replaces `io.LimitReader`. `TestShortLengthBody` + golden `041-short-length-body`.
- [x] [major] internal/rawhttp/builder.go:156-173 — nil-pointer panic. `lineEdited()` is true after `SetMethod/SetTarget` on a *response* (or `SetStatus` on a request is silently ignored), and `startLine()` then does `*b.status` on nil. Reproduced: `resp.Builder().SetMethod("POST").Bytes()` panics. A proxy must not panic on a rewrite rule. → Make `lineEdited()` depend on `m.kind` (request: method/target; response: status/reason), or use `b.m.status` when `b.status == nil`; add tests for the wrong-kind setters.
  Fixed: `lineEdited` is kind-scoped, `startLine` falls back to `m.status`, and the wrong-kind setters are documented as ignored. `TestBuilderWrongKindStartLine`.
- [x] [minor] internal/rawhttp/read.go:~140-215 — memory amplification: header count is unbounded inside the 1 MiB head. ~350k `a:` lines each allocate a `Header` plus two strings (Name/Value) plus `Raw` copy → tens of MB per message. → Cap header count (e.g. 1000, error `ErrHeadTooLarge`) or make `Header` hold offsets into `raw` instead of copied strings (this would also help the 0-alloc goal).
  Fixed by the cap (`maxHeaders = 1000`, `ErrHeadTooLarge`); offsets in `Header` would change the public API, so that stays a follow-up if the budget ever needs to scale. `TestHeaderCountLimit`.
- [x] [minor] internal/rawhttp/read.go:251 — reason phrase is found with `bytes.Index(line, fields[1])`, which matches inside the version if it contains the code (e.g. `HTTP/200 200 OK`). → Compute the offset from the field position (skip the version field first).
  Fixed: `parseStatusLine` walks the fields with `nextField`. `TestStatusLineReason`.
- [x] [minor] internal/rawhttp/read.go:18 — doc says "Non-positive means capture nothing", but `0` means `DefaultLimits` (`normalized`). → Fix the comment: "negative disables capture; zero means default".
  Fixed as suggested.
- [x] [minor] internal/rawhttp/read.go:~182 — a header name with trailing space (`Transfer-Encoding : chunked`) is kept as name `"Transfer-Encoding "`, so framing ignores it silently while a strict upstream may honour it (smuggling vector). No warning is recorded. → Add a `WarnSpaceBeforeColon`-style warning (and ideally treat the trimmed name as the framing name or error like net/http); add a golden case.
  Fixed: the name is trimmed for the structured view and framing, `WarnSpaceBeforeColon` is recorded, `Raw` keeps the spacing. `TestSpaceBeforeColon` + golden `039-space-before-colon`.
- [x] [minor] internal/rawhttp/read.go:107 — a leading empty line before the request line is rejected as `ErrMalformed`. RFC 9112 §2.2 says a server SHOULD ignore at least one leading CRLF, and keep-alive clients send a stray CRLF after a POST body. → Skip one leading blank line (record a warning) or document the decision.
  Fixed: leading blank lines are skipped (inside the head budget, so a CRLF flood still hits `ErrHeadTooLarge`), `WarnLeadingBlankLine` is recorded, and the skipped bytes stay out of `Raw`. `TestLeadingBlankLine` + golden `040-leading-blank-line`; golden `033-blank-line-only` now expects `EOF` instead of `malformed`.
- [x] [minor] builder.go:~97 — headers rebuilt without `Raw` always use `\r\n` even when the message used bare LF; the start-line path keeps the original terminator. → Reuse `m.terminator()` for synthesized header lines.
  Fixed. `TestBuilderSynthHeadersKeepBareLF`.
- [x] [nit] message.go (`ReadResponse`) — no-body rule omits `CONNECT` 2xx (a tunnel would be read as close-delimited). Fine for T-010 if T-011 handles it; note it in Handoff follow-ups.
  Noted in the `noResponseBody` doc comment and in the Handoff follow-ups; T-011 owns the tunnel.

### Review round 2

**Verdict: approve.** `make check` and `make test-int` green. Diff still inside `paths`. All round 1 findings re-verified in code and tests:
obs-fold is now linear (`endHeader` builds once; `BenchmarkObsFold` 50k folds ≈ 2.2 ms), `lengthReader` returns `io.ErrUnexpectedEOF` on a short body,
`lineEdited` is kind-scoped (no nil deref), header cap 1000, status-line field walk, leading blank lines skipped with a warning.
Extra checks: `FuzzReadRequest` / `FuzzReadResponse` 20 s each, no crashes (4.3M / 6.0M execs; the 60 s AC was run by the implementer); `BenchmarkHeaderIteration` 0 allocs/op; 59 golden cases.

- [ ] [nit] internal/rawhttp/read.go:117 — skipped leading blank lines are dropped from `Raw()`/`WriteTo`, so a forwarded message is not byte-identical to the input in that case. Documented and warned (`WarnLeadingBlankLine`); T-011/T-016 should keep that in mind when persisting "raw" bytes.
- [ ] [nit] internal/rawhttp/read.go:438 — `Content-Length: 5, 5` (comma list) is `ErrFraming`, stricter than net/http. Safe direction; revisit if the proxy must forward such probes (already listed in limitations).
