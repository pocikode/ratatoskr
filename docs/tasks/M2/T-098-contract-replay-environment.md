---
id: T-098
title: Contract - Replay collections, sessions, entries, send; Environments
milestone: M2
role: lead
status: changes_requested
depends_on: [T-030]
owner: "claude-code (opus-5.5)"
branch: task/T-098-contract-replay-environment
paths:
  - internal/api/graphql/schema/replay.graphqls
  - internal/api/graphql/schema/environment.graphqls
  - internal/api/graphql/resolvers/stubs_replay.go
  - internal/api/graphql/resolvers/stubs_replay_send.go
  - internal/api/graphql/resolvers/stubs_environment.go
  - internal/api/graphql/resolvers/resolver.go
  - internal/api/graphql/generated/**
  - web/src/lib/api/generated/**
  - web/src/lib/api/mocks/**
  - gqlgen.yml
  - docs/tasks/M2/T-098-contract-replay-environment.md
spec:
  - docs/01-feature-spec.md#F-REP-1, #F-REP-2 (incl. M2 contracts)
  - docs/01-feature-spec.md#F-ENV-1 (incl. M2 contract)
  - docs/03-architecture.md §6
---

## Goal
GraphQL contract for Replay (HTTP sessions only) and Environments, with stubs and mocks, so T-033/T-106/T-037 (backend)
and T-034/T-107/T-103 (frontend) can start in parallel.

## Scope
- In (`replay.graphqls`): `ReplaySessionKind {HTTP}`; `ReplayCollection implements Node {id, name, isDefault, sessions}`;
  `ReplayConnection {host, port, tls, sni}` + input; `ReplayOptions {followRedirects, updateContentLength, timeoutMs,
  applyMatchReplace}` + input (fields optional); `ReplaySession implements Node {id, kind, name, color: RequestColor,
  collection, connection, options, draft: Bytes!, sending: Boolean!, createdAt, entries(first, after): ReplayEntryConnection!}`
  (newest first, `first` 1..1000, default 100); `ReplayEntry implements Node {id, request: HttpRequest!, error: String, createdAt}`.
  Query `replayCollections` (collection order, sessions in position order), `replaySession(id)`.
  Mutation: `createReplayCollection(name)`, `renameReplayCollection(id, name)`, `deleteReplayCollection(id): ID!`,
  `reorderReplayCollections(ids)`, `createReplaySession(input {collectionId, fromRequestId, connection, raw, name})`,
  `updateReplaySession(id, input {name, connection, options, draft})`, `setReplaySessionColor(id, color)`,
  `duplicateReplaySession(id)`, `moveReplaySession(id, collectionId, index)`, `deleteReplaySession(id): ID!`,
  `sendReplaySession(id, input {raw, connection, options}): ReplayEntry!`, `cancelReplaySend(id): Boolean!`.
- In (`environment.graphqls`): `EnvironmentVariable {name, value, secret}` + input; `Environment implements Node {id, name,
  isGlobal, selected, variables}`. Query `environments` (Global first, then name order), `environment(id)`. Mutation
  `createEnvironment(name)`, `renameEnvironment(id, name)`, `deleteEnvironment(id): ID!`,
  `setEnvironmentVariables(id, variables): Environment!` (replaces the whole list), `selectEnvironment(id: ID): Environment`
  (null = none selected; returns the selection).
- In: `resolvers.Deps` fields `Replay`, `ReplaySend`, `Environment` (placeholder interfaces in the stub files; send/cancel
  stubs live in `stubs_replay_send.go` so T-033 and T-106 own separate files); `make gen`; mocks: replay (seed: Default +
  one more collection, 3 sessions, entries; `sendReplaySession` answers a canned response after ~300 ms and honours
  `cancelReplaySend`) and environments (Global + 2 project envs, one secret var), merged into the mock exchange.
- Out: WS/Pipeline sessions, HTTP/2, subscriptions (pages refetch after their own mutations).

## Acceptance criteria
- [x] Every type/field/argument documented with the F-REP-1/F-REP-2/F-ENV-1 contract rules and error codes: `CONFLICT`
      (delete Default collection, send while sending, rename/delete Global, duplicate environment name), `BAD_INPUT` (empty or
      long names, timeout outside 1000–300000 ms, port outside 1–65535, truncated source request, invalid variable name,
      duplicate variable name, value > 1 MiB, reorder ids not the exact set), `NOT_FOUND` (unknown IDs).
- [x] Send semantics documented on the field: draft saved first, blocking until done/timeout/cancel, interpolation → M&R →
      Content-Length order, fresh connection, failures as entries with `error`.
- [x] `make gen` clean; stubs compile; `make check` green.
- [x] Mocks cover CRUD, move, duplicate `(copy)`, send + cancel, env select/variables (vitest).

## Notes from lead
- Default collection: created lazily by the service, so `replayCollections` never returns an empty list.
- Keep `draft`/`raw` as `Bytes` (byte fidelity); the UI decodes for the editor (T-104).

## Plan (implementer, ≤15 lines)
1. `replay.graphqls`: kinds, collection/session/entry types, connection/options + inputs, entry connection (reuses
   `PageInfo`), queries, mutations; every field documents defaults, limits and error codes from F-REP-1/F-REP-2.
2. `environment.graphqls`: variable + input, environment, queries, mutations with F-ENV-1 rules/codes.
3. `gqlgen.yml`: lazy field resolvers for `ReplayCollection.sessions`, `ReplaySession.collection/entries`,
   `ReplayEntry.request` so list queries stay cheap; `omittable` fields where null ≠ omitted.
4. Stubs: `stubs_replay.go` (CRUD + field resolvers), `stubs_replay_send.go` (send/cancel), `stubs_environment.go`;
   `Deps.Replay`, `Deps.ReplaySend`, `Deps.Environment` placeholder interfaces. `make gen`.
5. Mocks: `replay.ts`, `environment.ts` (+ vitest), merged in `exchange.ts`. `make check`.

## Out-of-scope change needed (if any)
- `docs/01-feature-spec.md` (not in `paths`): the contract settles these details, which should be copied into the
  F-REP-1/F-REP-2/F-ENV-1 contract bullets in a docs task. Schema descriptions are authoritative until then.
  - Collection and session names: trimmed, 1–200 characters, no control characters, need not be unique;
    `(copy)` names cut the source to 193 characters. Default name `Untitled` when the bytes have no request line.
  - `draft`/`raw` at most 16 MiB decoded (`BAD_INPUT`). Send requires the bytes after interpolation, M&R and
    Content-Length update to parse as an HTTP/1.x request head (`BAD_INPUT`, no entry, draft stays saved);
    the body is every byte after the blank line.
  - Deleting a session or collection cancels in-flight sends. `moveReplaySession` index is 0-based; past the end
    appends; negative is `BAD_INPUT`. `updateReplaySession` is allowed while sending.
  - `cancelReplaySend` returns false when nothing is in flight; the cancelled send returns an entry with error `cancelled`.
  - `Environment.selected` is always false for Global; `selectEnvironment(<Global id>)` is `BAD_INPUT` (pass null).
    Deleting the selected environment leaves none selected. Connection `sni: ""` is `BAD_INPUT` (null = no override).

## Handoff (implementer)
- What changed: `replay.graphqls` and `environment.graphqls` (every type, field and argument documented, with the
  error codes from the acceptance criteria and the send pipeline on `sendReplaySession`). `gqlgen.yml`: lazy field
  resolvers for `ReplayCollection.sessions`, `ReplaySession.collection/entries`, `ReplayEntry.request`. Stubs:
  `stubs_replay.go` (CRUD + field resolvers, T-033), `stubs_replay_send.go` (send/cancel, T-106),
  `stubs_environment.go` (T-037); `Deps.Replay`, `Deps.ReplaySend`, `Deps.Environment` placeholder interfaces.
  Mocks: `mocks/replay.ts`, `mocks/environment.ts` merged into the mock exchange (`replaySendDelayMs` option,
  default 300 ms); send runs `{{var}}` interpolation from the environment mock; `mocks/replay.test.ts`.
  Review fixes: sends record the connection captured before the wait; `interpolate` works on byte strings and
  inserts values as UTF-8; Replay records use the History `slicer` (offset/limit honoured) and `extOf(path)`;
  `rawOf` reads every 4 MiB chunk. Tests cover each case in `mocks/replay.test.ts`.
  Second review fixes: records split head/body at the first LF or CRLF blank line; `fromRequestId` copies over
  16 MiB are `BAD_INPUT` before the body is read (documented on the input and mutation); default names decode
  UTF-8; `Bytes` inputs must be padded standard base64 like the server scalar. Each has a test.
- How to verify: `make gen` (no diff), `make check`, `cd web && bun run test -- src/lib/api/mocks`.
- Known limitations: the replay mock skips Match & Replace, Content-Length update and redirects, and checks only the
  request line (marked `shortcut:`). The generated models keep the lazy fields (e.g. `ReplayEntry.Request`) as plain
  struct fields; T-033/T-106 can set the ID there for the field resolver to load.
- Follow-ups (proposed tasks): docs task to copy the decisions above into `docs/01-feature-spec.md`.

## Review (reviewer)
- Verdict: changes_requested
- Findings:
  - [x] [major] `web/src/lib/api/mocks/replay.ts:425` — Updating the session connection during a send makes the completed entry report the new host and port. The send contract keeps the starting values. Pass the connection captured before the wait to the record builder and test an update during a pending send.
  - [x] [major] `web/src/lib/api/mocks/replay.ts:412` — Interpolation inserts JavaScript text into a byte string. A variable set to `é` produces `6Q==`. UTF-8 encodes it as `w6k=`. Reading the body with `東京` returns a GraphQL error. Encode the replacement values as UTF-8 bytes while preserving the untouched draft bytes. Test a non-ASCII value and compare the recorded bytes.
  - [x] [major] `web/src/lib/api/mocks/replay.ts:122` — The request and response body resolvers ignore `offset` and `limit`. A one-byte read at the end of a 4 MiB + 1 byte request returns the entire body. Reuse the History mock's body slicer, and update `rawOf` at line 165 to read all chunks so creating a Replay session from that record preserves its draft. Test both the last-byte read and the full copy.
  - [x] [minor] `web/src/lib/api/mocks/replay.ts:135` — Replay records inherit a file extension from an unrelated History fixture. A seeded `/api/v1/users` record reports `.js`, which gives the wrong result when sorting by extension. Derive `ext` from the recorded path using the same rule as History.
- `make check` passed, including all 303 web tests. `make gen` reproduced the committed output in a temporary checkout. GraphQL spot checks reproduced all four findings using the in-memory mocks. Review assumes one local frontend developer using the mock exchange. Live traffic and persistence belong to the backend follow-up tasks.

### Second review

- Reviewed `ed052f8`. The four findings above are resolved.
- [x] [major] `web/src/lib/api/mocks/replay.ts:127` — Sending an LF-only request with a body puts the body in `head` and returns an empty `body`. The raw HTTP parser accepts bare LF, and the contract puts every byte after the blank line in the body. Find the first blank line with either LF or CRLF, preserve its original bytes, and test the two fields.
- [x] [major] `web/src/lib/api/mocks/replay.ts:353` — Copying a History request bypasses the 16 MiB draft limit. A mock send expanded to a 17 MiB body can be copied successfully, then the resulting draft is rejected when saved or sent. Check the combined head and body size before reading body chunks, return `BAD_INPUT`, and document the limit for `fromRequestId`.
- [x] [minor] `web/src/lib/api/mocks/replay.ts:118` — A UTF-8 request target of `/café` produces the default name `GET /cafÃ©`. Decode the extracted name as UTF-8 before truncating it. Test the name and confirm that the original draft bytes still round-trip.
- [x] [minor] `web/src/lib/api/mocks/replay.ts:108` — The mock accepts unpadded `YQ` and returns it as a draft. The server's `Bytes` scalar requires standard padded base64. Match the scalar's input rules in the mock and emit padded base64 in returned drafts.
- `make check` passed, including 306 web tests. `make gen` reproduced both generated directories in a temporary checkout. GraphQL spot checks confirmed all four new findings. `git diff --check main...HEAD` flagged trailing whitespace emitted by the web schema generator. Review assumes one frontend developer using the in-memory mock.

### Third review

- Reviewed `4b8c6dd`. The four second-round cases are covered by regression tests. The `Bytes` finding below covers additional inputs accepted by the server.
- [ ] [major] `web/src/lib/api/mocks/replay.ts:427` — The request-line check rejects valid method tokens containing a hyphen. `M-SEARCH / HTTP/1.1` returns `BAD_INPUT`. The raw HTTP parser accepts `M-SEARCH` as the method. Accept the full method token syntax in the existing lightweight check and test that Replay records the method unchanged.
- [ ] [major] `web/src/lib/api/mocks/replay.ts:431` — A mock response delay of 1250 ms with `timeoutMs: 1000` returns a successful response after the deadline. Time the wait against the smaller of the response delay and the session timeout, and return a timeout entry with no response when the deadline wins. Test this alongside cancellation.
- [ ] [minor] `web/src/lib/api/mocks/replay.ts:133` — Sending `/search?q=a?b` records `query: "q=a"`. The History contract returns the full query and the server splits at the first `?`. Keep the whole suffix so History filters and Replay metadata see `q=a?b`.
- [ ] [minor] `web/src/lib/api/mocks/replay.ts:256` — `ReplayEntry.createdAt` records completion. With a 1250 ms mock wait, it is about 1250 ms later than the start time promised by the schema. Capture the timestamp before the wait and pass it to the entry builder so the editor can display the start time.
- [ ] [minor] `web/src/lib/api/mocks/replay.ts:108` — The mock rejects padded `Bytes` containing CR/LF, which the Go decoder ignores. It also echoes `YR==` unchanged. The server re-encodes those bytes as `YQ==`. Strip CR/LF, validate and decode, then return the canonical padded representation. Keep the unpadded-input rejection test and cover the accepted forms.
- Default `make check` failed twice in `src/pages/sitemap/sitemap.svelte.test.ts:132`, “drops a stale expansion result when filters change and supports loading beyond the first page.” Vitest reported `Error: Test timed out in 5000ms.` Both runs passed the other 308 web tests and all Go checks. `VITEST_MAX_WORKERS=2 make check` passed all 309 web tests and all Go checks.
- Review assumes one frontend developer using the in-memory mock exchange. `make gen` reproduced both generated directories. The proxy, store and raw HTTP packages were outside the diff, so `make test-int` was skipped.
- This is the third review round. Escalated to the lead under `docs/07-agent-workflow.md` §7.
