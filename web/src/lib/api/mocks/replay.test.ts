import { describe, expect, it } from 'vitest';
import { createMockClient } from './exchange';

const client = () => createMockClient({ count: 3, liveIntervalMs: 0, replaySendDelayMs: 50 });
type Res = {
  data?: Record<string, any>; // eslint-disable-line @typescript-eslint/no-explicit-any -- T-098: test shorthand
  error?: { graphQLErrors: { extensions: Record<string, unknown> }[] };
};
const code = (res: Res) => res.error?.graphQLErrors[0]?.extensions.code;

const TREE = `{ replayCollections { id name isDefault sessions { id name color } } }`;
const CONN = { host: 'example.test', port: 8080, tls: false };
const RAW = btoa('GET /a HTTP/1.1\r\nHost: example.test\r\n\r\n');

function run(c: ReturnType<typeof client>) {
  return {
    q: (q: string, v = {}) =>
      c.query(q, v, { requestPolicy: 'network-only' }).toPromise() as Promise<Res>,
    m: (q: string, v = {}) => c.mutation(q, v).toPromise() as Promise<Res>,
  };
}

describe('replay mock', () => {
  it('seeds Default plus one collection with three sessions and entries', async () => {
    const { q } = run(client());
    const tree = (await q(TREE)).data!.replayCollections;
    expect(tree.map((c: { name: string }) => c.name)).toEqual(['Default', 'Auth flows']);
    expect(tree[0].isDefault).toBe(true);
    expect(tree.flatMap((c: { sessions: unknown[] }) => c.sessions)).toHaveLength(3);
    const s = (
      await q(
        `{ replaySession(id: "rs3") { entries(first: 2) { totalCount edges { node { error request { source } } } pageInfo { hasNextPage } } } }`,
      )
    ).data!.replaySession.entries;
    expect(s.totalCount).toBe(3);
    expect(s.edges).toHaveLength(2);
    expect(s.pageInfo.hasNextPage).toBe(true);
    expect(s.edges[1].node.error).toBe('timeout after 60000 ms');
    expect(s.edges[0].node.request.source).toBe('REPLAY');
  });

  it('creates, renames, reorders, moves, duplicates and deletes', async () => {
    const { q, m } = run(client());
    const created = await m(`mutation { createReplayCollection(name: " Mine ") { id name } }`);
    expect(created.data!.createReplayCollection).toEqual({ id: 'rc10', name: 'Mine' });

    const fromScratch = await m(
      `mutation ($c: ReplayConnectionInput!, $raw: Bytes!) { createReplaySession(input: { connection: $c, raw: $raw }) { id name collection { isDefault } options { timeoutMs updateContentLength } } }`,
      { c: CONN, raw: RAW },
    );
    expect(fromScratch.data!.createReplaySession).toMatchObject({
      name: 'GET /a',
      collection: { isDefault: true },
      options: { timeoutMs: 60000, updateContentLength: true },
    });
    const sid = fromScratch.data!.createReplaySession.id;

    const fromHistory = await m(
      `mutation { createReplaySession(input: { fromRequestId: "1", collectionId: "rc10" }) { connection { host } draft } }`,
    );
    expect(fromHistory.error).toBeUndefined();
    expect(atob(fromHistory.data!.createReplaySession.draft)).toMatch(/ HTTP\/1\.1\r\n/);

    await m(`mutation { renameReplayCollection(id: "rc1", name: "Main") { id } }`);
    await m(`mutation { reorderReplayCollections(ids: ["rc10", "rc2", "rc1"]) { id } }`);
    const moved = await m(
      `mutation { moveReplaySession(id: "${sid}", collectionId: "rc2", index: 0) { collection { id } } }`,
    );
    expect(moved.data!.moveReplaySession.collection.id).toBe('rc2');
    const dup = await m(`mutation { duplicateReplaySession(id: "rs7") { name color } }`);
    expect(dup.data!.duplicateReplaySession).toEqual({
      name: 'POST /login (copy)',
      color: 'ORANGE',
    });

    let tree = (await q(TREE)).data!.replayCollections;
    expect(tree.map((c: { name: string }) => c.name)).toEqual(['Mine', 'Auth flows', 'Main']);
    expect(tree[1].sessions.map((s: { name: string }) => s.name)).toEqual([
      'GET /a',
      'POST /login',
      'POST /login (copy)',
      'GET /me',
    ]);

    expect(
      (await m(`mutation { deleteReplaySession(id: "${sid}") }`)).data!.deleteReplaySession,
    ).toBe(sid);
    expect(
      (await m(`mutation { deleteReplayCollection(id: "rc2") }`)).data!.deleteReplayCollection,
    ).toBe('rc2');
    tree = (await q(TREE)).data!.replayCollections;
    expect(tree.map((c: { id: string }) => c.id)).toEqual(['rc10', 'rc1']);
    expect((await q(`{ replaySession(id: "rs7") { id } }`)).data!.replaySession).toBeNull();
  });

  it('updates sessions and colours', async () => {
    const { m } = run(client());
    const up = await m(
      `mutation { updateReplaySession(id: "rs9", input: { name: "Me", options: { timeoutMs: 5000 } }) { name options { timeoutMs followRedirects } } }`,
    );
    expect(up.data!.updateReplaySession).toEqual({
      name: 'Me',
      options: { timeoutMs: 5000, followRedirects: false },
    });
    const col = await m(`mutation { setReplaySessionColor(id: "rs9", color: BLUE) { color } }`);
    expect(col.data!.setReplaySessionColor.color).toBe('BLUE');
  });

  it('sends with interpolation, rejects a second send, and cancels', async () => {
    const { m } = run(client());
    const SEND = `mutation ($c: ReplayConnectionInput!, $raw: Bytes!) { sendReplaySession(id: "rs9", input: { raw: $raw, connection: $c }) { error request { head response { code } } } }`;
    const raw = btoa(
      'GET /me HTTP/1.1\r\nHost: auth.example.test\r\nAuthorization: Bearer {{token}}\r\nX-U: {{ nope }}\r\n\r\n',
    );

    const ok = await m(SEND, { c: CONN, raw });
    expect(ok.data!.sendReplaySession.error).toBeNull();
    expect(ok.data!.sendReplaySession.request.response.code).toBe(200);
    const head = atob(ok.data!.sendReplaySession.request.head);
    expect(head).toContain('Bearer mock-secret-token');
    expect(head).toContain('{{ nope }}');

    const first = m(SEND, { c: CONN, raw });
    await new Promise((r) => setTimeout(r, 10));
    expect(code(await m(SEND, { c: CONN, raw }))).toBe('CONFLICT');
    expect((await m(`mutation { cancelReplaySend(id: "rs9") }`)).data!.cancelReplaySend).toBe(true);
    const cancelled = (await first).data!.sendReplaySession;
    expect(cancelled.error).toBe('cancelled');
    expect(cancelled.request.response).toBeNull();
    expect((await m(`mutation { cancelReplaySend(id: "rs9") }`)).data!.cancelReplaySend).toBe(
      false,
    );
  });

  it('keeps the starting connection when the session changes during a send', async () => {
    const { m } = run(client());
    const pending = m(
      `mutation ($c: ReplayConnectionInput!, $raw: Bytes!) { sendReplaySession(id: "rs9", input: { raw: $raw, connection: $c }) { request { host port } } }`,
      { c: CONN, raw: RAW },
    );
    await new Promise((r) => setTimeout(r, 10));
    await m(
      `mutation { updateReplaySession(id: "rs9", input: { connection: { host: "other.test", port: 9999, tls: true } }) { id } }`,
    );
    expect((await pending).data!.sendReplaySession.request).toEqual({
      host: 'example.test',
      port: 8080,
    });
  });

  it('interpolates non-ASCII values as UTF-8 and keeps other bytes', async () => {
    const { m } = run(client());
    await m(
      `mutation { setEnvironmentVariables(id: "env1", variables: [{ name: "v", value: "é東京" }]) { id } }`,
    );
    const raw = 'POST /x HTTP/1.1\r\nHost: example.test\r\n\r\n\xff{{v}}';
    const res = await m(
      `mutation ($c: ReplayConnectionInput!, $raw: Bytes!) { sendReplaySession(id: "rs9", input: { raw: $raw, connection: $c }) { request { body } } }`,
      { c: CONN, raw: btoa(raw) },
    );
    const want = [0xff, ...new TextEncoder().encode('é東京')];
    const got = Array.from(atob(res.data!.sendReplaySession.request.body), (ch) =>
      ch.charCodeAt(0),
    );
    expect(got).toEqual(want);
  });

  it('slices large bodies and copies them whole into a new session', async () => {
    const { q, m } = run(client());
    const head = 'POST /up HTTP/1.1\r\nHost: example.test\r\n\r\n';
    const raw = head + 'a'.repeat(4 * 1024 * 1024) + 'z';
    const sent = await m(
      `mutation ($c: ReplayConnectionInput!, $raw: Bytes!) { sendReplaySession(id: "rs9", input: { raw: $raw, connection: $c }) { request { id ext body(offset: 4194304, limit: 1) } } }`,
      { c: CONN, raw: btoa(raw) },
    );
    const r = sent.data!.sendReplaySession.request;
    expect(atob(r.body)).toBe('z');
    expect(r.ext).toBe('');
    const copy = await m(
      `mutation ($id: ID!) { createReplaySession(input: { fromRequestId: $id }) { draft } }`,
      { id: r.id },
    );
    expect(atob(copy.data!.createReplaySession.draft)).toBe(raw);
    const seeded = await q(
      `{ replaySession(id: "rs3") { entries(first: 1) { edges { node { request { ext } } } } } }`,
    );
    expect(seeded.data!.replaySession.entries.edges[0].node.request.ext).toBe('');
  });

  it('accepts token methods and keeps the whole query', async () => {
    const { m } = run(client());
    const res = await m(
      `mutation ($c: ReplayConnectionInput!, $raw: Bytes!) { sendReplaySession(id: "rs9", input: { raw: $raw, connection: $c }) { error request { method path query } } }`,
      { c: CONN, raw: btoa('M-SEARCH /search?q=a?b HTTP/1.1\r\nHost: example.test\r\n\r\n') },
    );
    expect(res.data!.sendReplaySession.error).toBeNull();
    expect(res.data!.sendReplaySession.request).toEqual({
      method: 'M-SEARCH',
      path: '/search',
      query: 'q=a?b',
    });
  });

  it('times out at timeoutMs with no response and dates the entry at the start', async () => {
    const { m } = run(createMockClient({ count: 3, liveIntervalMs: 0, replaySendDelayMs: 1250 }));
    const before = Date.now();
    const res = await m(
      `mutation ($c: ReplayConnectionInput!, $raw: Bytes!) { sendReplaySession(id: "rs9", input: { raw: $raw, connection: $c, options: { timeoutMs: 1000 } }) { error createdAt request { response { code } } } }`,
      { c: CONN, raw: RAW },
    );
    const e = res.data!.sendReplaySession;
    expect(e.error).toBe('timeout after 1000 ms');
    expect(e.request.response).toBeNull();
    expect(Date.parse(e.createdAt) - before).toBeLessThan(500);
  });

  it('normalizes Bytes like the server: CR/LF ignored, canonical padding', async () => {
    const { m } = run(client());
    const create = (raw: string) =>
      m(
        `mutation ($c: ReplayConnectionInput!, $raw: Bytes!) { createReplaySession(input: { connection: $c, raw: $raw }) { draft } }`,
        { c: CONN, raw },
      );
    expect((await create('YR==')).data!.createReplaySession.draft).toBe('YQ==');
    expect(
      (await create(RAW.slice(0, 8) + '\r\n' + RAW.slice(8))).data!.createReplaySession.draft,
    ).toBe(RAW);
  });

  it('rejects non-string Bytes variables', async () => {
    const { m } = run(client());
    for (const raw of [1234, ['YQ==']]) {
      const res = await m(
        `mutation ($c: ReplayConnectionInput!, $raw: Bytes!) { createReplaySession(input: { connection: $c, raw: $raw }) { draft } }`,
        { c: CONN, raw },
      );
      expect(code(res)).toBe('BAD_INPUT');
    }
  });

  it('records the origin-form path for an absolute-form target, like History', async () => {
    const { m } = run(client());
    const raw = 'GET http://example.test/docs?q=abc#frag HTTP/1.1\r\nHost: example.test\r\n\r\n';
    const res = await m(
      `mutation ($c: ReplayConnectionInput!, $raw: Bytes!) { sendReplaySession(id: "rs9", input: { raw: $raw, connection: $c }) { request { path query ext head } } }`,
      { c: CONN, raw: btoa(raw) },
    );
    const r = res.data!.sendReplaySession.request;
    expect({ path: r.path, query: r.query, ext: r.ext }).toEqual({
      path: '/docs',
      query: 'q=abc',
      ext: '',
    });
    expect(atob(r.head)).toBe(raw);
  });

  it('splits an LF-only request at its blank line', async () => {
    const { m } = run(client());
    const raw = 'POST /lf HTTP/1.1\nHost: example.test\n\nbody\n';
    const res = await m(
      `mutation ($c: ReplayConnectionInput!, $raw: Bytes!) { sendReplaySession(id: "rs9", input: { raw: $raw, connection: $c }) { request { head body } } }`,
      { c: CONN, raw: btoa(raw) },
    );
    const r = res.data!.sendReplaySession.request;
    expect(atob(r.head)).toBe('POST /lf HTTP/1.1\nHost: example.test\n\n');
    expect(atob(r.body)).toBe('body\n');
  });

  it('rejects copying a history request larger than 16 MiB', async () => {
    const { m } = run(client());
    await m(
      `mutation ($v: [EnvironmentVariableInput!]!) { setEnvironmentVariables(id: "env1", variables: $v) { id } }`,
      {
        v: [{ name: 'big', value: 'a'.repeat(1_000_000), secret: false }],
      },
    );
    const raw = 'POST /big HTTP/1.1\r\nHost: example.test\r\n\r\n' + '{{big}}'.repeat(17);
    const sent = await m(
      `mutation ($c: ReplayConnectionInput!, $raw: Bytes!) { sendReplaySession(id: "rs9", input: { raw: $raw, connection: $c }) { request { id } } }`,
      { c: CONN, raw: btoa(raw) },
    );
    const copy = await m(
      `mutation ($id: ID!) { createReplaySession(input: { fromRequestId: $id }) { id } }`,
      { id: sent.data!.sendReplaySession.request.id },
    );
    expect(code(copy)).toBe('BAD_INPUT');
  }, 30_000); // builds a 17 MB record; slow under a parallel full suite

  it('decodes UTF-8 default names and keeps the draft bytes', async () => {
    const { m } = run(client());
    const raw = btoa(
      String.fromCharCode(
        ...new TextEncoder().encode('GET /café HTTP/1.1\r\nHost: example.test\r\n\r\n'),
      ),
    );
    const res = await m(
      `mutation ($c: ReplayConnectionInput, $raw: Bytes) { createReplaySession(input: { connection: $c, raw: $raw }) { name draft } }`,
      { c: CONN, raw },
    );
    expect(res.data!.createReplaySession).toEqual({ name: 'GET /café', draft: raw });
  });

  it('reports BAD_INPUT, CONFLICT and NOT_FOUND', async () => {
    const { m } = run(client());
    const create = (input: string, v = {}) =>
      m(
        `mutation ${input.includes('$') ? '($c: ReplayConnectionInput, $raw: Bytes)' : ''} { createReplaySession(input: ${input}) { id } }`,
        v,
      );
    expect(code(await m(`mutation { deleteReplayCollection(id: "rc1") }`))).toBe('CONFLICT');
    expect(code(await m(`mutation { createReplayCollection(name: "  ") { id } }`))).toBe(
      'BAD_INPUT',
    );
    expect(
      code(
        await m(
          `mutation { renameReplayCollection(id: "rc1", name: "${'x'.repeat(201)}") { id } }`,
        ),
      ),
    ).toBe('BAD_INPUT');
    expect(code(await m(`mutation { reorderReplayCollections(ids: ["rc1"]) { id } }`))).toBe(
      'BAD_INPUT',
    );
    expect(code(await create(`{}`))).toBe('BAD_INPUT');
    expect(
      code(
        await create(`{ fromRequestId: "1", connection: $c, raw: $raw }`, { c: CONN, raw: RAW }),
      ),
    ).toBe('BAD_INPUT');
    expect(
      code(
        await create(`{ connection: $c, raw: $raw }`, { c: { ...CONN, port: 70000 }, raw: RAW }),
      ),
    ).toBe('BAD_INPUT');
    expect(code(await create(`{ fromRequestId: "999" }`))).toBe('NOT_FOUND');
    expect(code(await create(`{ connection: $c, raw: $raw }`, { c: CONN, raw: 'YQ' }))).toBe(
      'BAD_INPUT',
    );
    expect(
      code(
        await m(
          `mutation { updateReplaySession(id: "rs9", input: { options: { timeoutMs: 999 } }) { id } }`,
        ),
      ),
    ).toBe('BAD_INPUT');
    expect(
      code(
        await m(`mutation { moveReplaySession(id: "rs9", collectionId: "rc1", index: -1) { id } }`),
      ),
    ).toBe('BAD_INPUT');
    expect(code(await m(`mutation { deleteReplaySession(id: "nope") }`))).toBe('NOT_FOUND');
    expect(code(await m(`mutation { cancelReplaySend(id: "nope") }`))).toBe('NOT_FOUND');
  });
});

describe('environment mock', () => {
  const ENVS = `{ environments { id name isGlobal selected variables { name value secret } } }`;

  it('lists Global first, then by name, with one secret variable', async () => {
    const { q } = run(client());
    const envs = (await q(ENVS)).data!.environments;
    expect(envs.map((e: { name: string }) => e.name)).toEqual(['Global', 'Production', 'Staging']);
    expect(envs[2].selected).toBe(true);
    expect(
      envs
        .flatMap((e: { variables: { secret: boolean }[] }) => e.variables)
        .filter((v: { secret: boolean }) => v.secret),
    ).toHaveLength(1);
  });

  it('creates, renames, sets variables, selects and deletes', async () => {
    const { q, m } = run(client());
    const id = (await m(`mutation { createEnvironment(name: "Dev") { id } }`)).data!
      .createEnvironment.id;
    await m(`mutation { renameEnvironment(id: "${id}", name: "Local") { id } }`);
    const set = await m(
      `mutation { setEnvironmentVariables(id: "${id}", variables: [{ name: "a", value: "1" }, { name: "b", value: "2", secret: true }]) { variables { name secret } } }`,
    );
    expect(set.data!.setEnvironmentVariables.variables).toEqual([
      { name: 'a', secret: false },
      { name: 'b', secret: true },
    ]);
    expect(
      (await m(`mutation { selectEnvironment(id: "${id}") { name selected } }`)).data!
        .selectEnvironment,
    ).toEqual({
      name: 'Local',
      selected: true,
    });
    expect(
      (await m(`mutation { selectEnvironment(id: null) { id } }`)).data!.selectEnvironment,
    ).toBeNull();
    expect((await q(ENVS)).data!.environments.some((e: { selected: boolean }) => e.selected)).toBe(
      false,
    );
    expect((await m(`mutation { deleteEnvironment(id: "${id}") }`)).data!.deleteEnvironment).toBe(
      id,
    );
  });

  it('reports BAD_INPUT, CONFLICT and NOT_FOUND', async () => {
    const { m } = run(client());
    const vars = (v: string) =>
      m(`mutation { setEnvironmentVariables(id: "env1", variables: ${v}) { id } }`);
    expect(
      code(await m(`mutation { renameEnvironment(id: "env-global", name: "G") { id } }`)),
    ).toBe('CONFLICT');
    expect(code(await m(`mutation { deleteEnvironment(id: "env-global") }`))).toBe('CONFLICT');
    expect(code(await m(`mutation { createEnvironment(name: "Staging") { id } }`))).toBe(
      'CONFLICT',
    );
    expect(code(await m(`mutation { createEnvironment(name: "Global") { id } }`))).toBe('CONFLICT');
    expect(code(await m(`mutation { createEnvironment(name: "") { id } }`))).toBe('BAD_INPUT');
    expect(code(await vars(`[{ name: "1x", value: "" }]`))).toBe('BAD_INPUT');
    expect(code(await vars(`[{ name: "a", value: "" }, { name: "a", value: "" }]`))).toBe(
      'BAD_INPUT',
    );
    expect(code(await m(`mutation { selectEnvironment(id: "env-global") { id } }`))).toBe(
      'BAD_INPUT',
    );
    expect(code(await m(`mutation { deleteEnvironment(id: "nope") }`))).toBe('NOT_FOUND');
  });
});
