// Fake Replay backend for UI work before the real one lands (T-098).
// Resolvers follow internal/api/graphql/schema/replay.graphqls, including its error codes.
// Request records go into the history mock, so they use reserved hosts only.

import { GraphQLError } from 'graphql';
import { extOf, makeRequest, slicer, type MockRequest } from './history';

type Args = Record<string, unknown>;

interface Connection {
  host: string;
  port: number;
  tls: boolean;
  sni: string | null;
}

interface Options {
  followRedirects: boolean;
  updateContentLength: boolean;
  timeoutMs: number;
  applyMatchReplace: boolean;
}

export interface MockReplayEntry {
  id: string;
  request: MockRequest;
  error: string | null;
  createdAt: string;
}

export interface MockReplaySession {
  id: string;
  kind: 'HTTP';
  name: string;
  color: string | null;
  collectionId: string;
  connection: Connection;
  options: Options;
  /** Base64 of the draft bytes, as on the wire. */
  draft: string;
  requestTarget: () => string;
  sending: boolean;
  createdAt: string;
  /** Newest first. */
  list: MockReplayEntry[];
  collection: () => MockReplayCollection;
  entries: (args: Args) => unknown;
}

export interface MockReplayCollection {
  id: string;
  name: string;
  isDefault: boolean;
  ids: string[];
  sessions: () => MockReplaySession[];
}

/** What the replay mock needs from the history and environment mocks. */
export interface ReplayDeps {
  addRecord: (build: (id: number) => MockRequest) => MockRequest;
  request: (id: string) => MockRequest | null;
  interpolate: (text: string) => string;
}

export interface MockReplayOptions {
  /** How long a send takes before the canned response arrives, in ms (default 300). */
  sendDelayMs?: number;
}

const MAX_BYTES = 16 * 1024 * 1024;
const DEFAULT_OPTIONS: Options = {
  followRedirects: false,
  updateContentLength: true,
  timeoutMs: 60000,
  applyMatchReplace: false,
};

const fail = (code: string, message: string) => new GraphQLError(message, { extensions: { code } });

function checkName(name: unknown): string {
  const n = String(name).trim();
  // eslint-disable-next-line no-control-regex -- T-098: names must not contain control characters
  if (!n || [...n].length > 200 || /[\u0000-\u001f\u007f]/.test(n))
    throw fail('BAD_INPUT', 'name must be 1-200 characters without control characters');
  return n;
}

function checkConnection(c: Connection): Connection {
  // eslint-disable-next-line no-control-regex -- T-098: hosts must not contain whitespace or control characters
  if (!c.host || /[\s\u0000-\u001f\u007f]/.test(c.host)) throw fail('BAD_INPUT', 'invalid host');
  if (!Number.isInteger(c.port) || c.port < 1 || c.port > 65535)
    throw fail('BAD_INPUT', 'port must be 1-65535');
  if (c.sni === '') throw fail('BAD_INPUT', 'sni must not be empty; use null for no override');
  return { host: c.host, port: c.port, tls: c.tls, sni: c.sni ?? null };
}

function mergeOptions(cur: Options, p: Partial<Options> | null | undefined): Options {
  const next = { ...cur };
  for (const k of Object.keys(DEFAULT_OPTIONS) as (keyof Options)[])
    if (p?.[k] != null) (next as Record<string, unknown>)[k] = p[k];
  if (next.timeoutMs < 1000 || next.timeoutMs > 300000)
    throw fail('BAD_INPUT', 'timeoutMs must be 1000-300000');
  return next;
}

function checkBytes(b64: unknown): string {
  // Same input rules as the server's Bytes scalar (Go StdEncoding): padded, CR/LF ignored.
  if (typeof b64 !== 'string') throw fail('BAD_INPUT', 'Bytes must be a base64 string');
  const s = b64.replace(/[\r\n]/g, '');
  if (s.length % 4 !== 0 || !/^[A-Za-z0-9+/]*={0,2}$/.test(s))
    throw fail('BAD_INPUT', 'not valid base64');
  const bytes = atob(s);
  if (bytes.length > MAX_BYTES) throw fail('BAD_INPUT', 'request is larger than 16 MiB');
  return btoa(bytes); // canonical form, as the server re-encodes
}

/** HTTP/1.x request line; the method is an RFC 9110 token, e.g. M-SEARCH. */
// The target is delimited by bytes, not \s: JS treats UTF-8 continuation byte 0xA0 (in `à`) as whitespace.
const REQUEST_LINE = /^([!#$%&'*+.^_`|~0-9A-Za-z-]+) ([^ \r\n]+) HTTP\/1\.[01]\r?\n/;

/** Latin1 byte string → text, decoded as UTF-8; a leading U+FEFF is kept, not stripped. */
const utf8 = (bytes: string) =>
  new TextDecoder('utf-8', { ignoreBOM: true }).decode(
    Uint8Array.from(bytes, (ch) => ch.charCodeAt(0)),
  );

/** `METHOD target` from the request line, cut to 60 characters; `Untitled` without one. */
function defaultName(b64: string): string {
  const m = REQUEST_LINE.exec(atob(b64));
  return m ? [...utf8(`${m[1]} ${m[2]}`)].slice(0, 60).join('') : 'Untitled';
}

/** Target of the request line, UTF-8 decoded, cut to 2048 characters; empty without one or with ASCII controls. */
function requestTarget(b64: string): string {
  const t = REQUEST_LINE.exec(atob(b64))?.[2] ?? '';
  for (const ch of t) if (ch < ' ' || ch === '\x7f') return '';
  // 2048 code points span at most 8192 bytes, so decode only those.
  return [...utf8(t.slice(0, 8192))].slice(0, 2048).join('');
}

const body = (s: string) => slicer(Uint8Array.from(s, (ch) => ch.charCodeAt(0)));

/** A REPLAY record carrying exactly `raw` (latin1 text) sent at `startMs`, with a canned 200 unless `failed`. */
function record(
  id: number,
  raw: string,
  c: Connection,
  failed: boolean,
  startMs: number,
): MockRequest {
  const r = makeRequest(id, 0);
  const doneMs = Math.max(Date.now(), startMs);
  // The head ends at the first blank line, LF or CRLF, like the rawhttp parser.
  const blank = /\r?\n\r?\n/.exec(raw);
  const end = blank ? blank.index + blank[0].length : raw.length;
  const head = raw.slice(0, end);
  const rest = raw.slice(end);
  const line = REQUEST_LINE.exec(raw);
  // Same split as History's requestRow: drop the fragment, cut at the first `?`, strip absolute-form origin.
  const target = utf8(line?.[2] ?? '/').split('#', 1)[0]!;
  const q = target.indexOf('?');
  let path = q < 0 ? target : target.slice(0, q);
  const query = q < 0 ? '' : target.slice(q + 1);
  const scheme = path.indexOf('://');
  if (scheme >= 0) {
    const rest = path.slice(scheme + 3);
    const slash = rest.indexOf('/');
    path = slash < 0 ? '/' : rest.slice(slash);
  }
  const respBody = '{"ok":true,"mock":"replay"}';
  const respHead = `HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: ${respBody.length}\r\n\r\n`;
  return {
    ...r,
    source: 'REPLAY',
    createdAt: new Date(startMs).toISOString(),
    host: c.host,
    port: c.port,
    tls: c.tls,
    method: line?.[1] ?? 'GET',
    path,
    ext: extOf(path),
    query,
    color: null,
    note: null,
    isDropped: false,
    head: btoa(head),
    bodyLength: String(rest.length),
    body: body(rest),
    length: String(raw.length),
    response: failed
      ? null
      : {
          code: 200,
          length: String(respHead.length + respBody.length),
          roundtripMs: doneMs - startMs,
          createdAt: new Date(doneMs).toISOString(),
          head: btoa(respHead),
          bodyLength: String(respBody.length),
          body: body(respBody),
        },
  };
}

/** Full request bytes of a history record as latin1 text, read in 4 MiB chunks. */
function rawOf(r: MockRequest): string {
  let raw = atob(r.head);
  for (let off = 0; off < Number(r.bodyLength); off += 4194304)
    raw += atob(r.body({ offset: off, limit: 4194304, decode: false }));
  return raw;
}

/**
 * A fake Replay backend: root resolvers for graphql-js. Seeded on the first Replay operation
 * with the Default collection plus "Auth flows", three sessions and a few entries.
 */
export function createReplayMock(deps: ReplayDeps, { sendDelayMs = 300 }: MockReplayOptions = {}) {
  const collections: MockReplayCollection[] = [];
  const sessions = new Map<string, MockReplaySession>();
  const cancels = new Map<string, () => void>();
  let next = 1;

  const collection = (id: unknown): MockReplayCollection => {
    const c = collections.find((x) => x.id === String(id));
    if (!c) throw fail('NOT_FOUND', `replay collection ${String(id)} not found`);
    return c;
  };
  const session = (id: unknown): MockReplaySession => {
    const s = sessions.get(String(id));
    if (!s) throw fail('NOT_FOUND', `replay session ${String(id)} not found`);
    return s;
  };

  const addCollection = (name: string, isDefault = false): MockReplayCollection => {
    const c: MockReplayCollection = {
      id: `rc${next++}`,
      name,
      isDefault,
      ids: [],
      sessions: () => c.ids.map((id) => sessions.get(id)!),
    };
    collections.push(c);
    return c;
  };

  const addSession = (
    c: MockReplayCollection,
    fields: Pick<MockReplaySession, 'name' | 'connection' | 'draft'> & Partial<MockReplaySession>,
    at = c.ids.length,
  ): MockReplaySession => {
    const s: MockReplaySession = {
      id: `rs${next++}`,
      kind: 'HTTP',
      color: null,
      options: { ...DEFAULT_OPTIONS },
      sending: false,
      createdAt: new Date().toISOString(),
      list: [],
      ...fields,
      collectionId: c.id,
      collection: () => collection(s.collectionId),
      requestTarget: () => requestTarget(s.draft),
      entries: ({ first, after }: Args) => {
        const n = Number(first ?? 100);
        if (n < 1 || n > 1000) throw fail('BAD_INPUT', 'first must be 1..1000');
        // Deleting a History request cascades to its Replay entry, as in SQLite (project 0001_init.sql).
        s.list = s.list.filter((e) => deps.request(e.request.id));
        // Ids grow with send order and the list is newest first, so the cursor bounds the page even
        // after its entry was deleted.
        const seq = (id: string) => Number(id.slice(2));
        const start = after
          ? ((i) => (i < 0 ? s.list.length : i))(
              s.list.findIndex((e) => seq(e.id) < seq(String(after))),
            )
          : 0;
        const edges = s.list.slice(start, start + n).map((node) => ({ cursor: node.id, node }));
        return {
          edges,
          totalCount: s.list.length,
          pageInfo: {
            hasPreviousPage: start > 0,
            hasNextPage: start + n < s.list.length,
            startCursor: edges[0]?.cursor ?? null,
            endCursor: edges.at(-1)?.cursor ?? null,
          },
        };
      },
    };
    sessions.set(s.id, s);
    c.ids.splice(at, 0, s.id);
    return s;
  };

  const addEntry = (
    s: MockReplaySession,
    raw: string,
    error: string | null,
    connection = s.connection,
    createdAt = new Date().toISOString(),
  ): MockReplayEntry => {
    const e: MockReplayEntry = {
      id: `re${next++}`,
      request: deps.addRecord((id) =>
        record(id, raw, connection, error != null, Date.parse(createdAt)),
      ),
      error,
      createdAt,
    };
    s.list.unshift(e);
    return e;
  };

  const remove = (s: MockReplaySession) => {
    cancels.get(s.id)?.();
    sessions.delete(s.id);
    const c = collection(s.collectionId);
    c.ids.splice(c.ids.indexOf(s.id), 1);
  };

  let seeded = false;
  const seed = () => {
    if (seeded) return;
    seeded = true;
    const def = addCollection('Default', true);
    const auth = addCollection('Auth flows');
    const api = { host: 'api.example.com', port: 443, tls: true, sni: null };
    const users = addSession(def, {
      name: 'GET /api/v1/users',
      connection: api,
      draft: btoa('GET /api/v1/users HTTP/1.1\r\nHost: api.example.com\r\nAccept: */*\r\n\r\n'),
    });
    addEntry(users, atob(users.draft), null);
    addEntry(users, atob(users.draft), 'timeout after 60000 ms');
    addEntry(users, atob(users.draft), null);
    const login =
      'POST /login HTTP/1.1\r\nHost: auth.example.test\r\nContent-Type: application/json\r\nContent-Length: 27\r\n\r\n{"user":"a","pass":"{{pw}}"}';
    addEntry(
      addSession(auth, {
        name: 'POST /login',
        color: 'ORANGE',
        connection: { host: 'auth.example.test', port: 443, tls: true, sni: null },
        draft: btoa(login),
      }),
      login,
      null,
    );
    addSession(auth, {
      name: 'GET /me',
      connection: { host: 'auth.example.test', port: 8080, tls: false, sni: null },
      draft: btoa(
        'GET /me HTTP/1.1\r\nHost: auth.example.test:8080\r\nAuthorization: Bearer {{token}}\r\n\r\n',
      ),
    });
  };

  const root = {
    replayCollections: () => collections,
    replaySession: ({ id }: Args) => sessions.get(String(id)) ?? null,
    createReplayCollection: ({ name }: Args) => addCollection(checkName(name)),
    renameReplayCollection({ id, name }: Args) {
      const c = collection(id);
      c.name = checkName(name);
      return c;
    },
    deleteReplayCollection({ id }: Args) {
      const c = collection(id);
      if (c.isDefault) throw fail('CONFLICT', 'the Default collection cannot be deleted');
      for (const s of c.sessions()) remove(s);
      collections.splice(collections.indexOf(c), 1);
      return c.id;
    },
    reorderReplayCollections({ ids }: Args) {
      const want = (ids as string[]).map(String);
      const byId = new Map(collections.map((c) => [c.id, c]));
      if (
        want.length !== collections.length ||
        new Set(want).size !== want.length ||
        !want.every((id) => byId.has(id))
      )
        throw fail('BAD_INPUT', 'ids must list every collection exactly once');
      collections.splice(0, collections.length, ...want.map((id) => byId.get(id)!));
      return collections;
    },
    createReplaySession({ input }: Args) {
      const p = input as {
        collectionId?: string | null;
        fromRequestId?: string | null;
        connection?: Connection | null;
        raw?: string | null;
        name?: string | null;
      };
      const scratch = p.connection != null || p.raw != null;
      if ((p.fromRequestId != null) === scratch)
        throw fail('BAD_INPUT', 'give either fromRequestId or connection and raw');
      if (scratch && (p.connection == null || p.raw == null))
        throw fail('BAD_INPUT', 'a session from scratch needs both connection and raw');
      const c =
        p.collectionId != null ? collection(p.collectionId) : collections.find((x) => x.isDefault)!;
      let connection: Connection;
      let draft: string;
      if (p.fromRequestId != null) {
        const r = deps.request(p.fromRequestId);
        if (!r) throw fail('NOT_FOUND', `request ${p.fromRequestId} not found`);
        if (r.isTruncated) throw fail('BAD_INPUT', 'the stored request was truncated');
        if (atob(r.head).length + Number(r.bodyLength) > MAX_BYTES)
          throw fail('BAD_INPUT', 'request is larger than 16 MiB');
        connection = { host: r.host, port: r.port, tls: r.tls, sni: null };
        draft = btoa(rawOf(r));
      } else {
        connection = checkConnection(p.connection!);
        draft = checkBytes(p.raw);
      }
      const name = p.name != null ? checkName(p.name) : defaultName(draft);
      return addSession(c, { name, connection, draft });
    },
    updateReplaySession({ id, input }: Args) {
      const s = session(id);
      const p = input as {
        name?: string | null;
        connection?: Connection | null;
        options?: Partial<Options> | null;
        draft?: string | null;
      };
      // Validate everything before changing anything.
      const name = p.name != null ? checkName(p.name) : s.name;
      const connection = p.connection != null ? checkConnection(p.connection) : s.connection;
      const options = mergeOptions(s.options, p.options);
      const draft = p.draft != null ? checkBytes(p.draft) : s.draft;
      Object.assign(s, { name, connection, options, draft });
      return s;
    },
    setReplaySessionColor({ id, color }: Args) {
      const s = session(id);
      s.color = (color as string | null) ?? null;
      return s;
    },
    duplicateReplaySession({ id }: Args) {
      const s = session(id);
      const c = collection(s.collectionId);
      return addSession(
        c,
        {
          name: `${[...s.name].slice(0, 193).join('')} (copy)`,
          color: s.color,
          connection: { ...s.connection },
          options: { ...s.options },
          draft: s.draft,
        },
        c.ids.indexOf(s.id) + 1,
      );
    },
    moveReplaySession({ id, collectionId, index }: Args) {
      const s = session(id);
      const to = collection(collectionId);
      if (Number(index) < 0) throw fail('BAD_INPUT', 'index must be >= 0');
      const from = collection(s.collectionId);
      from.ids.splice(from.ids.indexOf(s.id), 1);
      to.ids.splice(Math.min(Number(index), to.ids.length), 0, s.id);
      s.collectionId = to.id;
      return s;
    },
    deleteReplaySession({ id }: Args) {
      const s = session(id);
      remove(s);
      return s.id;
    },
    async sendReplaySession({ id, input }: Args) {
      const s = session(id);
      if (s.sending) throw fail('CONFLICT', 'a send of this session is already in flight');
      const p = input as { raw: string; connection: Connection; options?: Partial<Options> | null };
      const draft = checkBytes(p.raw);
      const connection = checkConnection(p.connection);
      const options = mergeOptions(s.options, p.options);
      Object.assign(s, { draft, connection, options });
      // shortcut: interpolation only, no Match & Replace, Content-Length update or redirects;
      // the request-line check stands in for the rawhttp parser (T-106).
      const raw = deps.interpolate(atob(draft));
      if (!REQUEST_LINE.test(raw)) throw fail('BAD_INPUT', 'malformed request line');
      s.sending = true;
      const startedAt = new Date().toISOString();
      const timedOut = options.timeoutMs < sendDelayMs;
      const cancelled = await new Promise<boolean>((resolve) => {
        const t = setTimeout(() => resolve(false), Math.min(sendDelayMs, options.timeoutMs));
        cancels.set(s.id, () => {
          clearTimeout(t);
          resolve(true);
        });
      });
      cancels.delete(s.id);
      s.sending = false;
      const error = cancelled
        ? 'cancelled'
        : timedOut
          ? `timeout after ${options.timeoutMs} ms`
          : null;
      return addEntry(s, raw, error, connection, startedAt);
    },
    cancelReplaySend({ id }: Args) {
      const cancel = cancels.get(String(session(id).id));
      cancel?.();
      return cancel != null;
    },
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
