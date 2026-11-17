---
id: T-003
title: Svelte 5 + Vite + Bun scaffold with app shell, router and embedding
milestone: M0
role: frontend
status: in_review
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
