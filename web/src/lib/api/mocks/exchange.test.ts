import { describe, expect, it } from 'vitest';
import { pipe, subscribe, take, toPromise } from 'wonka';
import { createMockClient } from './exchange';

const REQUESTS = `query ($first: Int, $after: String, $query: String, $sort: RequestSort) {
  requests(first: $first, after: $after, query: $query, sort: $sort) {
    totalCount
    pageInfo { hasNextPage endCursor }
    edges { cursor node { id host method path color editedFrom { id } response { code } } }
  }
}`;

describe('mockHistoryExchange', () => {
  it('pages through requests with the real schema', async () => {
    const client = createMockClient({ count: 30, liveIntervalMs: 0 });
    const p1 = await client.query(REQUESTS, { first: 10 }).toPromise();
    expect(p1.error).toBeUndefined();
    const conn = p1.data.requests;
    expect(conn.totalCount).toBe(30);
    expect(conn.edges.map((e: { node: { id: string } }) => e.node.id)).toEqual(
      Array.from({ length: 10 }, (_, k) => String(k + 1)),
    );
    expect(conn.pageInfo.hasNextPage).toBe(true);

    const p3 = await client.query(REQUESTS, { first: 10, after: '19' }).toPromise();
    expect(p3.data.requests.pageInfo.hasNextPage).toBe(false);
    expect(p3.data.requests.edges.at(-1).node.id).toBe('30');
    // Row 25 is a seeded edited copy of row 24.
    expect(p3.data.requests.edges[4].node.editedFrom).toEqual({ id: '24' });
  });

  it('sorts descending and filters by literal', async () => {
    const client = createMockClient({ count: 50, liveIntervalMs: 0 });
    const desc = await client
      .query(REQUESTS, { first: 1, sort: { field: 'ID', direction: 'DESC' } })
      .toPromise();
    expect(desc.data.requests.edges[0].node.id).toBe('50');

    const host = await client
      .query(REQUESTS, { first: 100, query: 'req.host.eq:"cdn.example.net"' })
      .toPromise();
    for (const e of host.data.requests.edges) expect(e.node.host).toBe('cdn.example.net');
  });

  it('returns body chunks as base64', async () => {
    const client = createMockClient({ count: 5, liveIntervalMs: 0 });
    const res = await client
      .query(`{ request(id: "1") { head bodyLength body(offset: 0, limit: 4) } }`, {})
      .toPromise();
    expect(res.error).toBeUndefined();
    expect(atob(res.data.request.head)).toMatch(/^[A-Z]+ \/\S* HTTP\/1\.1\r\nHost: /);
    expect(atob(res.data.request.body).length).toBeLessThanOrEqual(4);
  });

  it('pushes colour changes to requestUpdated and reports NOT_FOUND', async () => {
    const client = createMockClient({ count: 5, liveIntervalMs: 0 });
    const update = pipe(
      client.subscription(`subscription { requestUpdated { matches request { id color } } }`, {}),
      take(1),
      toPromise,
    );
    await Promise.resolve();
    const set = await client
      .mutation(`mutation { setRequestColor(ids: ["2"], color: RED) { id color } }`, {})
      .toPromise();
    expect(set.data.setRequestColor).toEqual([{ id: '2', color: 'RED' }]);
    const ev = await update;
    expect(ev.data.requestUpdated.request).toMatchObject({ id: '2', color: 'RED' });
    expect(ev.data.requestUpdated.matches).toBe(true);

    const missing = await client
      .mutation(`mutation { setRequestNote(id: "999", note: "x") { id } }`, {})
      .toPromise();
    expect(missing.error?.graphQLErrors[0]?.extensions.code).toBe('NOT_FOUND');
  });

  it('deletes by ids and emits requestsDeleted', async () => {
    const client = createMockClient({ count: 10, liveIntervalMs: 0 });
    const deleted = pipe(
      client.subscription(`subscription { requestsDeleted { ids } }`, {}),
      take(1),
      toPromise,
    );
    await Promise.resolve();
    const res = await client
      .mutation(`mutation { deleteRequests(ids: ["3", "4", "404"]) }`, {})
      .toPromise();
    expect(res.data.deleteRequests).toBe('2');
    expect((await deleted).data.requestsDeleted.ids).toEqual(['3', '4']);
    const left = await client.query(REQUESTS, { first: 100 }).toPromise();
    expect(left.data.requests.totalCount).toBe(8);
  });

  it('streams live requestAdded events until unsubscribed', async () => {
    const client = createMockClient({ count: 3, liveIntervalMs: 5 });
    const ids: string[] = [];
    const { unsubscribe } = pipe(
      client.subscription(`subscription { requestAdded { id } }`, {}),
      subscribe((r) => ids.push(r.data.requestAdded.id)),
    );
    await new Promise((r) => setTimeout(r, 40));
    unsubscribe();
    expect(ids.slice(0, 2)).toEqual(['4', '5']);
    const seen = ids.length;
    await new Promise((r) => setTimeout(r, 30));
    expect(ids.length).toBe(seen);
  });

  it('completes the token at the cursor', async () => {
    const client = createMockClient({ liveIntervalMs: 0 });
    const res = await client
      .query(
        `{ queryCompletions(language: HTTPQL, text: "req.ho", cursor: 6) { from to items { label kind } } }`,
        {},
      )
      .toPromise();
    expect(res.data.queryCompletions).toMatchObject({
      from: 4,
      to: 6,
      items: [{ label: 'host', kind: 'FIELD' }],
    });
  });
});
