import { describe, expect, it, vi } from 'vitest';
import { pipe, subscribe } from 'wonka';
import { createMockClient } from './exchange';

type Client = ReturnType<typeof createMockClient>;

const SETTINGS = 'queuing requests responses query scopeId dropStatusCode';
const QUEUE = `query ($kind: InterceptItemKind) {
  interceptQueue(kind: $kind) { id kind raw heldAt request { id isDropped response { code } } }
}`;

const queue = async (c: Client, kind?: string) =>
  (await c.query(QUEUE, { kind }).toPromise()).data.interceptQueue as {
    id: string;
    kind: string;
    raw: string;
    request: { id: string; isDropped: boolean; response: { code: number } | null };
  }[];

const update = (c: Client, input: Record<string, unknown>) =>
  c
    .mutation(
      `mutation ($input: InterceptSettingsInput!) { updateInterceptSettings(input: $input) { ${SETTINGS} } }`,
      { input },
    )
    .toPromise();

/** Collects queue changes until the returned stop function is called. */
function changes(c: Client) {
  const out: { kind: string; id: string }[] = [];
  const { unsubscribe } = pipe(
    c.subscription(`subscription { interceptQueueChanged { kind item { id } } }`, {}),
    subscribe((r) =>
      out.push({
        kind: r.data.interceptQueueChanged.kind,
        id: r.data.interceptQueueChanged.item.id,
      }),
    ),
  );
  return { out, unsubscribe };
}

const client = () => createMockClient({ count: 10, liveIntervalMs: 0 });

describe('intercept mock', () => {
  it('seeds two held requests and one held response in FIFO order', async () => {
    const c = client();
    const items = await queue(c);
    expect(items.map((i) => i.kind)).toEqual(['REQUEST', 'REQUEST', 'RESPONSE']);
    expect(items[0]!.request.response).toBeNull();
    expect(items[2]!.request.response).not.toBeNull();
    expect(atob(items[0]!.raw)).toMatch(/^[A-Z]+ \S+ HTTP\/1\.1\r\n/);
    expect(atob(items[2]!.raw)).toMatch(/^HTTP\/1\.1 \d{3} /);
    expect((await queue(c, 'RESPONSE')).length).toBe(1);
  });

  it('round-trips settings and validates them', async () => {
    const c = client();
    const presets = await c.query(`{ scopePresets { id } }`, {}).toPromise();
    const scopeId = presets.data.scopePresets[0].id as string;

    const r = await update(c, { query: 'req.host.eq:"example.com"', scopeId, dropStatusCode: 403 });
    expect(r.error).toBeUndefined();
    const got = (await c.query(`{ interceptSettings { ${SETTINGS} } }`, {}).toPromise()).data
      .interceptSettings;
    expect(got).toMatchObject({ query: 'req.host.eq:"example.com"', scopeId, dropStatusCode: 403 });

    // Omitted scopeId keeps it, null clears it.
    expect((await update(c, { requests: true })).data.updateInterceptSettings.scopeId).toBe(
      scopeId,
    );
    expect((await update(c, { scopeId: null })).data.updateInterceptSettings.scopeId).toBeNull();

    const code = async (input: Record<string, unknown>) =>
      (await update(c, input)).error?.graphQLErrors[0]?.extensions.code;
    expect(await code({ dropStatusCode: 99 })).toBe('BAD_INPUT');
    expect(await code({ dropStatusCode: 600 })).toBe('BAD_INPUT');
    expect(await code({ scopeId: 'nope' })).toBe('NOT_FOUND');
    const bad = await update(c, { query: 'req.host.eq:"x' });
    expect(bad.error?.graphQLErrors[0]?.extensions).toEqual({
      code: 'BAD_QUERY',
      span: { start: 12, end: 14 },
    });
  });

  it('forward and drop remove items and emit REMOVED', async () => {
    const c = client();
    const [a, b, resp] = await queue(c);
    const ch = changes(c);
    await Promise.resolve();

    const fwd = await c
      .mutation(`mutation ($id: ID!) { forwardInterceptItem(id: $id) }`, { id: a!.id })
      .toPromise();
    expect(fwd.data.forwardInterceptItem).toBe(a!.id);
    const drop = await c
      .mutation(`mutation ($id: ID!) { dropInterceptItem(id: $id) }`, { id: resp!.id })
      .toPromise();
    expect(drop.data.dropInterceptItem).toBe(resp!.id);
    expect((await queue(c)).map((i) => i.id)).toEqual([b!.id]);
    await vi.waitFor(() =>
      expect(ch.out).toEqual([
        { kind: 'REMOVED', id: a!.id },
        { kind: 'REMOVED', id: resp!.id },
      ]),
    );
    ch.unsubscribe();

    const rec = async (id: string) =>
      (await c.query(`query ($id: ID!) { request(id: $id) { isDropped } }`, { id }).toPromise())
        .data.request.isDropped;
    expect(await rec(a!.request.id)).toBe(false);
    expect(await rec(resp!.request.id)).toBe(true);

    const again = await c
      .mutation(`mutation ($id: ID!) { dropInterceptItem(id: $id) }`, { id: a!.id })
      .toPromise();
    expect(again.error?.graphQLErrors[0]?.extensions.code).toBe('NOT_FOUND');
  });

  it('rejects unparsable raw on forward and keeps the item', async () => {
    const c = client();
    const [a] = await queue(c);
    const fwd = (raw: string) =>
      c
        .mutation(`mutation ($id: ID!, $raw: Bytes) { forwardInterceptItem(id: $id, raw: $raw) }`, {
          id: a!.id,
          raw,
        })
        .toPromise();
    expect((await fwd(btoa('garbage'))).error?.graphQLErrors[0]?.extensions.code).toBe('BAD_INPUT');
    expect((await queue(c)).length).toBe(3);
    expect((await fwd(btoa('GET / HTTP/1.1\r\nHost: example.com\r\n\r\n'))).error).toBeUndefined();
    expect((await queue(c)).length).toBe(2);
  });

  it('forward all / drop all count by kind', async () => {
    const c = client();
    await queue(c);
    const all = (op: string, kind: string | null) =>
      c.mutation(`mutation ($k: InterceptItemKind) { ${op}(kind: $k) }`, { k: kind }).toPromise();
    expect((await all('dropAllInterceptItems', 'RESPONSE')).data.dropAllInterceptItems).toBe(1);
    expect((await all('forwardAllInterceptItems', null)).data.forwardAllInterceptItems).toBe(2);
    expect(await queue(c)).toEqual([]);
  });

  it('turning queuing off releases everything and notifies settings subscribers', async () => {
    const c = client();
    await queue(c);
    const seen: boolean[] = [];
    const { unsubscribe } = pipe(
      c.subscription(`subscription { interceptSettingsChanged { queuing } }`, {}),
      subscribe((r) => seen.push(r.data.interceptSettingsChanged.queuing)),
    );
    await Promise.resolve();
    expect((await update(c, { queuing: false })).data.updateInterceptSettings.queuing).toBe(false);
    expect(await queue(c)).toEqual([]);
    await vi.waitFor(() => expect(seen).toEqual([false]));
    unsubscribe();
  });

  it('turning both sides off releases a mixed queue in FIFO order', async () => {
    const c = createMockClient({ count: 10, liveIntervalMs: 0, interceptLiveIntervalMs: 5 });
    const ch = changes(c);
    await vi.waitFor(() => expect(ch.out.length).toBeGreaterThanOrEqual(4));
    expect((await queue(c)).map((i) => i.kind).slice(0, 4)).toEqual([
      'REQUEST',
      'REQUEST',
      'RESPONSE',
      'REQUEST',
    ]);
    const seeded = (await queue(c)).slice(0, 3).map((i) => i.id);
    await update(c, { requests: false, responses: false });
    // Nothing is held once both sides are off, so every item comes back out in arrival order
    // (the seed is held before the subscription starts).
    const ids = (kind: string) => ch.out.filter((e) => e.kind === kind).map((e) => e.id);
    await vi.waitFor(() => expect(ids('REMOVED')).toEqual([...seeded, ...ids('ADDED')]));
    expect(await queue(c)).toEqual([]);
    ch.unsubscribe();
  });

  it('the timer holds only requests matching the query and scope', async () => {
    const c = createMockClient({ count: 10, liveIntervalMs: 0, interceptLiveIntervalMs: 5 });
    await update(c, { query: '"never-matches.invalid"' });
    const ch = changes(c); // the timer runs while someone watches the queue
    const before = (await queue(c)).length;
    const added: string[] = [];
    const sub = pipe(
      c.subscription(`subscription { requestAdded { id } }`, {}),
      subscribe((r) => added.push(r.data.requestAdded.id)),
    );
    await vi.waitFor(() => expect(added.length).toBeGreaterThanOrEqual(3));
    // Excluded traffic passes through; the condition change keeps already held items.
    expect((await queue(c)).length).toBe(before);
    expect(before).toBe(3);

    // A scope with an empty allowlist that denies every reserved host holds nothing either.
    const created = await c
      .mutation(`mutation { createScopePreset(name: "none", deny: ["*"]) { id } }`, {})
      .toPromise();
    await update(c, { query: '', scopeId: created.data.createScopePreset.id });
    const n = added.length;
    await vi.waitFor(() => expect(added.length).toBeGreaterThanOrEqual(n + 3));
    expect((await queue(c)).length).toBe(before);
    expect(ch.out.filter((e) => e.kind === 'ADDED')).toEqual([]);
    sub.unsubscribe();
    ch.unsubscribe();
  });

  it('turning responses off releases only responses', async () => {
    const c = client();
    await queue(c);
    await update(c, { responses: false });
    expect((await queue(c)).map((i) => i.kind)).toEqual(['REQUEST', 'REQUEST']);
  });

  it('holds new requests on a timer only when opted in', async () => {
    const c = createMockClient({ count: 10, liveIntervalMs: 0, interceptLiveIntervalMs: 10 });
    const ch = changes(c);
    await vi.waitFor(() =>
      expect(ch.out.filter((e) => e.kind === 'ADDED').length).toBeGreaterThanOrEqual(2),
    );
    ch.unsubscribe();

    const off = client();
    const quiet = changes(off);
    await queue(off);
    await new Promise((r) => setTimeout(r, 50));
    expect(quiet.out).toEqual([]);
    quiet.unsubscribe();
  });
});

describe('history mock edited pairs', () => {
  it('resolves editedBy and editedFrom both ways', async () => {
    const c = client();
    const q = `query ($id: ID!) { request(id: $id) { id editedFrom { id } editedBy { id } } }`;
    const get = async (id: string) => (await c.query(q, { id }).toPromise()).data.request;
    const c30 = createMockClient({ count: 30, liveIntervalMs: 0 });
    const get30 = async (id: string) => (await c30.query(q, { id }).toPromise()).data.request;
    expect(await get30('24')).toEqual({ id: '24', editedFrom: null, editedBy: { id: '25' } });
    expect(await get30('25')).toEqual({ id: '25', editedFrom: { id: '24' }, editedBy: null });
    expect(await get('3')).toMatchObject({ editedFrom: null, editedBy: null });
  });

  it('pairs one exchange: request edited, response bytes unchanged', async () => {
    const c = createMockClient({ count: 30, liveIntervalMs: 0 });
    const q = `query ($id: ID!) { request(id: $id) {
      host port tls method path query source head response { head body(limit: 4194304) }
    } }`;
    const get = async (id: string) => (await c.query(q, { id }).toPromise()).data.request;
    const [orig, edited] = [await get('24'), await get('25')];
    for (const k of ['host', 'port', 'tls', 'method', 'path', 'query'])
      expect(edited[k]).toEqual(orig[k]);
    expect(edited.source).toBe('INTERCEPT');
    expect(atob(edited.head)).toBe(atob(orig.head).replace(/\r\n\r\n$/, '\r\nX-Edited: 1\r\n\r\n'));
    expect(edited.response).toEqual(orig.response);
  });
});
