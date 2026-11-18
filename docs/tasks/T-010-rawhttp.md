---
id: T-010
title: rawhttp - lenient byte-preserving HTTP/1.x reader and writer
milestone: M1
role: backend
status: in_review
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
- Known limitations:
  - Conflicting duplicate Content-Length is an error, not a forwarded message (net/http parity). If the
    proxy must forward such smuggling probes, a `Limits` opt-in is needed first.
  - Trailer fields stay in the raw body bytes but are not parsed into the header view.
  - Bodies beyond `Limits.MaxBodyBytes` are forwarded but `Raw()` keeps only the first slice
    (`Truncated()` true).
  - `DecodedBody()` must be the only reader of a chunked message; mixing it with `Body()` splits the stream.
  - `Builder` drops header lines that could not be parsed at all (no colon) when headers are edited; the
    unedited head keeps them.
- Follow-ups (proposed tasks): T-016 should map `Warnings()` to the `framing_warning` column; T-011 owns
  the 1xx/`Connection: close` connection loop around these readers.


## Review (reviewer)
