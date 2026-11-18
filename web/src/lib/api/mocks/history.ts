// Fake HTTP History backend for UI work before the real one lands (T-013).
// Resolvers follow internal/api/graphql/schema/history.graphqls; values are in wire
// format (Bytes as base64, Time as RFC 3339, Int64 as decimal strings).
// Hosts use reserved names only (example.*, *.test): never real targets.

import { GraphQLError } from 'graphql';
import { nodeFilter } from './sitemap';

const COLORS = ['RED', 'ORANGE', 'YELLOW', 'GREEN', 'CYAN', 'BLUE', 'PURPLE', 'PINK'] as const;
const SOURCES = [
  'PROXY',
  'PROXY',
  'PROXY',
  'PROXY',
  'INTERCEPT',
  'REPLAY',
  'AUTOMATE',
  'WORKFLOW',
] as const;
const HOSTS = [
  'example.com',
  'api.example.com',
  'cdn.example.net',
  'shop.example.org',
  'auth.example.test',
];
const METHODS = ['GET', 'GET', 'GET', 'GET', 'POST', 'POST', 'PUT', 'DELETE', 'OPTIONS'];
const PATHS = [
  '/',
  '/index.html',
  '/static/app.js',
  '/static/site.css',
  '/img/logo.png',
  '/api/v1/users',
  '/api/v1/users/42',
  '/api/v1/orders',
  '/login',
  '/graphql',
];
const CODES = [200, 200, 200, 200, 204, 301, 304, 400, 401, 403, 404, 500];

type Args = Record<string, unknown>;

interface BodyArgs {
  offset: string | number;
  limit: number;
  decode: boolean;
}

export interface MockResponse {
  code: number;
  length: string;
  roundtripMs: number;
  createdAt: string;
  head: string;
  bodyLength: string;
  body: (args: BodyArgs) => string;
}

export interface MockRequest {
  id: string;
  source: string;
  host: string;
  port: number;
  tls: boolean;
  method: string;
  path: string;
  query: string;
  ext: string;
  length: string;
  createdAt: string;
  color: string | null;
  note: string | null;
  editedFrom: MockRequest | null;
  isDropped: boolean;
  isTruncated: boolean;
  head: string;
  bodyLength: string;
  body: (args: BodyArgs) => string;
  response: MockResponse | null;
}

/** Deterministic PRNG (mulberry32) so mock data is the same on every load. */
function rng(seed: number): () => number {
  return () => {
    seed = (seed + 0x6d2b79f5) | 0;
    let t = Math.imul(seed ^ (seed >>> 15), 1 | seed);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

const enc = new TextEncoder();

function b64(bytes: Uint8Array): string {
  let s = '';
  for (const b of bytes) s += String.fromCharCode(b);
  return btoa(s);
}

function badInput(message: string): GraphQLError {
  return new GraphQLError(message, { extensions: { code: 'BAD_INPUT' } });
}

function slicer(body: Uint8Array): (args: BodyArgs) => string {
  return ({ offset, limit }) => {
    const start = Number(offset);
    if (start < 0) throw badInput('offset must be >= 0');
    if (limit < 1 || limit > 4194304) throw badInput('limit must be 1..4194304');
    return b64(body.subarray(start, start + limit));
  };
}

function contentType(path: string): string {
  if (path.endsWith('.js')) return 'application/javascript';
  if (path.endsWith('.css')) return 'text/css';
  if (path.endsWith('.png')) return 'image/png';
  if (path.startsWith('/api') || path === '/graphql') return 'application/json';
  return 'text/html; charset=utf-8';
}

function responseBody(path: string, code: number, i: number): Uint8Array {
  if (code === 204 || code === 304) return new Uint8Array();
  if (path.endsWith('.png')) return Uint8Array.from({ length: 64 }, (_, k) => (k * 37 + i) & 0xff);
  const ct = contentType(path);
  if (ct === 'application/json') {
    return enc.encode(
      JSON.stringify({ id: i, name: `user${i}`, items: [1, 2, 3], ok: code < 400 }, null, 2),
    );
  }
  if (ct === 'text/css') return enc.encode(`body { margin: 0; }\n.row-${i} { color: #333; }\n`);
  if (ct === 'application/javascript') return enc.encode(`console.log("bundle ${i}");\n`);
  return enc.encode(
    `<!doctype html>\n<html><head><title>Page ${i}</title></head><body><h1>Hello ${i}</h1></body></html>\n`,
  );
}

/** Builds one fake record; `i` is the 1-based row ID. */
export function makeRequest(i: number, base = Date.UTC(2026, 10, 18, 9, 0, 0)): MockRequest {
  const r = rng(i);
  const pick = <T>(xs: readonly T[]): T => xs[Math.floor(r() * xs.length)]!;
  const host = pick(HOSTS);
  const tls = r() < 0.85;
  const method = pick(METHODS);
  const path = pick(PATHS);
  const query =
    path.startsWith('/api') && r() < 0.5 ? `page=${1 + Math.floor(r() * 9)}&sort=name` : '';
  const dot = path.lastIndexOf('.');
  const ext = dot > path.lastIndexOf('/') ? path.slice(dot) : '';
  const reqBody =
    method === 'POST' || method === 'PUT'
      ? enc.encode(JSON.stringify({ name: `item ${i}` }))
      : new Uint8Array();
  const target = query ? `${path}?${query}` : path;
  const reqHead = enc.encode(
    `${method} ${target} HTTP/1.1\r\nHost: ${host}\r\nUser-Agent: Mozilla/5.0 (mock)\r\nAccept: */*\r\n` +
      (reqBody.length
        ? `Content-Type: application/json\r\nContent-Length: ${reqBody.length}\r\n`
        : '') +
      '\r\n',
  );
  const createdMs = base + i * 1500 + Math.floor(r() * 1000);
  const isDropped = r() < 0.02;

  let response: MockResponse | null = null;
  if (!isDropped && r() > 0.03) {
    const code = pick(CODES);
    const body = responseBody(path, code, i);
    const roundtripMs = 5 + Math.floor(r() * 800);
    const head = enc.encode(
      `HTTP/1.1 ${code} ${code < 300 ? 'OK' : code < 400 ? 'Redirect' : code < 500 ? 'Client Error' : 'Server Error'}\r\n` +
        `Content-Type: ${contentType(path)}\r\nContent-Length: ${body.length}\r\nServer: mock\r\n\r\n`,
    );
    response = {
      code,
      length: String(head.length + body.length),
      roundtripMs,
      createdAt: new Date(createdMs + roundtripMs).toISOString(),
      head: b64(head),
      bodyLength: String(body.length),
      body: slicer(body),
    };
  }

  return {
    id: String(i),
    source: pick(SOURCES),
    host,
    port: tls ? 443 : 80,
    tls,
    method,
    path,
    query,
    ext,
    length: String(reqHead.length + reqBody.length),
    createdAt: new Date(createdMs).toISOString(),
    color: r() < 0.1 ? pick(COLORS) : null,
    note: r() < 0.05 ? `note on #${i}` : null,
    editedFrom: null,
    isDropped,
    isTruncated: false,
    head: b64(reqHead),
    bodyLength: String(reqBody.length),
    body: slicer(reqBody),
    response,
  };
}

type Listener = (event: Record<string, unknown>) => void;

// Sort keys; requests without a response, colour or note sort lowest (schema docs).
const SORT_KEYS: Record<string, (r: MockRequest) => string | number> = {
  ID: (r) => Number(r.id),
  HOST: (r) => r.host,
  METHOD: (r) => r.method,
  PATH: (r) => r.path,
  QUERY: (r) => r.query,
  STATUS: (r) => r.response?.code ?? -1,
  RESPONSE_LENGTH: (r) => Number(r.response?.length ?? -1),
  EXT: (r) => r.ext,
  ROUNDTRIP: (r) => r.response?.roundtripMs ?? -1,
  CREATED_AT: (r) => r.createdAt,
  TLS: (r) => Number(r.tls),
  PORT: (r) => r.port,
  SOURCE: (r) => r.source,
  COLOR: (r) => (r.color ? COLORS.indexOf(r.color as (typeof COLORS)[number]) : -1),
  NOTE: (r) => r.note ?? '',
  EDITED: (r) => Number(r.editedFrom !== null),
};

const COMPLETIONS: { label: string; kind: string; detail: string; isExtension?: boolean }[] = [
  ...['req', 'resp', 'row'].map((label) => ({ label, kind: 'NAMESPACE', detail: 'namespace' })),
  ...['method', 'host', 'path', 'query', 'ext', 'header', 'body', 'raw'].map((label) => ({
    label,
    kind: 'FIELD',
    detail: 'string',
  })),
  ...['port', 'len', 'code', 'roundtrip', 'id'].map((label) => ({
    label,
    kind: 'FIELD',
    detail: 'int',
  })),
  ...['color', 'note'].map((label) => ({
    label,
    kind: 'FIELD',
    detail: 'string',
    isExtension: true,
  })),
  ...[
    'eq',
    'ne',
    'gt',
    'gte',
    'lt',
    'lte',
    'cont',
    'ncont',
    'like',
    'nlike',
    'regex',
    'nregex',
  ].map((label) => ({
    label,
    kind: 'OPERATOR',
    detail: 'operator',
  })),
  ...HOSTS.map((h) => ({ label: `"${h}"`, kind: 'VALUE', detail: 'req.host' })),
];

export interface MockHistoryOptions {
  /** Number of seeded records (default 500). */
  count?: number;
  /** Interval of live `requestAdded` events in ms; 0 disables them (default 2000). */
  liveIntervalMs?: number;
}

/**
 * A fake history backend: root resolvers for graphql-js. Each instance has its own
 * records, so tests do not share state.
 */
export function createHistoryMock({ count = 500, liveIntervalMs = 2000 }: MockHistoryOptions = {}) {
  const rows: MockRequest[] = Array.from({ length: count }, (_, k) => makeRequest(k + 1));
  // Every 25th row is an edited copy of the row before it (F-HIST-4).
  for (let k = 24; k < rows.length; k += 25) rows[k]!.editedFrom = rows[k - 1]!;
  const byId = new Map(rows.map((r) => [r.id, r]));
  let nextId = count + 1;
  const listeners = new Set<Listener>();
  let liveCount = 0;
  let timer: ReturnType<typeof setInterval> | undefined;
  const emit = (event: Record<string, unknown>) => listeners.forEach((l) => l(event));

  // shortcut: matches the quoted literals (or the whole text) as a case-insensitive substring of
  // method, host, path and query; replace with the real HTTPQL engine once T-0xx lands it.
  const matcher = (query: unknown): ((r: MockRequest) => boolean) => {
    const text = typeof query === 'string' ? query.trim() : '';
    if (!text) return () => true;
    const node = nodeFilter(text);
    if (node) {
      const rest = matcher(node.rest);
      return (r) => node.match(r) && rest(r);
    }
    const terms = [...text.matchAll(/"([^"]*)"/g)].map((m) => m[1]!.toLowerCase());
    if (!terms.length) terms.push(text.toLowerCase());
    return (r) => {
      const hay = `${r.method} ${r.host}${r.path}?${r.query}`.toLowerCase();
      return terms.every((t) => hay.includes(t));
    };
  };

  // A hand-written iterator, not an async generator: a generator parked on its wait
  // promise cannot run return() until the next event, so unsubscribing would leak.
  function events<T>(
    pick: (e: Record<string, unknown>) => T | undefined,
    live = false,
  ): AsyncIterableIterator<T> {
    const queue: T[] = [];
    let wake = () => {};
    let done = false;
    const listener: Listener = (e) => {
      const v = pick(e);
      if (v === undefined) return;
      queue.push(v);
      wake();
    };
    listeners.add(listener);
    if (live && liveIntervalMs > 0 && liveCount++ === 0) {
      // One shared timer for all live subscribers, so each new row is emitted once.
      timer = setInterval(() => {
        const r = makeRequest(nextId++, Date.now());
        rows.push(r);
        byId.set(r.id, r);
        emit({ added: r });
      }, liveIntervalMs);
    }
    return {
      [Symbol.asyncIterator]() {
        return this;
      },
      async next() {
        while (!queue.length && !done) await new Promise<void>((resolve) => (wake = resolve));
        return done ? { done: true, value: undefined } : { done: false, value: queue.shift()! };
      },
      async return() {
        if (!done && live && liveIntervalMs > 0 && --liveCount === 0) clearInterval(timer);
        done = true;
        listeners.delete(listener);
        wake();
        return { done: true, value: undefined };
      },
    };
  }

  const find = (id: unknown): MockRequest => {
    const r = byId.get(String(id));
    if (!r)
      throw new GraphQLError(`request ${String(id)} not found`, {
        extensions: { code: 'NOT_FOUND' },
      });
    return r;
  };

  return {
    rows,
    matcher,
    root: {
      requests(args: Args) {
        const { first, after, last, before, query, sort } = args as {
          first?: number;
          after?: string;
          last?: number;
          before?: string;
          query?: string;
          sort?: { field: string; direction: string };
        };
        const key = SORT_KEYS[sort?.field ?? 'ID'] ?? SORT_KEYS.ID!;
        const dir = sort?.direction === 'DESC' ? -1 : 1;
        const list = rows.filter(matcher(query)).sort((a, b) => {
          const x = key(a);
          const y = key(b);
          return (x < y ? -1 : x > y ? 1 : Number(a.id) - Number(b.id)) * dir;
        });
        if (first != null && last != null) throw badInput('give only one of first and last');
        for (const n of [first, last])
          if (n != null && (n < 1 || n > 1000)) throw badInput('first/last must be 1..1000');
        // Cursors are row IDs, so they survive inserts; opaque to clients either way.
        const at = (id: string) => list.findIndex((r) => r.id === id);
        let start = after ? at(after) + 1 : 0;
        let end = before ? Math.max(at(before), 0) : list.length;
        if (last != null) start = Math.max(start, end - last);
        else end = Math.min(end, start + (first ?? 100));
        const edges = list.slice(start, end).map((node) => ({ cursor: node.id, node }));
        return {
          edges,
          totalCount: list.length,
          pageInfo: {
            hasPreviousPage: start > 0,
            hasNextPage: end < list.length,
            startCursor: edges[0]?.cursor ?? null,
            endCursor: edges.at(-1)?.cursor ?? null,
          },
        };
      },
      request: ({ id }: Args) => byId.get(String(id)) ?? null,
      queryCompletions({ text, cursor }: Args) {
        // shortcut: assumes ASCII text, where byte offsets equal string indices.
        const s = String(text);
        const to = Number(cursor);
        let from = to;
        while (from > 0 && !/[\s().:]/.test(s[from - 1]!)) from--;
        const prefix = s.slice(from, to).toLowerCase();
        const items = COMPLETIONS.filter((c) => c.label.toLowerCase().startsWith(prefix)).map(
          (c) => ({
            label: c.label,
            insertText: c.label,
            kind: c.kind,
            detail: c.detail,
            isExtension: c.isExtension ?? false,
          }),
        );
        return { from, to, items: items.slice(0, 100) };
      },
      setRequestColor({ ids, color }: Args) {
        const list = (ids as string[]).map(find);
        for (const r of list) {
          r.color = (color as string | null) ?? null;
          emit({ updated: r });
        }
        return list;
      },
      setRequestNote({ id, note }: Args) {
        const r = find(id);
        r.note = typeof note === 'string' && note.trim() ? note.trim() : null;
        emit({ updated: r });
        return r;
      },
      deleteRequests({ ids, query }: Args) {
        if ((ids == null) === (query == null))
          throw new GraphQLError('give exactly one of ids and query', {
            extensions: { code: 'BAD_INPUT' },
          });
        const doomed = ids ? new Set((ids as string[]).filter((id) => byId.has(id))) : null;
        const match = matcher(query);
        const gone = rows.filter((r) => (doomed ? doomed.has(r.id) : match(r)));
        for (const r of gone) byId.delete(r.id);
        for (const r of rows) if (r.editedFrom && !byId.has(r.editedFrom.id)) r.editedFrom = null;
        rows.splice(0, rows.length, ...rows.filter((r) => byId.has(r.id)));
        for (let k = 0; k < gone.length; k += 1000) {
          emit({ deleted: { ids: gone.slice(k, k + 1000).map((r) => r.id) } });
        }
        return String(gone.length);
      },
      requestAdded: ({ query }: Args) => {
        const match = matcher(query);
        return events((e) => {
          const r = e.added as MockRequest | undefined;
          return r && match(r) ? { requestAdded: r } : undefined;
        }, true);
      },
      requestUpdated: ({ query }: Args) => {
        const match = matcher(query);
        return events((e) => {
          const r = e.updated as MockRequest | undefined;
          return r ? { requestUpdated: { request: r, matches: match(r) } } : undefined;
        });
      },
      requestsDeleted: () =>
        events((e) => (e.deleted ? { requestsDeleted: e.deleted } : undefined)),
    },
  };
}
