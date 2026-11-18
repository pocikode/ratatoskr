import { describe, expect, it } from 'vitest';
import { createMockClient } from './exchange';

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

  it('rejects a malformed parentId', async () => {
    const res = await client().query(`{ sitemapNodes(parentId: "nope") { id } }`, {}).toPromise();
    expect(res.error?.graphQLErrors[0]?.extensions.code).toBe('BAD_INPUT');
  });
});
