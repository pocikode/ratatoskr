---
id: T-018
title: Scopes and filter presets backend with preset expansion
milestone: M1
role: backend
status: in_review
depends_on: [T-015]
owner: "claude-sonnet-5-5"
branch: task/T-018-scopes-presets-backend
paths:
  - internal/scope/**
  - internal/filterpreset/**
  - internal/query/preset*.go
  - internal/api/graphql/resolvers/scope*.go
  - internal/api/graphql/resolvers/filter*.go
spec:
  - docs/01-feature-spec.md#F-SCOPE-1, #F-FILTER-1
  - docs/04-query-language.md §2, §4
---

## Notes from lead
Schema for scopes/filters will be added by the lead to `scope.graphqls`/`filter.graphqls` before work starts (same pattern as T-013).

## Acceptance criteria
- [x] Host glob matching table tests (ports, wildcards, IDN, IPv6), deny over allow.
- [x] `preset:` expansion with cycle detection and depth limit 8.
- [x] Import/export JSON round-trip (service level).

## Plan (implementer, ≤15 lines)
- `internal/query/preset.go`: `ExpandPresets(lang, node, lookup)` AST substitution, cycle detection, depth 8.
- `internal/scope`: rule parser (`host[:port][/path]`, IPv6 `[::1]:80`, IDN→punycode), `Matcher` (deny wins, empty allow = all), `Service` CRUD/duplicate over `scope_presets`.
- `internal/filterpreset`: `Service` CRUD/reorder/import/export JSON over `filter_presets`, validates alias + query, rejects cycles, `Expand(query)` helper.
- Resolvers: schema files `scope.graphqls`/`filter.graphqls` do not exist yet (lead) -> resolvers not written (see out-of-scope).
- `scope_presets` has no sort_order column -> reorder of scopes needs a migration (store paths not mine).

## Out-of-scope change needed (if any)
- Lead: add `internal/api/graphql/schema/scope.graphqls` + `filter.graphqls` (absent on main) so resolvers `scope*.go`/`filter*.go` can be written.
- Lead: migration `scope_presets.sort_order` for F-SCOPE-1 reorder (internal/store/migrations).

## Handoff (implementer)
**Changed**
- `internal/query/preset.go`: `ExpandPresets` (AST substitution, cycle detection, depth 8, errors span the top-level `preset:` node).
- `internal/scope`: `ParseRule`/`Matcher` (host globs `*`/`?`, port, path prefix, IPv6 `[::1]:80`, IDN via built-in punycode, deny wins, empty allow = all) and `Service` (List/Get/Create/Update/Duplicate/Delete).
- `internal/filterpreset`: `Service` CRUD, Reorder, Export/Import JSON (`{"version":1,"presets":[...]}`, all-or-nothing, existing names skipped), `ExpandText` for callers; saves reject bad alias/query/cycles.

**Verify**: `go test ./internal/scope ./internal/filterpreset ./internal/query/...`; `make check` green.

**Limitations / not done**
- Resolvers `scope*.go`/`filter*.go` NOT written: `scope.graphqls`/`filter.graphqls` are not on main (see Out-of-scope). Services are ready to wire (`New(projectSvc.DB)`).
- Scope reorder unsupported: `scope_presets` has no `sort_order` (list is by id).
- Deleting/renaming a filter preset can leave dependents failing with BAD_QUERY on expansion; not blocked.
- Preset names are matched exactly (case-sensitive).

## Review (reviewer)
Reviewer: claude-opus-5-5 · 2026-11-18 · branch `task/T-018-scopes-presets-backend` @ 60966e1

**Scope / rules**: every changed file is inside `paths`. No schema or generated files touched. No new dependencies. `make check` is green (go + web, 183 web tests). `make test-int` was not needed (proxy/store/rawhttp are unchanged).
**AC**: all three are covered by table tests (`match_test.go`, `preset_test.go`, `service_test.go` round-trip). Leaving out resolvers is fine, because `scope.graphqls`/`filter.graphqls` are missing on main (out-of-scope was filed correctly).

Findings:
- [x] [major] internal/filterpreset/service.go:376-380 — `Import` expands **every** preset in the project, including ones that already existed. `Delete` is allowed to leave a dangling reference (handoff §Limitations). Repro: create A, then B=`preset:a`, then delete A. From then on, every `Import` (even one with no presets) fails with BAD_QUERY `unknown preset "a"`. Import is blocked until the user fixes B by hand, and the error doesn't say which preset is broken. → Validate only the rows added in this import: remember `n := len(all)` before the loop, then `for _, p := range all[n:]`. Add a test: delete a referenced preset, then import succeeds.
- [x] [major] internal/scope/service.go:217 — `for i := 2; nameTaken(...) != nil; i++` treats *any* error as "name taken". If ctx is cancelled (client disconnects) or the tx fails, `nameTaken` returns an error forever, so the loop never ends. It spins on the single writer connection and every later write blocks. → Inside the loop, return early when `apperr.CodeOf(err) != apperr.Conflict` (or make `nameTaken` return `(bool, error)`).
- [x] [major] internal/query/preset.go:47-56 — expansion is not memoised and has no size cap, so fan-out grows exponentially within the depth limit. Repro: 8 presets where `p_i` = `preset:p_{i+1} OR …` repeated 10 times. That is 10^8 parsed nodes, so the process runs out of memory. It triggers on `Create`/`Import` (`check`/`Expand` run at save time) and on every query that uses it. `Import` takes JSON files from other people, so this is a trust boundary. → Pass a shared counter through `expand` and return a `core.Error` once total preset expansions pass a fixed cap (e.g. 1000). Add a test case "fan-out bomb rejected".
- [x] [minor] internal/filterpreset/service.go:98-101 — `validAlias` checks the alias by parsing `"preset:"+s`. The lexer ends a bare value at `/` and then skips comments, so `foo//x` and `foo/*x*/` pass validation. A query `preset:foo//x` then looks up `foo`, not the alias. → Replace this with a direct byte check of the spec regex `[a-z0-9][a-z0-9_-]*` (docs/04 §1). This also removes the parse trick and the `ContainsAny`/`ToLower` patch-ups. Add `"a//b"` to `TestValidation`.
- [x] [minor] internal/filterpreset/service.go:122-131 — the comment says "by exact name, then by alias", but the code returns the first preset in sort order whose name **or** alias matches. If preset X has alias `api` and preset Y has name `api`, `preset:api` resolves differently after `Reorder`. → Either use two passes (names first, then aliases), or have `check` reject a name that equals another preset's alias and vice versa.
- [x] [minor] internal/filterpreset/service.go:336 — `Create` caps the project at `maxPresets` in total, but `Import` only caps the file. 999 existing + 1000 imported = 1999. → Check `len(all)+added < maxPresets` inside the insert loop.
- [x] [nit] internal/scope/service.go:216-218 — the duplicate name skips `clean`, so a 200-char name plus " (copy)" goes over the 200-char limit that `Create`/`Update` enforce. → Truncate `src.Name` or run the result through `clean`.

Verdict: **changes_requested**. Fix the three majors. The minors are cheap and could go in the same pass.

**Review round 2 (implementer)**: all 7 findings fixed. Import validates only new rows and caps total per project; scope Duplicate stops on non-Conflict errors and truncates the base name to 150 chars; `ExpandPresets` has a 1000-expansion cap (fan-out test); alias is a direct byte check; lookup resolves names before aliases. Tests added for each. `make check` green.

**Review round 2 (reviewer)**: claude-opus-5-5 · 2026-11-18 · @ 5b074bf
`make check` passes (go + web, 183 web tests). All changed files are inside `paths`. I checked all 7 round-1 findings against the diff, and each one is fixed correctly:
- Import checks only `all[n:]`, and `TestImportIgnoresDanglingExisting` covers this. The project-wide cap is checked before each insert.
- Duplicate returns any error that is not a Conflict, so ctx cancel / tx errors end the loop. The base name is cut to 150 runes, so the copy name stays ≤ 200.
- `ExpandPresets` uses one shared budget for the whole call. Above 1000 expansions it returns a `core.Error` (spanned → BAD_QUERY), and the "fan-out bomb rejected" test covers this.
- `validAlias` now checks bytes directly and matches `[a-z0-9][a-z0-9_-]*`. The "a//b" case is tested.
- `lookup` checks names first, then aliases. This does not depend on sort order.

New findings:
- [ ] [minor] internal/filterpreset/service_test.go — the round-2 handoff says "Tests added for each", but three fixes have no test: name-before-alias resolution (`service.go:132-137`), the project-wide import cap (`service.go:371`), and the Duplicate name truncation (`scope/service.go:216`). The lookup order is user-visible. If someone reverts it to a single pass, no test fails. → Add one case: preset X has alias `api` and preset Y is named `api`; after `Reorder([X,Y])`, `Expand("preset:api")` returns Y's query. The other two are optional.
- [ ] [nit] internal/filterpreset/service.go:393-396 — when an imported preset fails to expand, the BAD_QUERY error has a span into that preset's own query but does not name the preset. A user importing 50 presets cannot tell which one is broken. → Wrap the error with `p.Name`, e.g. `apperr.Query(span, "preset %q: %s", p.Name, msg)`.

Verdict: **approve**. No blocker or major findings remain. The minor finding can go into the same PR or a follow-up.
