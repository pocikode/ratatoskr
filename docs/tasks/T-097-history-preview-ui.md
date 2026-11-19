---
id: T-097
title: History response Preview tab + "Open in browser" (follow-up of T-017, unblocks T-021)
milestone: M1
role: frontend
status: in_review
depends_on: [T-017]
owner: Codex
branch: task/T-097-history-preview-ui
paths:
  - web/src/pages/http-history/**
  - web/src/lib/components/MessageViewer/**
  - docs/tasks/T-017-http-history-page.md   # Known limitations only
spec:
  - docs/01-feature-spec.md#F-HIST-2 (Preview contract paragraph)
  - docs/02-ui-layout.md (MessageViewer row)
---

## Goal
The History response pane offers the sandboxed Preview mode and an "Open in browser" action, both pointing at
`/api/rest/preview/{id}`.

## Scope
- In: pass `previewUrl` to the response `MessageViewer`; an "Open in browser" control in the viewer toolbar; tests.
- Out: the server route (T-096, runs in parallel; the URL is fixed by the spec), request-side preview, E2E (T-021).

## Acceptance criteria
- [x] The History response viewer shows tabs Raw · Pretty · Hex · Table · Preview. The request viewer is unchanged (no Preview).
- [x] Preview renders `<iframe sandbox="">` with `src="/api/rest/preview/{id}"` of the selected request (relative URL,
      so it works on `localhost` and on `ratatoskr.localhost`).
- [x] When `previewUrl` is set, the viewer toolbar shows an "Open in browser" link that opens the same URL in a new tab
      (`target="_blank"`, `rel="noopener noreferrer"`), reachable by keyboard and with an accessible name.
- [x] Selecting another request updates both the iframe `src` and the link.
- [x] Vitest: tab list per pane; iframe `src`/`sandbox`; link `href`/`target`/`rel`; src follows selection.
- [x] `make check` green.

## Notes from lead
- `MessageViewer` already supports `previewUrl` and adds the Preview tab when it is set; pass
  `modes={['raw','pretty','hex','table','preview']}` (or drop `modes`) on the response viewer only.
- Use a plain `<a>`, not `window.open`, and no REST client call. Keep existing roles/labels stable (T-021 selectors).
- Remove the T-017 known-limitation wording about `/preview/{id}` that this makes obsolete.

## Plan (implementer, ≤15 lines)
- Pass the selected request's relative preview URL to the History response viewer.
- Reuse the sandboxed Preview pane and add a toolbar link to the same URL.
- Cover both pane tab lists, link attributes, iframe attributes and selection changes in Vitest.
- Update the T-017 limitation and run `make check`.

## Out-of-scope change needed (if any)

## Handoff (implementer)
- What changed: History passes the loaded request's relative preview URL to the response viewer. The sandboxed iframe
  and the keyboard-accessible "Open in browser" link share that URL and follow row selection. Removed the obsolete
  preview limitation from T-017.
- How to verify: `make check` passed, including 231 web tests. The History page test covers both pane tab lists, link
  focus and attributes, iframe attributes, selection changes, and a row without a response. Headless Chromium with a
  stub preview response confirmed iframe rendering and new-tab navigation in both themes.
- Known limitations: Captured response bodies require the T-096 server route. The browser smoke check used a stub.
- Follow-ups (proposed tasks): T-021 covers end-to-end verification with the server route.

## Review (reviewer)
- Verdict: approve
- Checks: `make lint test web-check web-test` green (golangci-lint 0 issues, go test -race ok, svelte-check 0 errors/0 warnings,
  vitest 231/231). The first `make check` run stopped at lint only because another worktree was running golangci-lint at the
  same time. The rerun passed. `make test-int` was not needed (no proxy/store/rawhttp change).
- Scope: all 5 changed files are inside `paths`. There is no schema, generated-code or Go change.
- AC: all covered by `HttpHistoryPage.test.ts`: tab lists per pane, request pane has no link, link href/target/rel and
  keyboard focus after Copy, iframe `src`/`sandbox=""`, src and href follow the selection (11 → 10), and no link
  for a row without a response. Link and iframe both come from `history.detail`, so they always match the body shown.
  The plain `<a>` with `noopener noreferrer` follows the lead's note. The relative URL matches the F-HIST-2 contract.
- Findings:
  - [ ] [nit] web/src/lib/components/MessageViewer/MessageViewer.test.ts:138 — fixture still uses the old `/preview/42`
        path, while the prop doc now says `/api/rest/preview/{id}` → use `/api/rest/preview/42` the next time this file is touched.
