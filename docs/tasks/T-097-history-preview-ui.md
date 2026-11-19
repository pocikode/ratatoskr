---
id: T-097
title: History response Preview tab + "Open in browser" (follow-up of T-017, unblocks T-021)
milestone: M1
role: frontend
status: todo
depends_on: [T-017]
owner: ""
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
- [ ] The History response viewer shows tabs Raw · Pretty · Hex · Table · Preview. The request viewer is unchanged (no Preview).
- [ ] Preview renders `<iframe sandbox="">` with `src="/api/rest/preview/{id}"` of the selected request (relative URL,
      so it works on `localhost` and on `ratatoskr.localhost`).
- [ ] When `previewUrl` is set, the viewer toolbar shows an "Open in browser" link that opens the same URL in a new tab
      (`target="_blank"`, `rel="noopener noreferrer"`), reachable by keyboard and with an accessible name.
- [ ] Selecting another request updates both the iframe `src` and the link.
- [ ] Vitest: tab list per pane; iframe `src`/`sandbox`; link `href`/`target`/`rel`; src follows selection.
- [ ] `make check` green.

## Notes from lead
- `MessageViewer` already supports `previewUrl` and adds the Preview tab when it is set; pass
  `modes={['raw','pretty','hex','table','preview']}` (or drop `modes`) on the response viewer only.
- Use a plain `<a>`, not `window.open`, and no REST client call. Keep existing roles/labels stable (T-021 selectors).
- Remove the T-017 known-limitation wording about `/preview/{id}` that this makes obsolete.

## Plan (implementer, ≤15 lines)

## Out-of-scope change needed (if any)

## Handoff (implementer)
- What changed:
- How to verify:
- Known limitations:
- Follow-ups (proposed tasks):

## Review (reviewer)
- Verdict: approve | changes_requested
- Findings:
