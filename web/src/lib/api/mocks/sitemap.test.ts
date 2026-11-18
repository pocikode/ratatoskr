import { describe, expect, it } from 'vitest';
import { createMockClient } from './exchange';
import { createHistoryMock, makeRequest } from './history';
import { createSitemapMock } from './sitemap';

const client = () => createMockClient({ count: 300, liveIntervalMs: 0 });
const FIELDS = 'id parentId kind label url requestCount hasChildren query';
const nodes = async (c: ReturnType<typeof client>, args = '') =>
  (await c.query(`{ sitemapNodes${args} { ${FIELDS} } }`, {}).toPromise()).data.sitemapNodes as {
    id: string;
    parentId: string | null;
    kind: string;
    label: string;
    url: string;
    requestCount: number;
    hasChildren: boolean;
    query: string;
  }[];
const total = async (c: ReturnType<typeof client>, query: string) =>
  (
    await c
      .query(`query ($q: String) { requests(query: $q) { totalCount } }`, { q: query })
      .toPromise()
  ).data.requests.totalCount as number;

describe('sitemap mock', () => {
  it('counts every request once across hosts, and node queries select exactly the badge', async () => {
    const c = client();
    const hosts = await nodes(c);
    expect(hosts.every((h) => h.kind === 'HOST' && h.parentId === null)).toBe(true);
    expect(hosts.reduce((n, h) => n + h.requestCount, 0)).toBe(await total(c, ''));
    expect(hosts[0]!.label).toMatch(/^https?:\/\/[^/]+:\d+$/);

    const all = await nodes(c, '(recursive: true)');
    for (const n of all.slice(0, 40)) {
      expect(await total(c, `(${n.query})`)).toBe(n.requestCount);
    }
    const q = all.find((n) => n.kind === 'QUERY');
    expect(q?.label).toBe('?page&sort');
    expect(all.find((n) => n.id === q!.parentId)?.kind).toMatch(/FILE|DIRECTORY/);
  });

  it('loads children lazily and combines node and bar queries', async () => {
    const c = client();
    const [host] = await nodes(c);
    const kids = await nodes(c, `(parentId: ${JSON.stringify(host!.id)})`);
    expect(kids.length).toBeGreaterThan(0);
    expect(kids.every((k) => k.parentId === host!.id && k.url.startsWith(host!.url))).toBe(true);

    const filtered = await nodes(c, '(query: "\\"static\\"")');
    const n = filtered[0]!;
    expect(await total(c, `(${n.query}) and ("static")`)).toBe(n.requestCount);
  });

  it('uses the node query alone for an empty bar and rejects an empty group, like HTTPQL', async () => {
    const c = client();
    const [host] = await nodes(c);
    expect(await total(c, host!.query)).toBe(host!.requestCount);
    const res = await c
      .query(`query ($q: String) { requests(query: $q) { totalCount } }`, {
        q: `(${host!.query}) and ()`,
      })
      .toPromise();
    expect(res.error?.graphQLErrors[0]?.extensions.code).toBe('BAD_QUERY');
  });

  it('keeps requests without path segments on the host, query nodes only under path nodes', () => {
    const h = createHistoryMock({ count: 0, liveIntervalMs: 0 });
    const row = (i: number, path: string, query: string) => ({
      ...makeRequest(i),
      host: 'example.test',
      port: 443,
      tls: true,
      path,
      query,
    });
    const rows = [row(1, '/', 'x=1'), row(2, '*', 'x=2'), row(3, '/a', 'x=3')];
    const { sitemapNodes } = createSitemapMock(rows, h.matcher).root;
    const all = sitemapNodes({ recursive: true });
    expect(all.map((n) => [n.kind, n.label, n.requestCount])).toEqual([
      ['HOST', 'https://example.test:443', 3],
      ['FILE', 'a', 1],
      ['QUERY', '?x', 1],
    ]);
  });

  it('rejects a malformed parentId but returns [] for a well-formed absent node', async () => {
    const c = client();
    for (const id of [
      'nope',
      '[]',
      '[1]',
      '[true,2,{}]',
      '[true,"h",1,[]]',
      '[true,"h",1,["a"],[1]]',
    ]) {
      const res = await c
        .query(`query ($id: ID) { sitemapNodes(parentId: $id) { id } }`, { id })
        .toPromise();
      expect(res.error?.graphQLErrors[0]?.extensions.code, id).toBe('BAD_INPUT');
    }
    expect(await nodes(c, `(parentId: ${JSON.stringify('[true,"gone.test",1,["a"]]')})`)).toEqual(
      [],
    );
  });
});
