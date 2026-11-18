import { describe, expect, it } from 'vitest';
import { createMockClient } from './exchange';

const client = () => createMockClient({ count: 1, liveIntervalMs: 0 });
const code = (res: { error?: { graphQLErrors: { extensions: Record<string, unknown> }[] } }) =>
  res.error?.graphQLErrors[0]?.extensions.code;

const SCOPES = `{ scopePresets { id name allow deny } }`;
const FILTERS = `{ filterPresets { id name alias query } }`;

describe('scope preset mock', () => {
  it('lists, creates, duplicates, reorders and deletes', async () => {
    const c = client();
    const list = (await c.query(SCOPES, {}).toPromise()).data.scopePresets;
    expect(list.length).toBe(3);

    const created = await c
      .mutation(
        `mutation { createScopePreset(name: " New ", allow: ["*.example.test:8443/api"]) { id name deny } }`,
        {},
      )
      .toPromise();
    expect(created.error).toBeUndefined();
    expect(created.data.createScopePreset).toMatchObject({ name: 'New', deny: [] });

    const dup1 = await c
      .mutation(`mutation { duplicateScopePreset(id: "1") { name } }`, {})
      .toPromise();
    const dup2 = await c
      .mutation(`mutation { duplicateScopePreset(id: "1") { name } }`, {})
      .toPromise();
    expect(dup1.data.duplicateScopePreset.name).toBe('Example app (copy)');
    expect(dup2.data.duplicateScopePreset.name).toBe('Example app (copy 2)');

    const ids = (
      await c.query(SCOPES, {}, { requestPolicy: 'network-only' }).toPromise()
    ).data.scopePresets.map((p: { id: string }) => p.id);
    const reordered = await c
      .mutation(`mutation ($ids: [ID!]!) { reorderScopePresets(ids: $ids) { id } }`, {
        ids: [...ids].reverse(),
      })
      .toPromise();
    expect(reordered.data.reorderScopePresets.map((p: { id: string }) => p.id)).toEqual(
      [...ids].reverse(),
    );

    const del = await c.mutation(`mutation { deleteScopePreset(id: "2") }`, {}).toPromise();
    expect(del.data.deleteScopePreset).toBe('2');
  });

  it('reports BAD_INPUT, CONFLICT and NOT_FOUND', async () => {
    const c = client();
    const m = (q: string, v = {}) => c.mutation(q, v).toPromise();
    expect(code(await m(`mutation { createScopePreset(name: "  ") { id } }`))).toBe('BAD_INPUT');
    expect(
      code(
        await m(`mutation { createScopePreset(name: "x", allow: ["example.com:99999"]) { id } }`),
      ),
    ).toBe('BAD_INPUT');
    expect(code(await m(`mutation { createScopePreset(name: "API only") { id } }`))).toBe(
      'CONFLICT',
    );
    expect(code(await m(`mutation { deleteScopePreset(id: "999") }`))).toBe('NOT_FOUND');
    expect(code(await m(`mutation { reorderScopePresets(ids: ["1", "1", "2"]) { id } }`))).toBe(
      'BAD_INPUT',
    );
  });
});

describe('filter preset mock', () => {
  it('creates, rejects conflicts and cycles, and reorders', async () => {
    const c = client();
    const m = (q: string, v = {}) => c.mutation(q, v).toPromise();
    expect((await c.query(FILTERS, {}).toPromise()).data.filterPresets.length).toBe(3);

    const ok = await m(
      `mutation { createFilterPreset(name: "Slow", alias: "slow", query: "resp.roundtrip.gt:1000") { id alias } }`,
    );
    expect(ok.data.createFilterPreset.alias).toBe('slow');

    expect(
      code(
        await m(
          `mutation { createFilterPreset(name: "x", alias: "Bad Alias", query: "a") { id } }`,
        ),
      ),
    ).toBe('BAD_INPUT');
    expect(
      code(
        await m(`mutation { createFilterPreset(name: "y", alias: "errors", query: "a") { id } }`),
      ),
    ).toBe('CONFLICT');

    // "Errors" (id 5) referencing "API errors" closes a cycle.
    const cyc = await m(
      `mutation { updateFilterPreset(id: "5", name: "Errors", alias: "errors", query: "preset:\\"API errors\\"") { id } }`,
    );
    expect(code(cyc)).toBe('BAD_QUERY');
    expect(cyc.error?.graphQLErrors[0]?.extensions.span).toEqual({ start: 0, end: 19 });

    const missing = await m(
      `mutation { createFilterPreset(name: "z", query: "preset:nope") { id } }`,
    );
    expect(code(missing)).toBe('BAD_QUERY');

    expect(code(await m(`mutation { reorderFilterPresets(ids: ["4"]) { id } }`))).toBe('BAD_INPUT');
  });

  it('round-trips export and import, skipping existing names', async () => {
    const c = client();
    const exported = (await c.query(`{ exportFilterPresets }`, {}).toPromise()).data
      .exportFilterPresets;
    const doc = JSON.parse(exported);
    expect(doc.version).toBe(1);
    expect(doc.presets.map((p: { name: string }) => p.name)).toEqual([
      'No assets',
      'Errors',
      'API errors',
    ]);
    expect('alias' in doc.presets[2]).toBe(false);

    const same = await c
      .mutation(`mutation ($j: String!) { importFilterPresets(json: $j) }`, { j: exported })
      .toPromise();
    expect(same.data.importFilterPresets).toBe(0);

    const file = JSON.stringify({
      version: 1,
      presets: [
        { name: 'B', query: 'preset:a' },
        { name: 'A', alias: 'a', query: 'req.method.eq:"POST"' },
      ],
    });
    const added = await c
      .mutation(`mutation ($j: String!) { importFilterPresets(json: $j) }`, { j: file })
      .toPromise();
    expect(added.data.importFilterPresets).toBe(2);

    const bad = await c
      .mutation(`mutation ($j: String!) { importFilterPresets(json: $j) }`, {
        j: '{"version":2,"presets":[]}',
      })
      .toPromise();
    expect(code(bad)).toBe('BAD_INPUT');
  });
});
