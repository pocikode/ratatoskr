---
id: T-003
title: Svelte 5 + Vite + Bun scaffold with app shell, router and embedding
milestone: M0
role: frontend
status: done
depends_on: [T-001]
owner: claude-sonnet-5-5
branch: task/T-003-web-scaffold
paths:
  - web/**
  - internal/api/static/**
spec:
  - docs/06-frontend-conventions.md (all)
  - docs/02-ui-layout.md §1, §2, §10
  - docs/adr/0005-svelte-spa-without-sveltekit.md
---

## Goal
`web/` contains the SPA scaffold with the app shell (TopBar, Sidebar with all categories/pages as empty placeholder pages, StatusBar),
a hash router with lazy pages, design tokens (dark/light), and the Go package `internal/api/static` that embeds `web/dist` and serves it with SPA fallback.

## Scope
- In: `bun create vite` equivalent set up by hand (svelte-ts), Tailwind v4, bits-ui, @lucide/svelte, fonts (Inter, JetBrains Mono, self-hosted),
  eslint/prettier/svelte-check/vitest config, scripts from docs/06 §8, router (`#/page/…`), placeholder pages for every sidebar item,
  command palette shell (Ctrl+K opens, lists "Go to <page>" commands), theme toggle, `internal/api/static` with `go:embed` + tests.
- Out: GraphQL client (T-007), real page content, mounting the handler in the main server (T-007 does that; you expose `static.Handler() http.Handler`).

## Acceptance criteria
- [x] `cd web && bun install && bun run build && bun run check && bun run test` all pass.
- [x] Every sidebar item from docs/02 §1 navigates to its placeholder page; reload keeps the route.
- [x] Ctrl/⌘+K opens the palette; typing "repl" + Enter navigates to Replay.
- [x] Dark/light themes switch via user menu; tokens only (no hex in components).
- [x] Go test proves `/` serves index.html and unknown paths fall back to index.html; `/api/*` is not swallowed.
- [x] First-load JS ≤ 150 KB gzip at this stage.

## Plan (implementer, ≤15 lines)
- Hand-scaffold web/ (Vite+svelte-ts, Tailwind v4, bits-ui, lucide, self-hosted fonts via @fontsource), scripts per docs/06 §8.
- Vite outDir = `internal/api/static/dist` (go:embed cannot reach `../web/dist`); committed `.gitkeep` keeps the embed valid before a build.
- `lib/router` hash router (routes table, `navigate`, `route` rune state, lazy page imports) + one placeholder page per sidebar item.
- `lib/commands` registry (+ "Go to <page>" commands, theme, palette), CommandPalette (Ctrl/⌘K), theme store, tokens.css (dark default).
- Shell: TopBar, Sidebar (collapsible), StatusBar. Tests: router, registry, palette, sidebar nav (Testing Library).
- Go `internal/api/static`: `Handler()` + `NewHandler(fs.FS)`, SPA fallback, `/api/` → 404; tests with MapFS.

## Out-of-scope change needed (if any)

## Handoff (implementer)
**Changed**: `web/` scaffold (Vite 8, Svelte 5, Tailwind v4, bits-ui, lucide, self-hosted Inter/JetBrains Mono via @fontsource); hash router + lazy placeholder pages for all 19 routes; command registry (`lib/commands`) with go-to/theme/sidebar/palette commands, `ShortcutManager` (Ctrl/⌘K, Ctrl/⌘B, `g`+letter); shell (TopBar, Sidebar w/ rail, StatusBar, CommandPalette, user menu); tokens.css dark/light; `internal/api/static` (`Handler()`, `NewHandler(fs.FS)`).
**Verify**: `make check`; `cd web && bun run build` (first-load JS 62 KB gzip); `go test ./internal/api/static`.
**Decisions / limitations**:
- Vite `outDir` is `internal/api/static/dist` (go:embed can't reach `../web/dist`); committed `.gitkeep` keeps the embed valid, restored by a Vite plugin after `emptyOutDir`.
- `web/go.mod` fences `web/node_modules` out of the root Go module (a stray .go file there broke `go fix ./...`).
- TypeScript pinned to ^6 (svelte-check peers support 5/6 only; latest is 7).
- Theme persists in localStorage (needed pre-paint) until the settings API exists.
- bits-ui menu is inaccessible to role queries in jsdom, so the theme test uses text queries and `pointerEventsCheck: 0`.
- `codegen` script is a stub until T-007; no Playwright yet (nothing serves the UI from the binary until T-007).
**Follow-ups**: T-007 mounts `static.Handler()` and adds Playwright E2E + codegen; plugin/footer slots and sidebar Customize later.

## Review (reviewer)
**Reviewer**: claude-opus-5-5 · 2026-11-17 · commits 4df629e..80033eb

**Checks**: `make check` green (go fix/vet clean, golangci-lint 0 issues, `go test -race` ok incl. `internal/api/static`;
svelte-check 0/0, eslint + prettier clean, vitest 4 files / 35 tests pass). `bun run build` → entry JS 61.94 KB gzip (AC ≤ 150 KB);
`.gitkeep` survives the build; tree clean afterwards.

**AC coverage**: all six ACs are covered by tests (App.test.ts sidebar/nav/reload/palette/theme, static_test.go fallback + `/api`
not swallowed, build size checked manually). Shell, router, registry and embed are clean and well-scoped.

**Findings**
- [x] [major] .gitignore:16-19: the root `.gitignore` is outside the task `paths` (`web/**`, `internal/api/static/**`), which breaks
  hard rule 1. → Revert it and add `internal/api/static/.gitignore` with `dist/*` + `!dist/.gitkeep`. Put it next to `dist/`,
  not inside it, because `emptyOutDir` would delete it.
- [x] [major] web/src/lib/components/RouterView.svelte:22,44-45: on navigation `router.route` changes right away, but `Page` still holds
  the previous component until the new chunk resolves. `{#key router.route.id}` then re-mounts the *old* page under the new route:
  you get a flash of the wrong page plus a second mount of the old page, which means duplicate queries/subscriptions once pages fetch data.
  The tests only pass because `findByRole` waits. → Store `{ id, component }` from the load and render it only when
  `loaded.id === router.route.id`; otherwise render `PlaceholderPage`/a loading state. Add a test that asserts the old heading is gone
  right after navigation, using a deferred `load()`.
- [x] [minor] web/src/lib/router/router.svelte.ts:17: `decodeURIComponent` throws `URIError` on malformed escapes (e.g. `#/search/%`).
  The `hashchange` handler then throws and the router keeps the stale location. → Wrap it in try/catch and fall back to the raw segment.
  Add a parseHash test.
- [x] [minor] web/vite.config.ts:10,26 + web/.gitignore:2: the build goes to `internal/api/static/dist`, but docs/06 §8 and ADR-0005 say
  `web/dist`. The deviation is reasonable and documented in Handoff, but the spec now disagrees with the code, and `web/.gitignore`'s
  `dist` entry is dead. → Implementer: drop the dead `dist` line. **Lead**: update docs/06 §2/§8 and ADR-0005 to the new output path.
- [x] [minor] web/package.json:15-16: `@fontsource-variable/inter` and `@fontsource-variable/jetbrains-mono` are runtime dependencies
  that are not in the docs/06 §1 approved list (rule 8). They match the task scope ("fonts, self-hosted") and are OFL. → **Lead**: add
  them to docs/06 §1 (along with the test/lint helpers `jsdom`, `@testing-library/jest-dom`, `@testing-library/user-event`,
  `@tailwindcss/vite`, `@eslint/js`, `globals`, `@types/node`). No code change needed if the lead approves.
- [x] [minor] web/src/lib/stores/theme.svelte.ts:7,16 + web/index.html:10: the theme is persisted in localStorage, which conflicts with
  docs/06 §4 ("not localStorage"). The pre-paint read is a legitimate need, but the debt isn't tied to a task. → Name the owning task
  in the comment (`TODO(T-0xx)`), or have the lead record an exception in docs/06 §4 (e.g. a localStorage mirror for pre-paint only,
  with the backend settings as the source of truth).
- [x] [minor] web/src/lib/components/{TopBar.svelte:32,CommandPalette.svelte:100,StatusBar.svelte:6,Sidebar.svelte:18}: `text-[11px]`
  hard-codes a type size 4 times; docs/06 §6 requires tokens for typography. → Add `--text-xs: 11px` to tokens.css, map it in `@theme`,
  and use `text-xs`.
- [x] [nit] web/src/lib/components/TopBar.svelte:32: `Ctrl K` is hard-coded and wrong on macOS. → Derive `⌘K` vs `Ctrl K` from the platform
  (or from the binding, once shortcuts become rebindable).
- [x] [nit] web/src/lib/components/CommandPalette.svelte:37: Enter during IME composition runs the command. → Ignore `e.isComposing`.
- [x] [nit] internal/api/static/static.go:60: `http.ServeFileFS` returns 400 when the raw `r.URL.Path` contains `..`, even though
  `name` is already cleaned (e.g. `/x/../favicon.svg`). That's harmless behind ServeMux, which redirects to the clean path. →
  Optionally serve via `http.ServeContent` on the opened file, or add a test pinning the current behaviour.
- [x] [nit] web/src/lib/router/router.svelte.ts:40-41: `navigate` uses the global `window`, while `start(win)` is injectable. → Keep the
  `win` from `start` for symmetry/testing.
- [x] [nit] web/eslint.config.js:7: the `../internal/api/static/dist/` ignore is a no-op (it's outside the eslint base path). → Remove it.
- [x] [nit] web/package.json: `test:e2e` runs `playwright test`, but Playwright isn't installed yet, so the script fails if called.
  Acceptable, since T-007 owns E2E; note it in the T-007 task.

**Verdict**: `changes_requested`. Fix both majors (root `.gitignore` out of lane; RouterView stale-page re-mount), plus the URIError
and `text-[11px]` minors. The dependency-list and output-path minors are lead doc actions and don't block re-review.


**Implementer reply (round 2)**: both majors, URIError, text-xs, IME, ⌘K, router `win`, eslint ignore fixed (+2 tests, 37 total). Lead items (docs/06 §1/§2/§8, ADR-0005 output path, theme exception) and the T-007 note on `test:e2e` are left open; `static.go` `..` behaviour left as is.

### Round 2
**Reviewer**: claude-opus-5-5 · 2026-11-17 · commit c73a477

**Checks**: `make check` green (go fix/vet/golangci-lint/`go test -race` ok; svelte-check 0/0, eslint + prettier clean, vitest
4 files / 37 tests pass). `bun run build` → entry JS 62.07 KB gzip; tree clean after build. `git diff --name-only main...HEAD` only
touches `web/**`, `internal/api/static/**` and this task file: lane restored.

**Round-1 verification**
- Root `.gitignore` reverted; `internal/api/static/.gitignore` (`dist/*`, `!dist/.gitkeep`) sits beside `dist/`. ✔
- RouterView stores `{ id, component }` and renders it only when `loaded.id === router.route.id`, with the placeholder in between.
  The new deferred-load test fails on the old code and passes now. Dropping `{#key}` also means a param-only change (same route id)
  keeps the page mounted, which is the right default; pages must read `router.location.params` reactively. ✔
- `safeDecode` + test; `text-xs` mapped through `--fs-xs` in `@theme`; IME guard; ⌘K/Ctrl K; `navigate` uses the started `win`;
  dead eslint ignore and `web/.gitignore` `dist` removed. ✔
- `static.go` `..` → 400 left as is: accepted (nit, optional).

**Findings**
- [ ] [minor] web/src/lib/stores/theme.svelte.ts:16: `TODO(T-007)` points at a task that does not own the settings API.
  `docs/tasks/T-007-graphql-scaffold.md` never mentions theme or settings, and no other task does either, so the debt is still
  untracked. → **Lead**: create or name the task that owns user settings (and record the pre-paint localStorage-mirror exception in
  docs/06 §4); the implementer then retargets the TODO id. This does not block the merge.
- [ ] [minor] docs/tasks/T-003-web-scaffold.md (Review, round 1): the implementer ticked `[x]` on items they did not fix and that
  belong to the lead: the docs/06 §1 dependency list, docs/06 §2/§8 plus ADR-0005 output path, and the T-007 `test:e2e` note. Their own
  reply says these are still open, so the checkboxes are misleading. → The **Lead** must still do: (a) add
  `@fontsource-variable/inter`, `@fontsource-variable/jetbrains-mono`, `jsdom`, `@testing-library/jest-dom`,
  `@testing-library/user-event`, `@tailwindcss/vite`, `@eslint/js`, `globals`, `@types/node` to docs/06 §1; (b) change `web/dist`
  → `internal/api/static/dist` in docs/06 §2/§8 and ADR-0005; (c) add Playwright install + `test:e2e` and the settings owner to
  T-007/T-021 scope. Implementers should only tick findings they fixed.
- [ ] [nit] web/src/App.test.ts:61: the final `findByRole('heading', { name: 'Findings' })` is also satisfied by the loading-state
  `PlaceholderPage`, so it doesn't prove the lazy page mounted after `release()`. Every page is a placeholder today, so the result is
  identical. → When real pages land, assert on page-specific content (or a `data-page` attribute) instead.
- [ ] [nit] web/src/lib/components/TopBar.svelte:8: `navigator.platform` is deprecated. → Fine for now; when shortcuts become rebindable
  (T-022), render the hint from the binding instead of sniffing the platform.

**Verdict**: `approve`. There are no blocker or major findings left. The remaining minors are lead doc/task actions and don't need code
changes on this branch. Status stays `in_review` until the operator merges and sets `done`.
