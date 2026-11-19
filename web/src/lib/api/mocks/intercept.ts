// Fake Intercept backend for UI work before the real one lands (T-030).
// Resolvers follow internal/api/graphql/schema/intercept.graphqls, including its error
// codes. Held records come from the history mock, so they use reserved hosts only.

import { GraphQLError } from 'graphql';
import { makeRequest, type MockRequest, type MockResponse } from './history';
import type { MockScopePreset } from './presets';

type Args = Record<string, unknown>;
type Kind = 'REQUEST' | 'RESPONSE';

export interface MockInterceptItem {
  id: string;
  kind: Kind;
  request: MockRequest;
  raw: string;
  heldAt: string;
}

export interface MockInterceptSettings {
  queuing: boolean;
  requests: boolean;
  responses: boolean;
  query: string;
  scopeId: string | null;
  dropStatusCode: number;
}

export interface MockInterceptOptions {
  /** Interval in ms at which a new request is held while queuing; 0 (default) disables it. */
  liveIntervalMs?: number;
}

/** What the intercept mock needs from the history mock. */
export interface InterceptHistory {
  addRecord: (build: (id: number) => MockRequest) => MockRequest;
  updateRecord: (r: MockRequest) => void;
  matcher: (query: unknown) => (r: MockRequest) => boolean;
}

// shortcut: host globs with `*` only, exact port, path prefix; the real backend uses
// internal/scope. Enough to show held vs passed-through traffic in the UI.
function ruleMatches(rule: string, r: MockRequest): boolean {
  const m = /^\s*([^:/\s]+)(?::(\d+))?(\/\S*)?\s*$/.exec(rule);
  if (!m) return false;
  const host = m[1]!.replace(/[.+?^$()|[\]\\{}]/g, '\\$&').replace(/\*/g, '.*');
  const glob = new RegExp(`^${host}$`, 'i');
  return glob.test(r.host) && (!m[2] || Number(m[2]) === r.port) && r.path.startsWith(m[3] ?? '');
}

/** Scope rule from F-SCOPE-1: deny wins over allow, an empty allowlist allows everything. */
export function inScope(p: MockScopePreset, r: MockRequest): boolean {
  if (p.deny.some((d) => ruleMatches(d, r))) return false;
  return !p.allow.length || p.allow.some((a) => ruleMatches(a, r));
}

const fail = (code: string, message: string, span?: { start: number; end: number }) =>
  new GraphQLError(message, { extensions: span ? { code, span } : { code } });

const FULL = { offset: 0, limit: 4194304, decode: false };

/** Whole message as base64: head and body are both base64 of latin1-safe bytes. */
const rawOf = (m: MockRequest | MockResponse) => btoa(atob(m.head) + atob(m.body(FULL)));

// shortcut: checks the start line and the end of the headers only; the real backend runs the
// rawhttp parser. Good enough to exercise the BAD_INPUT path in the UI.
function checkRaw(kind: Kind, raw: string): void {
  let text: string;
  try {
    text = atob(raw);
  } catch {
    throw fail('BAD_INPUT', 'raw is not valid base64');
  }
  const line =
    kind === 'REQUEST' ? /^[A-Za-z]+ \S+ HTTP\/1\.[01]\r\n/ : /^HTTP\/1\.[01] \d{3}[ \r]/;
  if (!line.test(text)) throw fail('BAD_INPUT', `malformed ${kind.toLowerCase()} line`);
  if (!text.includes('\r\n\r\n')) throw fail('BAD_INPUT', 'missing end of headers');
}

// shortcut: only flags an unterminated string literal; the real backend parses HTTPQL.
function checkQuery(q: string): void {
  const quotes = [...q.matchAll(/"/g)];
  if (quotes.length % 2) {
    const start = quotes.at(-1)!.index;
    throw fail('BAD_QUERY', 'unterminated string', { start, end: q.length });
  }
}

/**
 * A fake intercept backend: root resolvers for graphql-js. Starts queuing (requests and
 * responses), with two requests and one response held from the first intercept operation on.
 * `scopeOf` looks up a scope preset by ID (null when unknown).
 */
export function createInterceptMock(
  history: InterceptHistory,
  scopeOf: (id: string) => MockScopePreset | null,
  { liveIntervalMs = 0 }: MockInterceptOptions = {},
) {
  const settings: MockInterceptSettings = {
    queuing: true,
    requests: true,
    responses: true,
    query: '',
    scopeId: null,
    dropStatusCode: 502,
  };
  const queue: MockInterceptItem[] = [];
  // Responses a held request will get once forwarded.
  const pending = new Map<string, MockResponse | null>();
  const listeners = new Set<(e: Record<string, unknown>) => void>();
  const emit = (e: Record<string, unknown>) => listeners.forEach((l) => l(e));
  let nextItem = 1;

  // Turns a record into a held one: a held request has no response yet, a held response has one.
  const prepare = (kind: Kind, r: MockRequest): MockRequest => {
    r.source = 'PROXY';
    r.isDropped = false;
    if (kind === 'REQUEST') {
      pending.set(r.id, r.response);
      r.response = null;
    } else {
      // Borrow a response from another seed when this one has none.
      for (let k = 1; !r.response; k++) r.response = makeRequest(Number(r.id) + k * 1000).response;
    }
    return r;
  };

  /** Puts an already stored, prepared record in the queue. */
  const hold = (kind: Kind, request: MockRequest) => {
    const item: MockInterceptItem = {
      id: `ii${nextItem++}`,
      kind,
      request,
      raw: rawOf(kind === 'REQUEST' ? request : request.response!),
      heldAt: new Date().toISOString(),
    };
    queue.push(item);
    emit({ interceptQueueChanged: { kind: 'ADDED', item } });
    return item;
  };

  const take = (id: unknown): MockInterceptItem => {
    const k = queue.findIndex((i) => i.id === String(id));
    if (k < 0) throw fail('NOT_FOUND', `intercept item ${String(id)} not found`);
    const [item] = queue.splice(k, 1);
    emit({ interceptQueueChanged: { kind: 'REMOVED', item } });
    return item!;
  };

  // shortcut: a forward with edits does not store an edited record (F-HIST-4); the history
  // mock already seeds edited pairs for the Original / Modified viewer.
  const forward = (item: MockInterceptItem) => {
    if (item.kind === 'REQUEST') {
      item.request.response = pending.get(item.request.id) ?? null;
      pending.delete(item.request.id);
      history.updateRecord(item.request);
    }
  };

  const drop = (item: MockInterceptItem) => {
    pending.delete(item.request.id);
    item.request.isDropped = true;
    history.updateRecord(item.request);
  };

  const ofKind = (kind: unknown) => queue.filter((i) => kind == null || i.kind === kind);

  // Removes every matching item in FIFO order and applies `act` to each.
  const releaseAll = (kind: unknown, act: (i: MockInterceptItem) => void): number => {
    const items = ofKind(kind);
    for (const i of items) act(take(i.id));
    return items.length;
  };

  // Seeded on the first intercept operation, so clients that never use Intercept see the
  // history exactly as configured.
  let seeded = false;
  const seed = () => {
    if (seeded) return;
    seeded = true;
    for (const kind of ['REQUEST', 'REQUEST', 'RESPONSE'] as const)
      hold(
        kind,
        history.addRecord((id) => prepare(kind, makeRequest(id, Date.now()))),
      );
  };

  // A new proxied request: held iff queuing, requests on, and it matches the query and scope
  // (F-INT-1); otherwise it passes through and only lands in history.
  const arrive = () => {
    let held = false;
    const request = history.addRecord((id) => {
      const r = makeRequest(id, Date.now());
      const scope = settings.scopeId ? scopeOf(settings.scopeId) : null;
      held =
        settings.queuing &&
        settings.requests &&
        history.matcher(settings.query)(r) &&
        (!scope || inScope(scope, r));
      r.source = 'PROXY';
      return held ? prepare('REQUEST', r) : r;
    });
    if (held) hold('REQUEST', request);
  };

  let timer: ReturnType<typeof setInterval> | undefined;
  let subscribers = 0;

  // Same hand-written iterator as the history mock, so unsubscribing never leaks.
  function events(key: string): AsyncIterableIterator<Record<string, unknown>> {
    const buf: Record<string, unknown>[] = [];
    let wake = () => {};
    let done = false;
    const listener = (e: Record<string, unknown>) => {
      if (!(key in e)) return;
      buf.push(e);
      wake();
    };
    listeners.add(listener);
    if (liveIntervalMs > 0 && subscribers++ === 0) {
      timer = setInterval(arrive, liveIntervalMs);
    }
    return {
      [Symbol.asyncIterator]() {
        return this;
      },
      async next() {
        while (!buf.length && !done) await new Promise<void>((resolve) => (wake = resolve));
        return done ? { done: true, value: undefined } : { done: false, value: buf.shift()! };
      },
      async return() {
        if (!done && liveIntervalMs > 0 && --subscribers === 0) clearInterval(timer);
        done = true;
        listeners.delete(listener);
        wake();
        return { done: true, value: undefined };
      },
    };
  }

  const root = {
    interceptSettings: () => settings,
    interceptQueue: ({ kind }: Args) => ofKind(kind),
    updateInterceptSettings({ input }: Args) {
      const p = input as Partial<MockInterceptSettings>;
      if (p.dropStatusCode != null && (p.dropStatusCode < 100 || p.dropStatusCode > 599))
        throw fail('BAD_INPUT', 'dropStatusCode must be 100-599');
      if (p.query != null) checkQuery(p.query);
      if (p.scopeId != null && !scopeOf(p.scopeId))
        throw fail('NOT_FOUND', `scope preset ${p.scopeId} not found`);
      for (const k of ['queuing', 'requests', 'responses', 'query', 'dropStatusCode'] as const)
        if (p[k] != null) (settings as unknown as Record<string, unknown>)[k] = p[k];
      if ('scopeId' in p) settings.scopeId = p.scopeId ?? null;
      // One pass in queue order, so a mixed release stays FIFO across kinds.
      const off = (i: MockInterceptItem) =>
        !settings.queuing || !(i.kind === 'REQUEST' ? settings.requests : settings.responses);
      for (const i of queue.filter(off)) forward(take(i.id));
      emit({ interceptSettingsChanged: { ...settings } });
      return settings;
    },
    forwardInterceptItem({ id, raw }: Args) {
      const held = queue.find((i) => i.id === String(id));
      if (held && typeof raw === 'string') checkRaw(held.kind, raw);
      const item = take(id);
      forward(item);
      return item.id;
    },
    dropInterceptItem({ id }: Args) {
      const item = take(id);
      drop(item);
      return item.id;
    },
    forwardAllInterceptItems: ({ kind }: Args) => releaseAll(kind, forward),
    dropAllInterceptItems: ({ kind }: Args) => releaseAll(kind, drop),
    interceptQueueChanged: () => events('interceptQueueChanged'),
    interceptSettingsChanged: () => events('interceptSettingsChanged'),
  };

  return {
    root: Object.fromEntries(
      Object.entries(root).map(([k, f]) => [
        k,
        (args: Args) => {
          seed();
          return (f as (a: Args) => unknown)(args);
        },
      ]),
    ) as typeof root,
  };
}
