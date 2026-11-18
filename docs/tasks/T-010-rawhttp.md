---
id: T-010
title: rawhttp - lenient byte-preserving HTTP/1.x reader and writer
milestone: M1
role: backend
status: in_progress
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
- [ ] ≥ 30 golden txtar cases round-trip byte-exact (malformed + normal), incl. obs-fold, bare LF, lowercase method, absolute-form, `*` target, huge headers (limit error).
- [ ] `FuzzReadRequest` / `FuzzReadResponse` run 60 s without panics; corpus committed.
- [ ] Zero allocations per header iteration in benchmark (after initial parse).
- [ ] No use of `net/http` types in the public API.

## Plan (implementer, ≤15 lines)
1. `message.go`: `Header{Name,Value}`, `Message` (head bytes + header spans + framing), `Raw/Head/Headers/Header/Body/DecodedBody/Framing/ContentLength/Close/Warnings/Truncated/WriteTo`.
2. `read.go`: `Limits` (head 1 MiB, body capture 64 MiB), `ReadRequest`, `ReadResponse(br, method, limits)`, lenient start-line/header parsing, warnings instead of fixes.
3. Framing follows net/http where input is ambiguous: chunked wins over CL (warn), identical duplicate CL deduped (warn), conflicting CL is an error.
4. `chunked.go`: one lenient state machine, raw mode (framing passed through) and decode mode; `body.go`: capture-capped reader + `truncated`.
5. `builder.go`: edits method/target/status/headers/body and reuses every untouched raw line.
6. `testdata/*.txtar` golden round-trips (≥30, escaped wire bytes, `-update`), chunked/builder unit tests, `FuzzReadRequest`/`FuzzReadResponse` + seed corpus, benchmark + 0-alloc header iteration test.

## Out-of-scope change needed (if any)

## Handoff (implementer)

## Review (reviewer)
