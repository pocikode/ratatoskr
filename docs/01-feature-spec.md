# 01 — Feature Specification (parity target: Caido v0.58, Aug 2026)

Each feature has an ID (`F-<AREA>-<n>`). Tasks reference these IDs. "MUST" = required for parity.
Acceptance criteria (AC) are written so a QA agent can turn them into tests.
Layout details live in `docs/02-ui-layout.md`; query syntax in `docs/04-query-language.md`.

Paid-only features in Caido (Assistant, Automate preprocessors, workflow-in-automate, filtered export,
unlimited projects/plugins) are **free and unlimited** here.

---

## A. Core proxy & networking (`F-NET`)

### F-NET-1 Listener
- Default listener `127.0.0.1:8080` serves **both** the proxy and the UI/API (like Hetty/Caido):
  - absolute-form request (`GET http://…`) or `CONNECT` → proxy;
  - origin-form request with `Host` = listener address or `ratatoskr.localhost` → UI/API.
  - origin-form request with any other `Host` → `421 Misdirected Request` (not forwarded) unless the listener has
    invisible mode on (F-NET-6). An absolute-form target equal to the listener address is treated as UI, never forwarded (loop guard).
  - Upstream failures (dial, TLS, timeout, malformed response) → the client gets a Ratatoskr `502` page; the exchange is
    recorded with that `502` as its response and the error text in its body (no separate error column in v1).
- Multiple listeners (address:port list) configurable in Settings → Network and via CLI `--listen` (repeatable).
- AC: `curl -x http://127.0.0.1:8080 http://example.test/` is recorded in history; `http://127.0.0.1:8080/` serves the UI.

### F-NET-2 HTTPS interception (MITM)
- Root CA generated on first start (ECDSA P-256, 10y), stored in instance data dir.
- Per-host leaf certificates issued on the fly from **SNI** (fallback: CONNECT host), cached (LRU, 1024).
- CA can be downloaded (PEM + DER) at `http://ratatoskr.localhost/ca.crt` and from Settings → Certificate;
  can be regenerated, imported (PEM cert+key) and exported.
- ALPN to client: `http/1.1` only (v1.0). Upstream: HTTP/1.1 by default.
- Upstream certificates are **not verified** (testing tool: targets often have self-signed/expired certs). A verify toggle
  in Settings → Network is a later addition.
- AC: browser with CA installed loads `https://example.com` with no warning; request appears with `tls=true`.

### F-NET-3 Raw fidelity
- The proxy parses HTTP/1.x itself (lenient parser); unknown/malformed-but-forwardable messages are forwarded byte-exact.
- Header order, case, duplicate headers, odd whitespace are preserved in storage and on the wire.
- Bodies stored raw (still compressed/chunked as on the wire); UI shows decoded view on demand.
- AC: golden tests with 30+ malformed/edge requests (duplicate CL, obs-fold, lowercase methods, absolute paths with fragments) round-trip byte-exact.

### F-NET-4 WebSocket proxying
- Upgrade handshakes are recorded as normal requests; subsequent frames are recorded as WS messages (see F-WS).
- Supports permessage-deflate (store decompressed payload + flag), continuation frames, ping/pong/close.

### F-NET-5 Upstream proxies
- Ordered list of rules: `{enabled, host glob, type http|socks5, address, auth}`; first match wins; "direct" fallback.

### F-NET-6 Invisible (transparent) proxying
- Listener option: treat origin-form requests as proxy traffic, routing by `Host` header / SNI; used for thick clients
  with DNS/hosts redirection. Optional "redirect to" host:port override per listener.

### F-NET-7 DNS rewrites
- Rules `{host glob → IP}` applied before dialing; plus custom resolver address.

### F-NET-8 Domain allowlist for UI access & local traffic
- Option to proxy (or bypass) traffic to localhost targets; list of hostnames allowed to reach the UI.

### F-NET-9 HTTP/2 upstream (beta flag)
- Replay and Replay Pipeline can send over HTTP/2 when enabled per session (h2 framing done by our engine).

---

## B. Workspace (`F-WSP`)

### F-WSP-1 Projects
- Create / rename / delete / duplicate / select project. Mark one as default (opened at start).
- A **temporary project** is created when none exists (deleted on exit unless saved).
- Project switch is instant for the UI (all stores reset + re-subscribe).
- Unlimited projects.
- AC: deleting a project removes its directory; switching projects never shows stale rows.
- Rules (contract: `project.graphqls`):
  - Names are trimmed; empty, longer than 200 characters, or containing control characters → `BAD_INPUT`. Names need
    not be unique.
  - Renaming a temporary project saves it (it stops being temporary).
  - A temporary project cannot be the default (`BAD_INPUT`). At most one project is the default; `setDefaultProject(null)`
    clears it.
  - Duplicating without a name (or a blank one) names the copy `<source name> (copy)`; the copy is not selected.
  - The project list is ordered by most recently opened first (never-opened projects by creation time).
  - Deleting the selected project selects the default project, or a new temporary project when the deleted one was the
    default or there is no default.
- AC: `renameProject` on a temporary project returns `isTemporary: false`; `setDefaultProject` on a temporary project →
  `BAD_INPUT`; `duplicateProject(id)` without a name → `"<name> (copy)"`; `projects` is ordered by last opened, newest
  first; `createProject(name: "  ")` → `BAD_INPUT` and `createProject(name: " a ")` → `"a"`.

### F-WSP-2 Backups
- Create backup of a project (zip: `project.db` + `files/` + manifest JSON), list, download, restore as new project, delete.
- Backups run as background tasks with progress (see F-UI-9 Tasks).

### F-WSP-3 Data management
- Delete requests matching an HTTPQL query (with preview count). Vacuum DB. Show DB size per project.

### F-WSP-4 Instance auth (remote use)
- `--auth-token` or Settings: when set, UI/API require bearer token (cookie after login page). Guest mode = no token on localhost.

---

## C. Overview group

### F-SITE-1 Sitemap
- Tree derived from history: `scheme://host:port` → path segments → (leaf) request list. Updates live.
- Respects the active Scope preset. Node badges: request count; icon by type (folder/file/query).
- Select node → bottom table of requests under that node (same table component as HTTP History) + request/response panes.
- Shift-click expands a node fully. Context menu: delete node (deletes underlying requests after confirm), add host to scope, copy URL.
- AC: 10k requests over 200 hosts render the tree in < 500 ms; deleting a node removes its rows from history.
- Derivation (T-094 contract, `sitemap.graphqls`), computed at read time from the requests that match the active scope
  preset and query bar; nothing is stored:
  - **Host** node per `(tls, host, port)`, label `http(s)://host:port` (port always shown; IPv6 hosts in brackets).
  - **Path** nodes: a path starting with `/` splits on `/` into its non-empty segments, as stored (not percent-decoded,
    case-sensitive); `/a//b/` → `a`, `b`. Paths not starting with `/` (`*`, absolute-form) and `/` itself have no
    segments and sit directly on the host. Kind **directory** when it has child path nodes, else **file**.
  - **Query** nodes: children of a path node, one per distinct set of query parameter *names* (values ignored) among
    requests whose path ends at that node. Names = non-empty `&`-separated pieces of `req.query` cut at the first `=`;
    label `?a&b` (sorted, deduplicated). Requests with no names stay on the path node only.
  - Count badge = requests at or below the node. Children order: path nodes by label (byte order), then query nodes by label.
- Requests "under a node" are defined by an HTTPQL expression the server returns with the node (`SitemapNode.query`),
  already combined with the query bar the tree was fetched with: the node expression alone when the bar is empty (only
  whitespace/comments), else `(<node expr>) and (<bar>⏎)` (newline ends a trailing `//` comment; HTTPQL rejects `()`).
  The UI passes it unchanged, same scope, for the request table and for delete (`deleteRequests`), never composing
  queries itself. So the confirm dialog's count (the badge) is exactly what gets deleted.
- Live: the page listens to `requestAdded`/`requestUpdated`/`requestsDeleted` with the page's query/scope and refetches
  the roots and the expanded nodes, at most once per second. No dedicated sitemap subscription.
- Shift-click: loads the whole subtree in one call, capped at 5000 nodes (beyond that: error, expand level by level).

### F-SCOPE-1 Scopes
- Scope preset = `{name, allowlist[], denylist[]}`; entries are host globs (`*.example.com`, `api.*.com`), optional
  port and path prefix. Deny wins over allow. Empty allowlist = everything allowed.
- Create / edit / duplicate / delete / reorder presets. Each traffic page has a scope selector (none or one preset).
- "Add to scope" / "Exclude from scope" context actions everywhere a request appears.
- Intercept and workflows can be limited to the active scope.
- Filtering stored traffic by a scope (history, sitemap, delete-by-query, live subscriptions) uses the same query path as
  HTTPQL: the preset becomes `(allow₁ OR … ) AND NOT (deny₁ OR …)` (allow part omitted when the allowlist is empty).
  One rule = `req.host` glob (`*` any run, `?` one char; case-insensitive) AND `req.port` (if set) AND `req.path` prefix
  (if set; case-sensitive, like the in-memory matcher). SQL result and in-memory `Matcher.InScope` must agree.

### F-FILTER-1 Filter presets
- Preset = `{name, alias (lowercase-kebab), query}`. CRUD + import/export JSON.
- Referencable from any query as `preset:"Name"` or `preset:alias`. Cycles detected → error.
- Pages show quick-apply chips of presets next to the query bar.

---

## D. Proxy group

### F-INT-1 Intercept
- Global toggle in top bar: **Forwarding** (pass-through) ⇄ **Queuing** (hold). Separate toggles for
  Requests, Responses, WebSocket messages.
- Optional HTTPQL condition + scope preset deciding which traffic is held (others pass through).
- Queue tables (requests / responses / ws messages) with: id, host, method, path, time-held. Badge with queue count on sidebar item.
- Selected item opens editable raw editor. Buttons: **Forward**, **Drop**, Forward all, Drop all; undo edits.
- Actions: send to Replay / Automate, add to scope, run active workflow, copy as cURL.
- Edited requests are stored as a new record linked to the original (`edited_from`), both visible (F-HIST-4).
- When switching to Forwarding, all held items are released in FIFO order.
- AC: with queuing on, a browser request blocks until Forward; Drop returns `HTTP 502` (configurable) to client and marks record dropped.
- Contract (lead, 2026-11-19, M2):
  - Settings are per project (`intercept_settings`): `requests` (default on), `responses` (default off), `query`
    (HTTPQL, empty = everything), `scopeId` (none = everything), `dropStatusCode` (100–599, default 502). `queuing` is
    **not** persisted: every start and every project switch begins in Forwarding.
  - A request is held iff queuing ∧ `requests` ∧ it matches `query` and the scope; a response iff queuing ∧ `responses` ∧
    the exchange matches. The in-memory evaluator (docs/04) runs on the message after Match & Replace; `resp.*` clauses are
    false for a request. Changing the condition does not release items already held.
  - The queue is in memory, FIFO, not persisted. Item IDs are opaque. A held request already has its history record (a
    pending row without response, docs/03 §4), so it shows in HTTP History.
  - Only fully buffered messages are held: a body over the capture cap (64 MiB) passes through unheld (logged at info).
  - Queue limit (lead, 2026-11-19, T-031 review): at most 1 000 held items and 256 MiB of held message bytes (the sum of
    `raw` sizes) across both kinds. A message that would exceed either limit passes through unheld, logged at info, like
    the capture cap. Items already held are never released to make room.
  - A scope preset that no longer exists matches nothing: while `scopeId` points to a deleted preset nothing is held,
    and each failed check is logged at warn. A query evaluation error is treated the same way.
  - Forward takes optional replacement bytes, which must parse as an HTTP/1.x request (or response) — else `BAD_INPUT`
    with the parse error. `updateContentLength` (default true) sets `Content-Length` to the body size when the message has
    no `Transfer-Encoding` and has a body or a `Content-Length` header; otherwise the bytes go out exactly as given.
    Identical duplicate `Content-Length` headers become one, at the first one's position. The rule itself never rewrites a
    head that does not parse (including duplicate `Content-Length` headers with different values) and leaves those bytes
    exactly as given; that case matters for Replay (F-REP-2), which sends unparsed bytes. Intercept never reaches it:
    replacement bytes that fail to parse, conflicting lengths included, get `BAD_INPUT` and the item stays held
    (clarified 2026-11-19, T-031). Forward without replacement bytes sends the held message unchanged and ignores
    `updateContentLength`.
    A replacement response is parsed with the exchange's request method. The rule leaves `Content-Length` alone on a
    response that has no body by definition (to `HEAD`, or status 1xx, 204 or 304); it still counts as parsed bytes and
    goes out exactly as given (clarified 2026-11-19, T-031 review).
    Editing `Host` or the target never changes the destination (the original connection's host, port and TLS).
  - Replacement bytes are at most 16 MiB (else `BAD_INPUT`), so their base64 fits the 32 MiB API request cap (docs/03
    §6). A held message over 16 MiB can only be forwarded unchanged or dropped; the UI shows it read-only and says why.
  - Drop of a request: nothing goes upstream, the client gets `dropStatusCode` with a short HTML body of our own wording,
    the record gets `isDropped`. Drop of a response: the client gets the same answer; the record keeps the server's
    response and gets `isDropped`. Dropping discards edits (no edited record, F-HIST-4).
  - Turning queuing off, or turning `requests`/`responses` off, forwards the affected held items unchanged in FIFO order.
    A project switch does the same before the new project's settings load. If the client disconnects while its request
    is held, the item leaves the queue and the record stays without a response.
  - Project switch (clarified 2026-11-19, T-031): an item belongs to the project database that was selected when it was
    held. Only the selected project's items are visible: in `interceptQueue`, as IDs for Forward/Drop (otherwise
    `NOT_FOUND`), in the Forward all / Drop all counts and in `interceptQueueChanged`. A switch releases the old items
    without `REMOVED` events, because clients drop all project-scoped state on `projectChanged` `SELECTED` and refetch.
    It then emits `interceptSettingsChanged` with the new project's settings (Forwarding).
  - An item enters the queue only after its history record has been written (docs/03 §4, clarified 2026-11-19, T-031):
    the proxy waits for that write before it holds the message, which costs at most one flush interval. `ADDED`,
    `interceptQueue` and `InterceptItem.request` therefore always find the record.
  - Live updates: subscriptions `interceptQueueChanged` (item added / removed) and `interceptSettingsChanged`. The
    sidebar badge is the number of held items.
  - WebSocket messages are M4 (T-053). In M2 the Intercept actions are Forward, Drop, Forward all, Drop all, undo edits
    (client-side), send to Replay and copy as cURL. "Add to scope" follows the History context-menu item (F-HIST-3)
    and "run active workflow" lands with M5.

### F-HIST-1 HTTP History table
- Virtualized table of all requests in the project; live insertion at top or bottom (sort-aware) via subscription.
- Columns (show/hide, reorder, resize, persisted): ID, Host, Method, Path, Query, Status, Length (resp), Ext,
  Roundtrip ms, Created at, TLS, Port, Source, Color, Comment/Note, Edited flag. Sort by any column.
- Query bar (HTTPQL-compatible, autocomplete, syntax highlight, error underline) + scope selector + preset chips.
- Row colour (8 colours + none) and note per row; set manually or by workflows.
- Multi-select (shift/ctrl). Bulk actions: send to Replay, send to Automate (first), run active workflow, export selection, delete, colour.
- AC: 100k rows scroll smoothly; filter result for simple host query on 100k rows < 300 ms.

### F-HIST-2 Request/response viewer
- Split view below (or beside, toggle) the table: request pane, response pane, each with modes:
  **Raw**, **Pretty** (formatted JSON/XML/HTML/JS by content-type), **Hex**, **Headers/Params table**, **Preview**
  (sandboxed iframe for HTML/images), plus "open response in browser" (served from `ratatoskr.localhost/preview/{id}`).
- Preview contract (lead, 2026-11-19): the response is served at `GET /api/rest/preview/{requestId}` on the UI origin
  (`http://ratatoskr.localhost/api/rest/preview/{id}` when browsing through the proxy, F-NET-2). Body = the response
  body with chunked framing and content coding removed, first 4 MiB only. Status 200; `Content-Type` copied from the
  stored response (none → `application/octet-stream`); no other stored header is forwarded. Every answer carries
  `Content-Security-Policy: sandbox`, `X-Content-Type-Options: nosniff`, `Cache-Control: no-store` and
  `Referrer-Policy: no-referrer`. Unknown/non-numeric id, no stored response or no open project → 404. Same auth as other `/api/rest/` routes.
  Preview mode and "Open in browser" exist on the **response** pane only.
- Read-only in history; selection → context menu (send selection to Decoder-like convert workflow, copy, search).
- Auto-decoding of `Content-Encoding` (gzip, deflate, br, zstd) and chunked for display only.

### F-HIST-3 Context menu (rows & panes)
Send to Replay (Ctrl+R) · Send to Automate (Ctrl+M) · Run active workflow ▸ · Add host to scope / exclude ·
Create finding · Copy URL / as cURL / as raw / as fetch() / as Python requests · Set colour ▸ · Edit note ·
Open in browser · Filter by this host/path/status · Delete · Export.

### F-HIST-4 Modifications
- Records edited by Intercept or Match & Replace show an "edited" indicator; viewer can toggle **Original / Modified**.
- Contract (lead, 2026-11-19, M2): an exchange changed by Match & Replace or Intercept is stored as two records.
  The **original** record holds the request as the client sent it and the response as the server sent it. The **edited**
  record (`editedFrom` = original) holds the request as the server received it and the response as the client received
  it; when only the response changed, both carry the same request bytes. The edited record's source is `intercept` when
  the user changed anything in Intercept, otherwise the original's source. An original has at most one edited record
  (`HttpRequest.editedBy`). Both rows are listed in history and both show the edited indicator; the `EDITED` sort key
  stays `editedFrom`. Original / Modified switches both viewer panes between the two records at once. Replay and
  Automate store only what they sent and never create a pair.

### F-WS-1 WS History
- Top table: WebSocket **streams** (id, host, path, direction count, created, TLS, source).
- Bottom: messages of selected stream (direction ↑/↓, format text/binary, length, time) + message viewer (raw/pretty/hex).
- Query bar with **StreamQL** (see `04-query-language.md`). Send message to Replay (WS session).

### F-MR-1 Match & Replace
- Rules organised in **collections** (folders), ordered (drag to reorder), each `enabled` toggle.
- Rule = `{name, condition HTTPQL (optional), sources[] (proxy, replay, automate, workflow, plugin, intercept), section, operation, matcher, replacer}`.
- Sections & operations:
  - Request: First line (update raw), Method (update), Path (update raw), Query (update raw, update param, add param, remove param), Header (update raw, update value, add, remove), Body (update raw)
  - Response: First line (update raw), Status code (update), Header (update raw, update value, add, remove), Body (update raw)
  - WebSocket message: client→server, server→client (update raw)
- Matchers: **Full** (whole section / all items), **String** (exact), **Regex** (RE2 syntax — no look-around/backrefs; document this), **Name** (header/param name).
- Replacers: **Term** (string; regex capture groups `$1`, `${name}`), **Workflow** (convert workflow applied to the match).
- Body encoding awareness: decode `Content-Encoding` before matching, re-encode after, fix `Content-Length`.
- **Tester panel**: paste/pick a request/response → shows result diff live.
- Also applies in Replay when the session toggle "Apply Match & Replace" is on.
- Import/export JSON.
- AC: rule `Request Header / add / User-Agent: x` adds header on every proxied request; record marked edited; tester shows diff.
- Contract (lead, 2026-11-19, M2):
  - Order: collections in order, rules in order. Each enabled rule whose `sources` include the traffic's source and whose
    condition matches applies to the output of the previous rule. The condition (HTTPQL, empty = always) is evaluated in
    memory on the message as changed so far; for request sections no `resp.*` information exists (those clauses are false).
  - Sources: `proxy` (captured traffic, before Intercept holds it), `intercept` (a message forwarded from the Intercept
    editor, after the user's edit), `replay` (when the session toggle is on), `automate`, `workflow`, `plugin` (stored
    now, applied once those features land).
  - Matchers: `FULL` (the whole section, or every item), `STRING` (exact, case-sensitive, every occurrence), `REGEX`
    (Go RE2, every match; `$1`, `${name}` and `$$` expand in the term; an invalid pattern is `BAD_INPUT`), `NAME`.
  - **Update raw** works on the section text: the start line, the path, the raw query string, each header line
    (`Name: value`) on its own, or the body. **Update value** (header, query param): `NAME` selects items by name (headers
    case-insensitive, params case-sensitive) and the term replaces the whole value; `STRING`/`REGEX` replace inside every
    item's value; `FULL` replaces every item's value. **Add**: the term is `Name: value` (header) or `name=value` (param),
    appended last; the matcher is ignored. **Remove**: `NAME` by name, `STRING`/`REGEX` against the item text, `FULL`
    removes all. **Method / Status code update**: the matcher works on the token; a status result that is not three
    digits skips the rule (logged).
  - Body: one `Content-Encoding` of gzip, deflate, br or zstd is removed before matching and reapplied after; other or
    stacked codings skip body rules. A message whose body exceeds the capture cap (64 MiB) passes through with no rule
    applied (logged at info). A changed body keeps its framing: `Content-Length` is updated,
    a chunked body becomes one chunk plus the last chunk (trailers kept). Messages no rule changed keep their bytes.
  - The Workflow replacer arrives with M5 (T-066) and WebSocket sections with M4 (T-053); M2 has the Term replacer only.
  - Tester: one draft rule (saved or not) against pasted or picked bytes, request or response → output bytes, whether the
    condition matched and whether anything changed. The UI shows a line diff.
  - Export/import JSON `{"version":1,"collections":[{"name":…,"rules":[…]}]}`; import appends collections, all or nothing.
  - Limits: names 1–200 characters, ≤ 1000 rules per project, term ≤ 1 MiB. Rules reload on every change and on project switch.
  - API details (lead, 2026-11-19, T-099; schema `matchreplace.graphqls`): there is no default collection (the list may
    be empty); new collections and rules go last. `STRING`, `REGEX` and `NAME` need a non-empty value; `FULL` ignores it; `ADD` ignores the matcher
    (any kind and value).
    An `ADD` header term must be `Name: value` with a token name (`BAD_INPUT`); a param term is appended as written.
    Term expansion (`$1`, `${name}`, `$$`) only happens with `REGEX`. The tester takes `MatchReplaceTestRuleInput` (a rule without name, enabled and sources, so a tester request stays
    under the 32 MiB API cap whatever the draft name), rejects
    a section for the other message kind, a `Content-Length` that is not decimal or disagrees with another (exact decimal values, leading zeroes ignored), and a body
    whose length differs from `Content-Length` (a response with no body may declare any length, as for `HEAD`; body
    rules leave an empty response that declares a nonzero length unchanged, since it may answer `HEAD`, but fill one
    with `Content-Length: 0`), and a request with bytes after the head but neither `Content-Length` nor
    `Transfer-Encoding` (such a request has no body, so those bytes are not part of it; a header-only request stays
    valid and a rule may add a body) (`BAD_INPUT`); otherwise `raw` is framed exactly as the wire parser
    (`internal/rawhttp`) frames it, e.g. `Transfer-Encoding` is ignored on HTTP/1.0, and evaluates the condition on the given message only (clauses
    about the other message are false). Import rejects unknown fields; an unparsable condition there is `BAD_INPUT` naming
    the rule. Export rule fields are those of `MatchReplaceRuleInput`, enum values as strings, no IDs. A non-`ADD` `FULL`
    matcher is stored with an empty value.
  - Size budget (lead, 2026-11-19, T-099 review) so every request fits the 32 MiB API cap (docs/03 §6): a condition is
    at most 64 KiB; a project holds at most 1000 collections, and the UTF-8 sum of all its rules' conditions, matcher
    values and terms is at most 3 MiB (any create, update or import past either is `BAD_INPUT`). A valid export, JSON
    escaped again inside the import request, then stays under ~25 MiB. The tester takes at most 12 MiB of `raw`
    (`BAD_INPUT`); this fits only when `raw` is sent as unwrapped base64 (no line breaks, which `Bytes` would accept)
    in GraphQL variables, as the UI does; the UI does not send a larger pasted or picked message and says why. Its `output` is at most 16 MiB:
    a rule whose result would be larger is `BAD_INPUT` ("tester output must be at most 16 MiB"), detected while
    replacing, before the full result is allocated or encoded; no partial output. The UI shows that message.

---

## E. Testing group

### F-REP-1 Replay sessions & collections
- Left panel: collections tree (Default collection always exists) with sessions; drag & drop sessions between
  collections; reorder; rename; duplicate; delete; search sessions by name/host/path.
- Top: tab strip of open sessions (close, close others, close to the right, rename by double-click, tag colour).
- Session types: **HTTP**, **WebSocket**, **Pipeline**.
- New session: from scratch (URL dialog with connection info) or sent from any request (Ctrl+R).
- Contract (lead, 2026-11-19, M2):
  - A collection named "Default" is created with the project's first Replay use; it can be renamed, not deleted
    (`CONFLICT`), and receives new sessions when no collection is given. Deleting a collection deletes its sessions;
    deleting a session deletes its entries. Request records always stay in HTTP History.
  - Only `HTTP` sessions exist in M2; WS (M4) and Pipeline (M7) add enum values then.
  - A session holds: name, collection, position, tag colour (`RequestColor`, optional), connection (host, port, TLS, SNI
    override), options (follow redirects off, update Content-Length on, timeout 60 s within 1–300 s, Apply Match &
    Replace off) and the draft request bytes. New sessions are named `METHOD path` cut to 60 characters.
  - From a history request: its stored request bytes and connection (host, port, TLS) are copied; the SNI override stays
    unset, so the host is used as SNI (F-REP-2) and follows later host edits. `BAD_INPUT` when the stored *request*
    bytes were truncated; a truncated response does not block the copy. Records stored before request-only
    truncation was tracked (project migration 0003) count as truncated when their record-level flag is set. From scratch: host, port, TLS and request bytes are given; the URL dialog builds `GET <path> HTTP/1.1`
    with a `Host` header.
  - Duplicate copies draft, connection, options and colour (not entries) as `<name> (copy)`.
  - Session search, open tabs and the active tab are client-side; open tabs persist in `localStorage` per project.
  - (lead, 2026-11-19, T-034 escalation) "Duplicate" applies to sessions only; collections can be created, renamed and
    deleted but not duplicated in M2. Open-tab order is client-side and is persisted with the open tabs; tab drag reorder
    lands in T-111. The collections tree must not fetch draft bytes: it reads `ReplaySession.requestTarget` (T-110) for
    path search, and only the session editor loads `draft`.

### F-REP-2 HTTP session editor
- Connection bar: scheme/host/port/TLS/SNI override, HTTP/1.1 vs HTTP/2 (beta), "Apply Match & Replace" toggle, environment selector.
- Request editor (CodeMirror, raw HTTP, syntax highlight, `{{var}}` highlighting & hover value).
- **Send** (Ctrl+Enter), **Cancel** in-flight. Options: follow redirects (off by default), update Content-Length automatically (on), timeout.
- Response pane with same modes as F-HIST-2 + roundtrip time, size, status.
- Entry history per session: `<` `>` navigation and history dropdown; each send = one stored entry (request + response).
- AC: sending `GET / HTTP/1.1` with a deliberately wrong `Content-Length` and auto-update off sends that exact byte stream.
- Contract (lead, 2026-11-19, M2):
  - Send takes the editor's bytes, connection and options, saves them as the session's draft, and returns when the
    response is complete, the timeout passes or the send is cancelled. One send per session at a time (`CONFLICT`).
  - Steps: `{{var}}` interpolation (F-ENV-1) → Match & Replace (source `replay`, if enabled) → Content-Length update (if
    enabled; same rule as Intercept, F-INT-1) → write on a **fresh** connection (no pooling, closed after the exchange;
    TLS without certificate verification, SNI = override, else the host when it is a name; ALPN `http/1.1`). The timeout
    covers dial to the last response byte. Responses are read up to the capture cap (64 MiB), then cut (`isTruncated`).
  - Each send stores one request record (source `replay`, the bytes as sent) with its response, and one entry
    `{request, error, createdAt}`. Dial, TLS, timeout and cancel failures are entries with `error` and no response.
  - Follow redirects: up to 10 hops on 301/302/303/307/308 with a `Location`. 307/308 resend method and body; 303 (except
    HEAD) and 301/302 after POST switch to GET without body, dropping `Content-Length`, `Content-Type` and
    `Transfer-Encoding`. Target, `Host` and connection follow the resolved `Location`. Each hop is its own request record;
    the entry points to the last one.
  - History navigation (`‹ ›`, dropdown) shows an entry's request in the editor and its response; editing it makes it the draft.
  - The editor keeps loaded bytes exactly (CR/LF untouched) and Enter inserts CRLF. A message that is not valid UTF-8
    opens read-only with a notice. HTTP/1.x only in M2 (HTTP/2: T-081).
  - Line-ending rule (lead, 2026-11-19, T-104 escalation): CRLF, lone LF and lone CR are each one line break, so the caret
    never sits inside CRLF. A line break the user did not touch keeps its original bytes. A line break the edit creates
    (Enter, Shift+Enter, paste, line commands) is CRLF. Undo and redo restore the original bytes of line breaks.
  - The connection bar's environment selector changes the project's selected environment (F-ENV-1); it is not per session.

### F-REP-3 WebSocket session
- Connect using the upgrade request; message composer (text/binary), message log both directions, reconnect, close.

### F-REP-4 Pipeline session (race conditions)
- List of requests (add from existing sessions or history). Strategies: **Sequential** (ordered, on one or many connections)
  and **Last-byte synchronization** (all requests written except final byte, then final bytes flushed together).
- Requests table shows per-request status; Results tab shows all responses side by side.
- AC: integration test with local server measuring arrival spread < 5 ms for 20 requests with last-byte sync.

### F-REP-5 Replay workflows
- From the request editor context menu: run convert workflows on selection (replace selection with output).

### F-AUTO-1 Automate session
- Same left panel structure as Replay (sessions list). Editor with request template and **placeholders**:
  select text → "Add placeholder" (+), remove (–), clear all. Placeholder default value = original text.
- Strategies:
  - **Sequential** (Sniper): one placeholder at a time, others keep original.
  - **All** (Battering ram): same payload in all placeholders.
  - **Parallel** (Pitchfork): one payload set per placeholder, iterate in lockstep (sets must be same length or stop at shortest — show warning).
  - **Matrix** (Cluster bomb): cartesian product.
- Payload types per set: **Simple list** (paste/edit), **Hosted file** (F-FILE), **Numbers** (from, to, step, padding, hex/dec), **Null payloads** (repeat N times / forever).
- **Preprocessors** (ordered, per set): Prefix, Suffix, URL-encode (charset field, non-ASCII toggle), Workflow (convert workflow), Base64, Hash.
- **Settings**: workers (concurrency), delay between requests (ms, jitter), retries on error + backoff, timeout,
  update Content-Length, follow redirects, HTTP version, apply Match & Replace, rate-limit (req/s).
- **Run** creates a paired results tab; Pause / Resume / Cancel; progress bar; ETA.
- Results table: payload columns, status, length, roundtrip, error, plus **extractors** (custom columns via regex group or JSON path on response) — sortable and HTTPQL-filterable (`row`, `resp.*`).
- Results rows open request/response viewer; send to Replay; create finding.
- AC: Matrix with sets of 3 and 4 values produces exactly 12 requests with correct substitutions; Pause halts dispatch within 1 s.

### F-WF-1 Workflows
- Page with three tabs: **Passive**, **Active**, **Convert**. List of workflows with enable toggle (passive), name, description,
  "global shortcut" (active/convert can be bound to a shortcut), import/export JSON, duplicate, delete.
- Visual editor (node graph): palette of nodes on the left, canvas centre, node inspector on the right, bottom run/test panel.
- Node inputs are **constant** values or **references** `$node_alias.output_name`; string **interpolation** `{{ $node.out }}` inside text inputs.
- Node aliases: `[a-z0-9_-]+`, unique per workflow.
- Execution: DAG from the start node; If/Else branches; each node gets typed inputs, returns typed outputs; errors stop the run and log.
- Node catalogue (MUST):

| Node | Passive | Active | Convert |
|---|:-:|:-:|:-:|
| On Intercept Request / On Intercept Response (start) | ✓ | | |
| Active Start / Active End | | ✓ | |
| Convert Start / Convert End | | | ✓ |
| Passive End | ✓ | | |
| If/Else (boolean input) | ✓ | ✓ | ✓ |
| If/Else JavaScript | ✓ | ✓ | ✓ |
| JavaScript | ✓ | ✓ | ✓ |
| Shell (choose shell, stdin = input, env vars for req/resp) | ✓ | ✓ | ✓ |
| In Scope (scope preset) | ✓ | ✓ | |
| Matches HTTPQL | ✓ | ✓ | |
| Set Color (row colour) | ✓ | ✓ | |
| Check Finding / Create Finding | ✓ | ✓ | |
| Logging | ✓ | ✓ | |
| Send Request (send modified request, output response) | | ✓ | |
| Base64 / Hex / HTML / URL encode & decode | | | ✓ |
| Join, Trim, JSON Minify, JSON Prettify, JWT Decode | | | ✓ |
| Match & Replace (regex/string on input) | | | ✓ |
| MD5 / SHA1 / SHA2 (256/384/512) hash | | | ✓ |

- JavaScript nodes: TypeScript/JS source, `export async function run(input, sdk)`; SDK gives request/response objects,
  `sdk.console`, `sdk.findings`, `sdk.requests.send`, `sdk.env`, `sdk.scope`, encoders. Typed definitions shipped to the editor.
- Integrations: Active workflows appear in request context menus; Convert workflows appear in editor selection menus,
  as Match & Replace replacers, and as Automate preprocessors. Passive workflows run on proxied traffic (bounded worker pool, never block forwarding beyond a configurable timeout).
- Test panel: run against a chosen request (passive/active) or input text (convert) and show per-node outputs + logs.
- Community/default workflows bundled: Base64 encode/decode, URL encode/decode, JWT decode, MD5, "colour in-scope GET".
- AC: convert workflow `Convert Start → Base64 Encode → Convert End` on input `abc` returns `YWJj`; passive workflow `In Scope → Matches HTTPQL resp.code.eq:500 → Set Color red` colours matching rows only.

### F-AI-1 Assistant (bring-your-own-key)
- Settings → AI: providers **Anthropic, OpenAI, Google, OpenRouter, OpenAI-compatible base URL (Ollama, LiteLLM, vLLM)**; keys stored encrypted at rest (instance key file); model list fetched or typed.
- Assistant page: chat sessions list (left), chat (centre), attached context chips (requests, selections). Streaming responses, Markdown + code blocks with "copy" and "send to Replay".
- Quick actions from context menu: **Explain request**, **Suggest attack vectors**, **Generate CSRF PoC** (HTML), **Write payloads for selection**.
- Privacy notice before first send; per-session toggle "redact cookies/Authorization".
- Tool-use (phase 2): the model may call safe tools (search history, read request, create replay session, add finding) — every tool call shown and user-approvable.
- AC: with a mock provider, "Explain request" sends the raw request (redacted when toggle on) and streams the answer.

---

## F. Logging group

### F-SEARCH-1 Search
- Like HTTP History but spans **all sources** (proxy, replay, automate, workflow, plugin, intercept) with a `source:` filter
  and source-inclusion toggles in advanced options. Same table, viewer, context menu.

### F-FIND-1 Findings
- Left: reporters list (All, Manual, each workflow/plugin name) with counts. Centre: findings table (title, host, path, reporter, created, request id).
  Right/bottom: finding detail (Markdown description editable, linked request/response viewer).
- Create manually from any request. Dedupe key (`reporter + dedupe_key`) prevents duplicates. Delete, export (JSON/CSV/Markdown).
- HTTPQL filtering on linked request + finding fields (`finding.title`, `finding.reporter` — our extension).

### F-EXP-1 Exports
- From History/Search/Automate results: export **all** or **current rows** (current filter + scope) as **JSON** or **CSV**
  (CSV flattens response columns with `response_` prefix). Large exports run as background tasks.
- Exports page lists exports: name, format, rows, size, status, created; download, rename, delete.

---

## G. Workspace group (pages)

### F-ENV-1 Environment
- Global environment (instance-wide) + per-project environments. One "selected" environment per project.
- Variables `{name, value, kind: plain|secret}`; secrets masked in UI and never sent to AI or exports.
- Interpolation `{{name}}` in Replay, Automate, workflows' Send Request; selected env overrides Global.
- Workflows/plugins can set variables (e.g. refreshed JWT).
- Contract (lead, 2026-11-19, M2):
  - Global lives in instance.db (`env_global`), always exists, cannot be renamed or deleted. Project environments live
    in project.db; at most one is selected per project (none = Global only).
  - Environment names 1–200 characters, unique per project ("Global" is reserved). Variable names
    `[A-Za-z_][A-Za-z0-9_.-]*`, ≤ 100 characters, unique per environment (case-sensitive); values ≤ 1 MiB.
  - Secret values are sealed at rest with `secret.key` (docs/03 §8). The API returns them to authenticated clients; the
    UI masks them behind a reveal toggle. Exports (M3) and the Assistant (M6) must leave them out.
  - Interpolation: `{{name}}`, spaces inside the braces allowed. Lookup: selected environment, then Global. Unknown
    names stay verbatim (editors mark them). One pass: substituted values are not interpolated again. No escape syntax.
  - No subscription: pages refetch after their own mutations.

### F-FILE-1 Files (hosted files)
- Upload (drag & drop, multi), list (name, size, created), rename, delete, preview first N lines. Used by Automate and plugins.

### F-PLUG-1 Plugins
- Plugin package = zip with `manifest.json` + `backend/*.js` and/or `frontend/*.js` + `frontend/*.css`.
- Install from file, from URL, or from a **community index URL** (JSON list: name, author, version, downloads, description, repo).
- Installed tab: enable/disable, update, uninstall, view logs, settings. Store tab: filter Official/Community, sort, multi-select install/update.
- Backend plugins run in a sandboxed JS runtime with a typed SDK (requests, findings, scope, env, files, storage, HTTP send, events: onInterceptRequest/Response, onProjectChange).
- Frontend plugins get a typed SDK: register pages, sidebar items, commands, command-palette entries, context-menu items, footer slots, editor extensions, call own backend functions, storage, toasts.
- Plugin API is ours (not Caido-compatible); document it in `docs/plugins-sdk.md` when M6 starts.

---

## H. Cross-cutting UI (`F-UI`)

- **F-UI-1 Command palette** (Ctrl/⌘+K): go to page/plugin, run command, run workflow, switch project, toggle intercept, search sessions.
- **F-UI-2 Shortcuts**: all commands rebindable in Settings → Shortcuts; one shortcut may map to multiple contexts; conflicts warned.
- **F-UI-3 Resizable panes** everywhere; sizes persisted per page.
- **F-UI-4 Sidebar customization**: reorder pages & plugin pages, pin, hide items/categories, create categories, reset to defaults; collapse sidebar.
- **F-UI-5 Appearance**: dark/light/system theme, UI font size, editor font size, density, custom CSS (Settings tab), custom JS (Settings tab, runs in UI with SDK).
- **F-UI-6 Editors**: CodeMirror 6, HTTP syntax mode, search (Ctrl+F), fold JSON, wrap toggle, prettify, "show non-printable", large-message virtualization (> 5 MB → read-only chunked view).
- **F-UI-7 Toasts & notifications**, **F-UI-8 Onboarding checklist** (CA install, browser proxy, first project).
- **F-UI-9 Background tasks** indicator (exports, backups, automate runs, plugin installs) in the top bar.
- **F-UI-10 Logs viewer** (Settings → Logs) streaming backend logs; debug mode toggle.

## I. CLI (`F-CLI`)
`ratatoskr [--listen addr]… [--data-dir path] [--ui-listen addr] [--no-open] [--auth-token t] [--debug] [--allow-guests]`
`ratatoskr ca export|regenerate` · `ratatoskr project list|backup|restore` · `ratatoskr version`.
Env vars mirror flags (`RATATOSKR_LISTEN`, …). Config file `config.toml` in data dir.

## J. MCP server (`F-MCP`, M6)
Optional MCP endpoint (`/mcp`, streamable HTTP, token-protected) exposing tools: search history (HTTPQL), get request,
send request, create replay session, list/create findings, list scopes, set environment variable. Lets external AI agents drive Ratatoskr.
