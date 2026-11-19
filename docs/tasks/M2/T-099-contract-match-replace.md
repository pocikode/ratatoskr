---
id: T-099
title: Contract - Match & Replace collections, rules, tester, import/export
milestone: M2
role: lead
status: changes_requested
depends_on: [T-098]
owner: "claude-code (opus-5.5)"
branch: task/T-099-contract-match-replace
paths:
  - internal/api/graphql/schema/matchreplace.graphqls
  - internal/api/graphql/resolvers/stubs_matchreplace.go
  - internal/api/graphql/resolvers/resolver.go
  - internal/api/graphql/generated/**
  - web/src/lib/api/generated/**
  - web/src/lib/api/mocks/**
  - gqlgen.yml
  - docs/tasks/M2/T-099-contract-match-replace.md
  - docs/01-feature-spec.md (F-MR-1 API details only)
  - docs/tasks/M2/T-108-match-replace-tester.md (acceptance criteria: tester size notice)
  - docs/tasks/M2/T-102-match-replace-service.md (acceptance criteria: tester output cap)
spec:
  - docs/01-feature-spec.md#F-MR-1 (incl. "Contract (lead, 2026-11-19, M2)")
  - docs/03-architecture.md §6
---

## Goal
GraphQL contract for Match & Replace so T-035/T-102 (backend) and T-036/T-108 (frontend) can start.

## Scope
- In: enums `MatchReplaceSource {PROXY, INTERCEPT, REPLAY, AUTOMATE, WORKFLOW, PLUGIN}`, `MatchReplaceSection
  {REQUEST_FIRST_LINE, REQUEST_METHOD, REQUEST_PATH, REQUEST_QUERY, REQUEST_HEADER, REQUEST_BODY, RESPONSE_FIRST_LINE,
  RESPONSE_STATUS_CODE, RESPONSE_HEADER, RESPONSE_BODY}`, `MatchReplaceOperation {UPDATE_RAW, UPDATE, UPDATE_VALUE, ADD,
  REMOVE}`, `MatchReplaceMatcherKind {FULL, STRING, REGEX, NAME}`, `MatchReplaceMessageKind {REQUEST, RESPONSE}`.
- Types: `MatchReplaceRule implements Node {id, name, enabled, collection, condition: String!, sources, section, operation,
  matcher: MatchReplaceMatcher {kind, value: String!}, term: String!}`; `MatchReplaceCollection implements Node {id, name,
  rules}`; `MatchReplaceRuleInput`; `MatchReplaceTestResult {output: Bytes!, conditionMatched: Boolean!, changed: Boolean!}`.
- Query: `matchReplaceCollections` (ordered, rules ordered), `matchReplaceRule(id)`, `exportMatchReplace: String!`.
- Mutation: `createMatchReplaceCollection(name)`, `renameMatchReplaceCollection`, `deleteMatchReplaceCollection(id): ID!`
  (deletes its rules), `reorderMatchReplaceCollections(ids)`, `createMatchReplaceRule(collectionId, input)`,
  `updateMatchReplaceRule(id, input)`, `setMatchReplaceRuleEnabled(id, enabled)`, `moveMatchReplaceRule(id, collectionId,
  index)`, `deleteMatchReplaceRule(id): ID!`, `importMatchReplace(json: String!): Int!` (collections added),
  `testMatchReplace(rule: MatchReplaceRuleInput!, kind: MatchReplaceMessageKind!, raw: Bytes!): MatchReplaceTestResult!`.
- The valid section × operation table and which matchers each operation accepts are written in the schema docs:
  first line / path / body → `UPDATE_RAW`; method / status code → `UPDATE`; query / header → `UPDATE_RAW`,
  `UPDATE_VALUE`, `ADD`, `REMOVE`. `NAME` only with `UPDATE_VALUE`/`REMOVE`; `ADD` ignores the matcher.
- In: `resolvers.Deps.MatchReplace` placeholder, stubs, `make gen`, mock (2 collections, 4 rules on reserved hosts; tester
  mock does STRING/FULL header and body replacement only, with a `shortcut:` comment).
- Out: Workflow replacer (T-066), WebSocket sections (T-053), subscriptions.

## Acceptance criteria
- [x] Every type/field/argument documented with the F-MR-1 contract rules and errors: `BAD_INPUT` (invalid section ×
      operation, matcher not allowed, invalid regex with the RE2 message, empty sources, names 1–200, term > 1 MiB,
      > 1000 rules, bad import JSON/version, reorder ids not the exact set), `BAD_QUERY` with span (condition), `NOT_FOUND`.
- [x] Export/import JSON format documented (`{"version":1,"collections":[…]}`, all or nothing, appends).
- [x] `make gen` clean; stubs compile; `make check` green.
- [x] Mock covers CRUD, enable, move, import/export round-trip and the tester (vitest).

## Notes from lead
- `condition` is `String!` (empty = always) to match how `InterceptSettings.query` is typed.
- RE2 limits (no look-around, no backrefs) must be stated on `MatchReplaceMatcherKind.REGEX` (F-MR-1 "document this").

## Plan (implementer, ≤15 lines)
1. `matchreplace.graphqls`: enums, rule/collection/matcher types, input, test result, queries, mutations; every
   rule from F-MR-1 contract (section × operation table, matchers, errors, JSON format) in descriptions.
2. gqlgen.yml: `MatchReplaceCollection.rules` and `MatchReplaceRule.collection` lazy resolvers (like Replay).
3. `stubs_matchreplace.go` returning `ErrNotImplemented`; `Deps.MatchReplace` placeholder; `make gen`.
4. Mock `mocks/matchreplace.ts` (2 collections, 4 rules, reserved hosts) wired in `exchange.ts`; validation + tester
   (STRING/FULL on header/body only, `shortcut:` comment); vitest for CRUD, enable, move, import/export, tester.
5. `make check`, Handoff, `in_review`.

## Out-of-scope change needed (if any)

## Handoff (implementer)
- What changed: `matchreplace.graphqls` (5 enums, rule/collection/matcher types, rule input, test result; 3 queries,
  11 mutations; section × operation table, matcher rules, errors and export JSON in the descriptions).
  `gqlgen.yml`: `MatchReplaceRule.collection` is a lazy resolver with a hidden `CollectionID` extra field (rules are
  embedded in collections, so the back-reference stays acyclic). `stubs_matchreplace.go` + `Deps.MatchReplace`
  placeholder; regenerated code. Mock `mocks/matchreplace.ts` (2 collections, 4 rules, one disabled) wired into
  `exchange.ts`; `matchreplace.test.ts` covers CRUD, enable, move, reorder, validation codes, import/export, tester.
  Size budget (review of `3ff8e51`): condition ≤ 64 KiB, ≤ 1000 collections, ≤ 3 MiB rule text per project,
  tester `raw` ≤ 12 MiB, tester `output` ≤ 16 MiB (lead escalation); in F-MR-1, the schema, the mock, and a new T-108 criterion (T-108 file added to paths).
  Import is validated against the schema (`createMatchReplaceMock(schema)` uses `coerceInputValue` on
  `MatchReplaceRuleInput` plus exact-key checks); malformed imports are rejected with nothing appended.
  F-MR-1 gained an "API details" bullet for the decisions the contract makes (no default collection, non-empty matcher
  values, `ADD` header term format, tester and import specifics); spec added to this task's paths for that.
- How to verify: `make gen` (no diff), `make check`; `cd web && bunx --bun vitest run src/lib/api/mocks/matchreplace.test.ts`.
- Known limitations: mock tester only does STRING/FULL `UPDATE_RAW` on headers/bodies plus header `ADD`, and treats
  every condition as matched, and leaves chunked, content-coded and bodyless (1xx/204/304) response bodies unchanged (`shortcut:` comment);
  the tester rejects a body whose length differs from `Content-Length` (also in the contract); mock HTTPQL check only flags unterminated strings; mock regex check
  is JS `RegExp` plus a look-around/backref reject, not RE2.
- Follow-ups (proposed tasks): none; T-035/T-102/T-036/T-108 can start.

## Review (reviewer)
- Verdict: changes_requested
- Findings:
  - [x] [major] web/src/lib/api/mocks/matchreplace.ts:322 — JSON import bypasses GraphQL input validation:
    `normalize` assumes typed rule fields, but parsed JSON has no runtime type/enum or unknown-field checks.
    Reproduced through the generated schema's GraphQL executor: export the seed, change the first rule's
    `sources` to `["BOGUS"]`, and import it. Import returns 2 and appends both collections; a subsequent
    `matchReplaceCollections { rules { sources } }` fails with
    `Enum "MatchReplaceSource" cannot represent value: "BOGUS"`. Invalid matcher kinds and
    `enabled: "yes"` also import successfully and break later queries. Unknown rule fields are silently
    accepted; a missing collection name becomes `"undefined"`, and missing `rules` becomes an empty list.
    This violates the documented import rejection/all-or-nothing contract even for a single mock client.
    → Validate object shape, required/unknown fields, scalar types and enum values at every JSON level
    before appending anything; reuse the schema's rule-input coercion where practical. Add a table-driven
    import test asserting `BAD_INPUT` and unchanged collections/export for these malformed inputs.
    → Fixed: import now requires the exact key set at top, collection and rule level, then runs each
    rule through graphql-js `coerceInputValue` against the schema's `MatchReplaceRuleInput` (enums,
    scalar types, nested matcher fields, unknown fields) before `normalize`; collection names must be
    strings and `rules` an array. Messages name collection and rule position. `it.each` in
    `matchreplace.test.ts` covers 14 malformed inputs, asserting `BAD_INPUT`, an unchanged export and a
    still-queryable tree.
- Verification: `make check` passed (Go race tests, vet/lint, Svelte/ESLint/Prettier, 43 Vitest files / 335 tests).
  `make gen` passed and left the working tree unchanged. All 12 changed files match the task's paths.
  Reviewed the schema, stubs, mock exchange/callers, CRUD/tester tests and linked contract against the expected
  single-process desktop use with at most 1000 rules. The explicitly documented tester/HTTPQL/regex mock shortcuts
  are accepted for this contract task.
- Not checked: browser E2E and `make test-int`; no UI, proxy, store or rawhttp implementation changed.

### Follow-up review

- Verdict is `changes_requested`. The original invalid-enum case now returns `BAD_INPUT`.
- [x] [major] `web/src/lib/api/mocks/matchreplace.ts:175` accepts malformed import JSON.
  Replacing `sources: ["PROXY"]` with `sources: "PROXY"` or removing `matcher.value` each returns
  2 and appends both seed collections. GraphQL input coercion wraps the string in a list and supplies
  the default matcher value. Check that `sources` is an array and `matcher` has both export keys
  before coercion. Add both cases to the existing tests that check `BAD_INPUT` and unchanged exports.
  → Fixed: `importRule` requires `sources` to be an array and `matcher` to have exactly `kind`/`value`
  before coercion; both cases added to the `it.each` import table.
- [x] [minor] `web/src/lib/api/mocks/matchreplace.ts:386` rejects valid HTTP method tokens.
  Testing the same header-add draft on a request starting with `M-SEARCH / HTTP/1.1` returns
  `BAD_INPUT`. Allow the full HTTP token character set and add an extension-method case to the tester test.
  → Fixed: request line accepts RFC 9110 `tchar`s; tester test adds an `M-SEARCH *` header-add case.
- [x] [minor] `web/src/lib/api/mocks/matchreplace.ts:79` reports JavaScript string offsets where
  the API specifies UTF-8 byte offsets. For `req.path.eq:"é" AND req.host.eq:"x`, the returned
  span is `{start:32,end:34}` and the byte span is `{start:33,end:35}`. Measure the UTF-8 byte
  length of the prefix and full condition, and extend the span test with a non-ASCII value.
  → Fixed: span uses `utf8Len` of the prefix and condition; validation test asserts `{start:33,end:35}`.
- Verification covered the generated schema, mock, and Go stubs for single-process desktop use with
  up to 1000 rules per project. All three findings were reproduced through the GraphQL executor.
  `make check` passed with 349 web tests.
- Browser E2E and `make test-int` were skipped for this contract review.

### Review of `3ff8e51`

- Verdict is `changes_requested`.
- [x] [major] `internal/api/graphql/schema/matchreplace.graphqls:309` — A valid project export can exceed
  the 32 MiB GraphQL POST limit in `internal/api/server.go:20`. An export with 32 rules and a 1 MiB term
  per rule produces a 33,560,983-byte import request, above the 33,554,432-byte cap. The server returns
  413 before the import resolver runs. Define a size limit that guarantees every valid export fits on
  import, or use a bounded upload that supports the full export and preserves all-or-nothing behavior.
  → Fixed (spec F-MR-1 "Size budget" + schema): a condition is at most 64 KiB; a project holds at most
  1000 collections and 3 MiB of rule text (conditions + matcher values + terms, UTF-8). Worst case
  (control characters escaped 6×, then 7× once the export is embedded in the import request, plus
  1000 rules and 1000 collections with 200-character names) is about 25 MiB, under the cap. Mock
  enforces it and tests it.
- [x] [major] `internal/api/graphql/schema/matchreplace.graphqls:327` — Base64 for a 24 MiB history
  message already fills the 32 MiB GraphQL POST limit. The GraphQL document and draft rule push it over
  the cap. The tester cannot return a result for some messages below the documented 64 MiB capture cap.
  Define a tester input budget and surface it in T-108, or pass a history ID for picked messages if the
  tester must support the full capture range.
  → Fixed: tester `raw` is at most 12 MiB (`BAD_INPUT`): 16 MiB of base64 plus the draft rule
  (≤ 2 MiB + 64 KiB, escaped ≤ 6×) stays under 32 MiB. T-108 gained a criterion: larger inputs are not
  sent and a notice explains why. Mock enforces it and tests it.
- [x] [minor] `web/src/lib/api/mocks/matchreplace.ts:77` — The quote counter rejects a valid HTTPQL
  clause such as `req.path.eq:"a\"b"`. The GraphQL executor returns `BAD_QUERY` and points to the
  closing quote. Skip escaped characters when scanning quoted literals. Test both a valid escaped quote
  and an escaped final quote without a closing delimiter.
  → Fixed: the scanner skips the character after `\` inside a literal; both cases tested.
- [x] [minor] `web/src/lib/api/mocks/matchreplace.ts:129` — A non-ADD `FULL` matcher created with
  `value: "unused"` returns and exports that value, although the output field documents an empty
  string for `FULL`. Canonicalize the value when storing a non-ADD `FULL` matcher, or document that
  the contract preserves ignored values.
  → Fixed: stored empty (mock `normalize`), documented on `MatchReplaceMatcherInput.value` and in F-MR-1; tested.
- Verification assumed one desktop process and up to 1000 rules per project. `make check` passed
  (43 Vitest files, 351 tests), `make gen` left the tree clean, and all changed files matched the task's
  paths. The mock findings were reproduced through the generated schema with graphql-js. The transport
  findings were checked against the serialized size and the server's existing HTTP 413 test.
- Browser E2E and `make test-int` were skipped. This branch adds a contract, stubs and a mock.

### Lead escalation after review of `02e8b35`

- Verdict is `changes_requested`. The previous findings are resolved. The review protocol's three-round limit has been
  reached. The lead needs to decide the tester's result limit and error behavior before T-102 and T-108 implement it.
- [x] [major] `internal/api/graphql/schema/matchreplace.graphqls:327` — The tester caps the request and leaves the result
  uncapped. A short literal that occurs throughout a body can expand a modest fixture to tens of MiB even with an
  ordinary replacement string. The mock already builds the entire replacement with `join` and base64-encodes it on
  the UI thread (`web/src/lib/api/mocks/matchreplace.ts:162`). Larger valid inputs can exhaust memory or fail to return
  the documented result, even with one desktop user. → Define a tester output limit and a documented error when it is
  exceeded. Enforce the limit while producing replacement bytes, before allocating or base64-encoding the full result.
  Cover a small input whose replacement crosses the limit in the mock and downstream tester.
  → Lead decision: tester `output` ≤ 16 MiB (base64 ≈ 21.3 MiB response); larger is `BAD_INPUT`
  "tester output must be at most 16 MiB", detected from the replacement growth before the result is built,
  no partial output. In F-MR-1 size budget and the schema; mock checks growth before `join`; new criteria in
  T-102 (resolver) and T-108 (UI shows the message). Test: 64 KiB of `b`, 300-byte term → `BAD_INPUT`.
- [x] [minor] `web/src/lib/api/mocks/matchreplace.ts:421` — The tester accepts a valid response encoded as base64 with its
  required trailing padding removed. Reproduced through graphql-js. The Go `Bytes` scalar uses
  `base64.StdEncoding.DecodeString` and returns `BAD_INPUT` for that input. → Validate the padded form in the mock and
  add the unpadded response to the existing rejection cases.
  → Fixed: `raw` length must be a multiple of 4; unpadded response added to the rejection cases.
- Verification: all changed files are in scope. `make check` passed (43 Vitest files, 353 tests).
  `make gen` left the tree unchanged. A standalone serialization check for size-budget fixtures with heavily
  escaped text produced a 23,903,217-byte import request and a 29,754,518-byte tester request. Both fit the
  33,554,432-byte HTTP cap. The output expansion finding was assessed from the data flow without a large allocation.
- Browser E2E and `make test-int` were skipped because this branch adds a contract, stubs and a mock.

### Follow-up on the lead decision in `5aac666`

- Verdict is `changes_requested`.
- [x] [major] `web/src/lib/api/mocks/matchreplace.ts:165` — `FULL` skips the output budget.
  A response with 17 short headers and a 1 MiB replacement header returned 17,825,845 decoded
  bytes through GraphQL. Count the growth from `FULL` before assembling the response and test
  that an oversized result returns `BAD_INPUT`.
  → Fixed: `applyRule` plans each replacement (length + builder) and counts the exact growth for
  FULL, STRING and header ADD before building; the 17 × 1 MiB FULL header case is tested.
- [x] [major] `web/src/lib/api/mocks/matchreplace.ts:163` — The fixed 32-byte allowance rejects
  valid STRING results. A response without `Content-Length` whose body replacement produces
  exactly 16,777,216 bytes returned `BAD_INPUT`. Account for the actual framing size and test
  the inclusive 16 MiB boundary.
  → Fixed: no fixed allowance; the rewritten `Content-Length` header's length difference is counted
  exactly. Test: a result of exactly 16,777,216 bytes passes, one byte more is `BAD_INPUT`.
- [x] [minor] `web/src/lib/api/mocks/matchreplace.ts:430` — The mock rejects a padded base64
  response with a trailing CRLF and accepts the same response with four trailing spaces.
  The Go `Bytes` scalar accepts the CRLF and rejects the spaces. Strip CR and LF before
  checking padding and reject other whitespace before calling `atob`.
  → Fixed: CR/LF stripped, then the padded alphabet is required; trailing CRLF and spaces tested.
- Verification used the generated schema with graphql-js and assumed one desktop process with
  up to 1000 rules. `make check` passed (43 Vitest files, 353 tests). `make gen` left the tree
  unchanged before this review note. Browser E2E and `make test-int` were skipped.

### Review of `8490278`

- Verdict is `changes_requested`.
- [x] [major] `web/src/lib/api/mocks/matchreplace.ts:186` — The body size check runs before it
  accounts for the rewritten `Content-Length`. A response with a 17,362-byte body and a
  zero-padded 40-digit length returns `BAD_INPUT` for a replacement whose final output is
  exactly 16,777,216 bytes. The same response with an unpadded length returns that exact size.
  Check the combined body and framing change before building the replacement. Add the padded
  length case to the boundary test.
  → Fixed: body growth and the rewritten `Content-Length` difference are counted in one check
  before building; the 40-digit zero-padded case passes at exactly 16 MiB and fails one byte over.
- [x] [major] `web/src/lib/api/mocks/matchreplace.ts:183` — Spreading all the rebuilt headers into
  `splice` exceeds V8's function argument limit. A paste with 200,000 short header lines, about
  1.2 MB in and out, returns a GraphQL error without a code. Keep the mapped array or assign
  entries by index. Verify the large header list on V8, since the Bun runtime accepted it.
  → Fixed: rebuilt headers are assigned by index; a 200,000-header test passes under both
  `bunx --bun vitest` and Node v26 (`bunx vitest`).
- Verified both cases through graphql-js with the generated schema on Node. All 24 focused
  Vitest tests passed. Review assumes one desktop process and the documented tester byte caps.
  Full `make check`, browser E2E, and Go integration tests were skipped.

### Follow-up on the lead decision in `7aa62dc`

- Verdict is `changes_requested`. The previous two findings are resolved. Return this follow-up to the lead under the review round limit.
- [x] [major] `web/src/lib/api/mocks/matchreplace.ts:172` — The mock converts the same STRING matcher to bytes for every header line. With 1000 short headers and a 64 KiB matcher that matches none of them, the mock tester took about 2.5 seconds in Bun and returned unchanged output. The documented limits allow a 1 MiB matcher and far more headers. The same work runs synchronously in the browser. Convert the STRING matcher to bytes once per test before scanning headers.
  → Fixed: the STRING matcher is converted once per test in `applyRule`; a 1000-header, 64 KiB
  no-match case is tested (unchanged output, < 1 s; the old code took ~1.5 s there).
- Verification assumed one desktop process and inputs inside the tester byte caps. `make check` passed with 353 web tests. `make gen` left the tree clean. All 14 changed files are in scope. Browser E2E and Go integration tests were skipped.

### Follow-up on `9fcfc66`

- Verdict is `changes_requested`. The matcher is now encoded once. The review round limit
  has been reached. The remaining findings go to the lead.
- [x] [major] `web/src/lib/api/mocks/matchreplace.ts:174` — The tester splits a near-12 MiB
  body of repeated `a` into about 12.6 million entries before checking the output limit.
  With a two-byte replacement, the result exceeds the cap, but V8 has already allocated
  about 96 MiB for the split array. Count non-overlapping matches without storing them,
  then build the replacement after the framing-inclusive size check.
  → Fixed: STRING matches are counted with an `indexOf` loop (no array), and the replacement is
  built with `replaceAll` only after the size check; near-12 MiB `a` × 2-byte term tested as `BAD_INPUT`.
- [x] [minor] `web/src/lib/api/mocks/matchreplace.ts:456` — The mock accepts a bare CR
  followed by text in the response status line. GraphQL returned `changed: true` for
  `HTTP/1.1 200\rX\r\n\r\noriginal` with a response-body rule. Validate the entire
  status line through its CRLF and add a malformed-input regression case.
  → Fixed: response status line must be `HTTP/1.x NNN[ reason]\r\n` with no CR/LF in the reason;
  the bare-CR case is rejected and a reason-less `HTTP/1.1 204\r\n` is accepted (both tested).
- All 24 focused Vitest tests passed. The malformed input was checked through the generated
  GraphQL schema and the allocation measured in Node/V8. Review assumes one desktop user
  and inputs within the documented tester size limits. Full checks and browser E2E were skipped.

### Follow-up on `2895b16`

- Verdict is `changes_requested`. The previous findings are resolved. Return this follow-up to the lead under the review round limit.
- [x] [major] `web/src/lib/api/mocks/matchreplace.ts:177` — Dense STRING replacements still build a large
  intermediate result when the output fits the cap. Through the mock GraphQL mutation in Node v26 (V8),
  changing each `a` to `b` in a response just under 12 MiB produced a 12 MiB result and peaked at
  624 MiB resident memory. The output-size check passes, and the synchronous `replaceAll` call took
  about a second in the full mutation. Build the approved result in bounded batches so temporary
  memory does not grow with the match count. Test a dense replacement that stays within the limit,
  including literal dollar signs in the term.
  → Fixed: `replaceBytes` scans with `indexOf` and joins every 8192 parts into a chunk, so the
  temporary parts array stays bounded. The term is inserted literally, with no `$` patterns. In Node v26, a
  12 MiB `a`→`b` run grew RSS by 89 MiB and took 0.38 s (`replaceAll`: 470 MiB and 0.74 s). A test replaces
  4 Mi `a` with `$&` and checks the exact output. The four whitespace-only lines are `make gen` output,
  which may not be edited by hand.
- Verification assumed a single desktop user and the documented tester size limits. `make check`
  passed (43 Vitest files, 353 tests). `make gen` left the tree clean before this review note.
  `git diff --check` flagged four whitespace-only lines emitted in the generated web schema.
  Browser E2E and Go integration tests were skipped.

### Review of `f99c64c`

- Verdict is `changes_requested`.
- [x] [major] `web/src/lib/api/mocks/matchreplace.ts:209` applies body rules to chunk
  framing. Replacing the one-byte body `a` with `bb` in a chunked response returns
  a chunk prefixed with size `1`. Apply body replacements to the payload and rebuild
  the framing, or document and enforce a mock limit that leaves chunked bodies unchanged.
  Add a regression case for a replacement that changes the payload length.
  → Fixed (documented mock limit): body rules skip messages with `Transfer-Encoding` or
  `Content-Encoding` (output unchanged, `changed: false`), stated in the `applyRule` `shortcut:` comment and
  the Handoff. Real re-framing is T-102. Chunked `a`→`bb` and a gzip-coded case are tested.
- [x] [major] `web/src/lib/api/mocks/matchreplace.ts:205` keeps a tuple and builder
  closure for each header. A 12,000,019-byte response with 2 million short headers,
  changing each value from `1` to `2`, returned successfully in Node v26.12.0 but took
  979 ms and peaked at 756 MiB RSS (93 MiB before the call). Count growth in one pass
  and build the result in bounded batches in a second pass without retaining per-header plans.
  → Fixed: pass one counts growth per header with `measure` (no tuples/closures), pass two builds
  in 8192-line joined batches. Same 2M-header case in Node v26: 588 ms, max RSS +257 MiB (was 979 ms, +663 MiB);
  the rest is the split line array and base64 strings.
- All 24 focused Vitest tests passed. Both findings were reproduced through the generated
  schema with graphql-js on Node. Review assumed one desktop user and the tester's
  documented input and output caps. Full checks, browser E2E, and Go integration were skipped.

### Review of `506f0d5`

- Verdict is `changes_requested`. The chunked and content-coded body shortcut is covered by the
  focused tests. The following finding goes to the lead under the review round limit.
- [x] [major] `web/src/lib/api/mocks/matchreplace.ts:221` — Body replacement can create invalid
  request framing. A valid POST with two identical `Content-Length: 1` fields and body `a`
  becomes a POST with lengths `2` and `1` after a FULL replacement with `bb`. The project
  parser accepts the input and rejects the output. A header-only POST given the same rule gains
  bytes without a length field, so the parser sees an empty body. When the body changes, emit
  one correct `Content-Length` if one was present or a request gains a body. Include that exact
  header change in the output budget, preserve original bytes on a no-op, and test both cases.
  → Fixed: a changed body drops every `Content-Length` and emits one correct field (at the first one's
  position, or appended when a request gains a body); the removed and added header bytes are counted in
  the same pre-build output check. Tests: duplicate lengths → one `Content-Length: 2`, header-only POST
  gains `Content-Length: 2`, a no-op on duplicate lengths returns the original bytes.
- All 24 focused Vitest tests passed. Both framing cases were reproduced directly through the
  mock resolver using synthetic request bytes. Review assumed one desktop user within the
  documented tester limits. Full `make check`, browser E2E and Go integration were skipped.

### Review of `5328647`

- Verdict is `changes_requested`. The duplicate-length and header-only request
  regressions pass.
- [x] [major] `web/src/lib/api/mocks/matchreplace.ts:220` — A FULL response-body
  rule with term `bb` appends those bytes to a valid 204 No Content response and
  reports `changed: true`. The wire parser treats 1xx, 204 and 304 responses as
  bodyless (`internal/rawhttp/read.go:354`), so the extra bytes are outside the
  response. Skip body rules for those statuses and cover the 204 case in the tester.
  → Fixed: body rules leave 1xx, 204 and 304 responses unchanged (`changed: false`), stated in the
  `applyRule` `shortcut:` comment; tested with 204, 304 and 101 and a FULL `bb` term.
- Reproduced through the mock resolver with synthetic bytes. All 24 focused
  Vitest tests passed. Review assumed a single desktop user and the documented
  tester size caps. Full checks, browser E2E, and Go integration were skipped.

### Review of `e0742c3`

- Verdict is `changes_requested`. Review assumed one desktop user within the documented tester limits.
- [x] [major] `web/src/lib/api/mocks/matchreplace.ts:506` — The tester accepts a truncated request
  whose Content-Length is 4 but whose body contains one byte. Through the generated GraphQL schema,
  a FULL body replacement returned `changed: true` and rewrote the length to 2. Validate fixed-length
  body completeness before applying the draft, and cover the `BAD_INPUT` result in the mock test.
  → Fixed: without `Transfer-Encoding`, every `Content-Length` must be digits equal to the body length
  (`BAD_INPUT`); a response with an empty body may declare any length (HEAD reply, 304). In the schema
  (`testMatchReplace`) and F-MR-1 tester details. Tests: lengths 4/`a`, 1/`ab`, `x`/`a` rejected; an
  empty-body response with `Content-Length: 4` accepted.
- [x] [minor] `internal/api/graphql/schema/matchreplace.graphqls:209` — Matcher input documentation
  says a STRING value is always required, while ADD ignores the matcher. A header ADD with
  `matcher: {kind: STRING}` succeeds in the mock. Document the ADD exception in the matcher and
  rule input descriptions so downstream implementations agree.
  → Fixed: `MatchReplaceMatcherInput.value`, `MatchReplaceRuleInput` and F-MR-1 API details say `ADD`
  ignores the matcher (any kind and value, stored as given).
- All 24 focused Vitest tests passed. Full `make check`, generation, browser E2E and Go integration
  were skipped.

### Review of `bdb885e`

- Verdict is `changes_requested`. Escalate to the lead under the review round limit.
- [x] [major] web/src/lib/api/mocks/matchreplace.ts:512 — The empty-response exception skips
  all length-field validation. An empty 200 response with `Content-Length: x` or conflicting
  lengths of 4 and 5 succeeds through GraphQL. A FULL body draft even rewrites either case
  to length 2 and reports a change. Validate length syntax and agreement between duplicates
  before waiving the body-size comparison, as the wire parser does at
  `internal/rawhttp/read.go:346`. Add both rejection cases.
  → Lead decision (escalation): every `Content-Length` must be decimal and all must agree, always (`BAD_INPUT`);
  only the body-size comparison is waived for an empty response. In the schema and F-MR-1. Tests: `x` and 4/5 on an
  empty 200 rejected.
- [x] [major] web/src/lib/api/mocks/matchreplace.ts:225 — The mock accepts an empty 200
  response with length 4 under the documented HEAD exception, then a FULL body draft adds
  `bb` and rewrites the length to 2. For the length-mismatch case admitted by this exception,
  preserve the response bytes on body drafts and document the policy for T-102. Extend the
  HEAD fixture to assert the result of a FULL draft.
  → Lead decision (escalation): an empty response declaring a non-zero length is treated as a `HEAD` reply; body rules
  leave it unchanged (`changed: false`). Binding for T-102 via F-MR-1 API details and the `testMatchReplace` doc. An
  empty 200 with `Content-Length: 0` is a real empty body and a FULL rule fills it. Both cases tested.
- Both findings were reproduced through graphql-js against the generated schema with
  synthetic messages. Review assumed one desktop user within the documented tester limits.
  `make check` passed (43 Vitest files, 353 tests). `make gen` left the tree clean.
  Browser E2E and Go integration were skipped because no browser UI, proxy, store, or
  raw HTTP implementation changed.

### Review of `fc2102f`

- Verdict is `changes_requested`. Escalate to the lead under the review round limit.
- [ ] [major] web/src/lib/api/mocks/matchreplace.ts:522 — Duplicate lengths are compared as
  JavaScript numbers. An empty 200 response with lengths 9007199254740992 and 9007199254740993
  succeeds with `changed: false` through the generated GraphQL schema. Both strings round to
  the same number, so the empty-response exception then admits the message. Compare the decimal
  strings exactly after removing leading zeroes. Add this pair to the existing rejection cases
  and preserve acceptance of numerically equal values such as 04 and 4.
- The earlier findings are resolved by the existing regression tests. `make check` passed
  (43 Vitest files, 353 tests), and `make gen` left the tree unchanged. Review assumed one
  desktop user within the documented tester size limits.
- Browser E2E and Go integration were skipped. This task changes the contract, stubs and mock.
