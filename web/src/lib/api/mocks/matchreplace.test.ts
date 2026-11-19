import { describe, expect, it } from 'vitest';
import { createMockClient } from './exchange';

type Res = {
  data?: Record<string, any>; // eslint-disable-line @typescript-eslint/no-explicit-any -- T-099: test shorthand
  error?: { graphQLErrors: { extensions: Record<string, unknown> }[] };
};
const code = (res: Res) => res.error?.graphQLErrors[0]?.extensions.code;

const TREE = `{ matchReplaceCollections { id name rules { id name enabled section operation matcher { kind value } term collection { id } } } }`;
const RULE = `id name enabled sources collection { id } matcher { kind value } term`;

function run() {
  const c = createMockClient({ count: 1, liveIntervalMs: 0 });
  return {
    q: (q: string, v = {}) =>
      c.query(q, v, { requestPolicy: 'network-only' }).toPromise() as Promise<Res>,
    m: (q: string, v = {}) => c.mutation(q, v).toPromise() as Promise<Res>,
  };
}

const addHeader = {
  name: 'Add',
  sources: ['PROXY'],
  section: 'REQUEST_HEADER',
  operation: 'ADD',
  matcher: { kind: 'FULL' },
  term: 'X-Test: 1',
};
const CREATE = `mutation ($c: ID!, $i: MatchReplaceRuleInput!) { createMatchReplaceRule(collectionId: $c, input: $i) { ${RULE} } }`;
const TEST = `mutation ($r: MatchReplaceRuleInput!, $k: MatchReplaceMessageKind!, $raw: Bytes!) { testMatchReplace(rule: $r, kind: $k, raw: $raw) { output conditionMatched changed } }`;

describe('match & replace mock', () => {
  it('seeds two collections with four rules', async () => {
    const { q } = run();
    const tree = (await q(TREE)).data!.matchReplaceCollections;
    expect(tree.map((c: { name: string }) => c.name)).toEqual(['Headers', 'Bodies']);
    expect(tree.flatMap((c: { rules: unknown[] }) => c.rules)).toHaveLength(4);
    expect(tree[1].rules[0].collection.id).toBe(tree[1].id);
  });

  it('creates, renames, reorders and deletes collections', async () => {
    const { q, m } = run();
    const c = (
      await m(`mutation { createMatchReplaceCollection(name: " Mine ") { id name rules { id } } }`)
    ).data!.createMatchReplaceCollection;
    expect(c.name).toBe('Mine');
    expect(code(await m(`mutation { createMatchReplaceCollection(name: "  ") { id } }`))).toBe(
      'BAD_INPUT',
    );
    expect(
      (await m(`mutation { renameMatchReplaceCollection(id: "${c.id}", name: "B") { name } }`))
        .data!.renameMatchReplaceCollection.name,
    ).toBe('B');
    const ids = (await q(TREE)).data!.matchReplaceCollections.map((x: { id: string }) => x.id);
    expect(
      code(
        await m(`mutation ($ids: [ID!]!) { reorderMatchReplaceCollections(ids: $ids) { id } }`, {
          ids: ids.slice(1),
        }),
      ),
    ).toBe('BAD_INPUT');
    const reordered = (
      await m(`mutation ($ids: [ID!]!) { reorderMatchReplaceCollections(ids: $ids) { id } }`, {
        ids: [...ids].reverse(),
      })
    ).data!.reorderMatchReplaceCollections;
    expect(reordered.map((x: { id: string }) => x.id)).toEqual([...ids].reverse());
    const first = ids[0];
    expect(
      (await m(`mutation { deleteMatchReplaceCollection(id: "${first}") }`)).data!
        .deleteMatchReplaceCollection,
    ).toBe(first);
    expect(code(await m(`mutation { deleteMatchReplaceCollection(id: "${first}") }`))).toBe(
      'NOT_FOUND',
    );
    expect(
      (await q(TREE)).data!.matchReplaceCollections.flatMap((x: { rules: unknown[] }) => x.rules),
    ).toHaveLength(2);
  });

  it('creates, updates, enables, moves and deletes rules', async () => {
    const { q, m } = run();
    const [headers, bodies] = (await q(TREE)).data!.matchReplaceCollections;
    const r = (await m(CREATE, { c: headers.id, i: { ...addHeader, sources: ['PROXY', 'PROXY'] } }))
      .data!.createMatchReplaceRule;
    expect(r).toMatchObject({
      enabled: true,
      sources: ['PROXY'],
      matcher: { kind: 'FULL', value: '' },
    });

    const updated = (
      await m(
        `mutation ($id: ID!, $i: MatchReplaceRuleInput!) { updateMatchReplaceRule(id: $id, input: $i) { ${RULE} } }`,
        {
          id: r.id,
          i: { ...addHeader, name: 'Renamed', enabled: false },
        },
      )
    ).data!.updateMatchReplaceRule;
    expect(updated).toMatchObject({ name: 'Renamed', enabled: false });
    expect(
      (await m(`mutation { setMatchReplaceRuleEnabled(id: "${r.id}", enabled: true) { enabled } }`))
        .data!.setMatchReplaceRuleEnabled.enabled,
    ).toBe(true);

    const moved = (
      await m(
        `mutation { moveMatchReplaceRule(id: "${r.id}", collectionId: "${bodies.id}", index: 0) { collection { id } } }`,
      )
    ).data!.moveMatchReplaceRule;
    expect(moved.collection.id).toBe(bodies.id);
    const tree = (await q(TREE)).data!.matchReplaceCollections;
    expect(tree[1].rules[0].id).toBe(r.id);
    expect(tree[0].rules).toHaveLength(2);
    expect(
      code(
        await m(
          `mutation { moveMatchReplaceRule(id: "${r.id}", collectionId: "${bodies.id}", index: -1) { id } }`,
        ),
      ),
    ).toBe('BAD_INPUT');

    expect(
      (await m(`mutation { deleteMatchReplaceRule(id: "${r.id}") }`)).data!.deleteMatchReplaceRule,
    ).toBe(r.id);
    expect(
      (await q(`{ matchReplaceRule(id: "${r.id}") { id } }`)).data!.matchReplaceRule,
    ).toBeNull();
  });

  it('rejects invalid rules with the contract codes', async () => {
    const { q, m } = run();
    const c = (await q(TREE)).data!.matchReplaceCollections[0].id;
    const bad = async (i: object) => code(await m(CREATE, { c, i: { ...addHeader, ...i } }));
    expect(await bad({ sources: [] })).toBe('BAD_INPUT');
    expect(await bad({ section: 'REQUEST_BODY', operation: 'ADD' })).toBe('BAD_INPUT');
    expect(await bad({ operation: 'UPDATE_RAW', matcher: { kind: 'NAME', value: 'A' } })).toBe(
      'BAD_INPUT',
    );
    expect(await bad({ operation: 'UPDATE_RAW', matcher: { kind: 'STRING' } })).toBe('BAD_INPUT');
    expect(await bad({ operation: 'UPDATE_RAW', matcher: { kind: 'REGEX', value: '(?=a)' } })).toBe(
      'BAD_INPUT',
    );
    expect(await bad({ term: 'no colon' })).toBe('BAD_INPUT');
    expect(await bad({ name: 'x'.repeat(201) })).toBe('BAD_INPUT');
    const q2 = await m(CREATE, { c, i: { ...addHeader, condition: 'req.host.eq:"x' } });
    expect(code(q2)).toBe('BAD_QUERY');
    expect(q2.error!.graphQLErrors[0]!.extensions.span).toEqual({ start: 12, end: 14 });
    const q3 = await m(CREATE, {
      c,
      i: { ...addHeader, condition: 'req.path.eq:"é" AND req.host.eq:"x' },
    });
    expect(q3.error!.graphQLErrors[0]!.extensions.span).toEqual({ start: 33, end: 35 });
    expect(code(await m(CREATE, { c: 'nope', i: addHeader }))).toBe('NOT_FOUND');
  });

  it('round-trips export and import, all or nothing', async () => {
    const { q, m } = run();
    const json = (await q(`{ exportMatchReplace }`)).data!.exportMatchReplace;
    const parsed = JSON.parse(json);
    expect(parsed.version).toBe(1);
    expect(parsed.collections[0].rules[0]).not.toHaveProperty('id');
    const IMPORT = `mutation ($j: String!) { importMatchReplace(json: $j) }`;
    expect((await m(IMPORT, { j: json })).data!.importMatchReplace).toBe(2);
    const tree = (await q(TREE)).data!.matchReplaceCollections;
    expect(tree.map((c: { name: string }) => c.name)).toEqual([
      'Headers',
      'Bodies',
      'Headers',
      'Bodies',
    ]);
    const again = JSON.parse((await q(`{ exportMatchReplace }`)).data!.exportMatchReplace);
    expect(again.collections.slice(2)).toEqual(parsed.collections);

    expect(code(await m(IMPORT, { j: '{' }))).toBe('BAD_INPUT');
    expect(code(await m(IMPORT, { j: '{"version":2,"collections":[]}' }))).toBe('BAD_INPUT');
    parsed.collections[1]!.rules![0]!.sources = [];
    expect(code(await m(IMPORT, { j: JSON.stringify(parsed) }))).toBe('BAD_INPUT');
    expect((await q(TREE)).data!.matchReplaceCollections).toHaveLength(4);
  });

  type Obj = Record<string, unknown>;
  type Dump = Obj & {
    collections: (Obj & { rules?: ((Obj & { matcher: Obj }) | null)[] })[];
  };

  it.each([
    ['unknown source', (d: Dump) => (d.collections[0]!.rules![0]!.sources = ['BOGUS'])],
    ['sources not a list', (d: Dump) => (d.collections[0]!.rules![0]!.sources = 'PROXY')],
    ['missing matcher value', (d: Dump) => delete d.collections[0]!.rules![0]!.matcher.value],
    ['unknown matcher kind', (d: Dump) => (d.collections[0]!.rules![0]!.matcher.kind = 'GLOB')],
    ['non-boolean enabled', (d: Dump) => (d.collections[0]!.rules![0]!.enabled = 'yes')],
    ['non-string term', (d: Dump) => (d.collections[1]!.rules![0]!.term = 7)],
    ['unknown rule field', (d: Dump) => (d.collections[0]!.rules![0]!.extra = 1)],
    ['unknown matcher field', (d: Dump) => (d.collections[0]!.rules![0]!.matcher.x = 1)],
    ['missing rule field', (d: Dump) => delete d.collections[0]!.rules![0]!.condition],
    ['rule not an object', (d: Dump) => (d.collections[0]!.rules![0] = null)],
    ['missing collection name', (d: Dump) => delete d.collections[1]!.name],
    ['non-string collection name', (d: Dump) => (d.collections[1]!.name = 5)],
    ['missing rules', (d: Dump) => delete d.collections[1]!.rules],
    ['unknown collection field', (d: Dump) => (d.collections[1]!.id = 'x')],
    ['unknown top-level field', (d: Dump) => (d.extra = true)],
    ['collections not an array', (d: Dump) => Object.assign(d, { collections: {} })],
  ])('rejects an import with %s and changes nothing', async (_, mutate) => {
    const { q, m } = run();
    const json = (await q(`{ exportMatchReplace }`)).data!.exportMatchReplace;
    const data = JSON.parse(json);
    mutate(data);
    const IMPORT = `mutation ($j: String!) { importMatchReplace(json: $j) }`;
    expect(code(await m(IMPORT, { j: JSON.stringify(data) }))).toBe('BAD_INPUT');
    expect((await q(`{ exportMatchReplace }`)).data!.exportMatchReplace).toBe(json);
    expect((await q(TREE)).error).toBeUndefined();
  });

  it('tests header and body rules', async () => {
    const { m } = run();
    const req = btoa('GET / HTTP/1.1\r\nHost: www.example.com\r\n\r\n');
    const added = (await m(TEST, { r: { ...addHeader, sources: [] }, k: 'REQUEST', raw: req }))
      .data!.testMatchReplace;
    expect(added).toEqual({
      output: btoa('GET / HTTP/1.1\r\nHost: www.example.com\r\nX-Test: 1\r\n\r\n'),
      conditionMatched: true,
      changed: true,
    });
    const search = btoa('M-SEARCH * HTTP/1.1\r\nHost: 239.255.255.250\r\n\r\n');
    const ext = (await m(TEST, { r: { ...addHeader, sources: [] }, k: 'REQUEST', raw: search }))
      .data!.testMatchReplace;
    expect(atob(ext.output)).toBe(
      'M-SEARCH * HTTP/1.1\r\nHost: 239.255.255.250\r\nX-Test: 1\r\n\r\n',
    );

    const resp = btoa('HTTP/1.1 200 OK\r\nContent-Length: 3\r\n\r\nabc');
    const body = {
      name: '',
      sources: [],
      section: 'RESPONSE_BODY',
      operation: 'UPDATE_RAW',
      matcher: { kind: 'STRING', value: 'b' },
      term: 'xyz',
    };
    const out = (await m(TEST, { r: body, k: 'RESPONSE', raw: resp })).data!.testMatchReplace;
    expect(atob(out.output)).toBe('HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\naxyzc');
    const same = (
      await m(TEST, {
        r: { ...body, matcher: { kind: 'STRING', value: 'q' } },
        k: 'RESPONSE',
        raw: resp,
      })
    ).data!.testMatchReplace;
    expect(same).toMatchObject({ output: resp, changed: false });

    expect(code(await m(TEST, { r: body, k: 'REQUEST', raw: req }))).toBe('BAD_INPUT');
    expect(code(await m(TEST, { r: body, k: 'RESPONSE', raw: btoa('garbage') }))).toBe('BAD_INPUT');
  });
});
