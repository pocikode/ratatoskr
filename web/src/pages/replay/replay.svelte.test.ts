import type { Client } from '@urql/core';
import { beforeEach, describe, expect, it } from 'vitest';
import { ReplayState, type Collection } from './replay.svelte';

const s = (id: string, name = id) =>
  ({
    id,
    name,
    color: null,
    connection: { host: 'h', port: 80, tls: false },
    draft: btoa(`GET /${id} HTTP/1.1\r\n\r\n`),
  }) as Collection['sessions'][number];
const col = (id: string, ...ids: string[]): Collection => ({
  id,
  name: id,
  isDefault: id === 'a',
  sessions: ids.map((i) => s(i)),
});

/** A fake urql client: `fail` makes every mutation (and, with `failQuery`, the refetch) error. */
function fake(tree: Collection[], opts: { fail?: boolean; failQuery?: boolean } = {}) {
  const calls: { name: string; vars: Record<string, unknown> }[] = [];
  const result = (data: unknown, err: boolean) => ({
    toPromise: async () => (err ? { error: { message: 'down', graphQLErrors: [] } } : { data }),
  });
  const client = {
    query: () => result({ replayCollections: tree }, !!opts.failQuery),
    mutation: (
      doc: { definitions: { name?: { value: string } }[] },
      vars: Record<string, unknown>,
    ) => {
      calls.push({ name: doc.definitions[0]!.name?.value ?? '', vars });
      return result({ createReplaySession: { id: 'new' } }, !!opts.fail);
    },
  } as unknown as Client;
  return { client, calls };
}

beforeEach(() => localStorage.clear());

describe('ReplayState.move', () => {
  it('translates a filtered drop into the full-collection index', async () => {
    const { client, calls } = fake([col('a', 'hidden', 'match-b', 'match-a')]);
    const st = new ReplayState(client, 'p');
    await st.load();
    st.search = 'match';
    // visible rest of match-a = [match-b]; dropping before it is visible index 0 → full index 1.
    await st.move('match-a', 'a', 0);
    expect(calls.at(-1)?.vars).toMatchObject({ id: 'match-a', collectionId: 'a', index: 1 });
    // Append (group header) goes after every session, hidden or not.
    await st.move('match-b', 'a', 99);
    expect(calls.at(-1)?.vars).toMatchObject({ index: 2 });
  });

  it('restores the previous tree when the mutation and the refetch both fail', async () => {
    const tree = [col('a', '1', '2'), col('b')];
    const { client } = fake(tree, { fail: true, failQuery: true });
    const st = new ReplayState(client, 'p');
    st.collections = tree;
    await st.move('1', 'b', 0);
    expect(st.collections).toBe(tree);
    expect(st.error).toBe('down');
  });
});

describe('ReplayState.move stale rollback', () => {
  it('does not overwrite a newer tree when the move fails', async () => {
    const tree = [col('a', '1', '2'), col('b')];
    const { client } = fake(tree, { fail: true, failQuery: true });
    const st = new ReplayState(client, 'p');
    st.collections = tree;
    const p = st.move('1', 'b', 0);
    const newer = [col('a', '1'), col('b')]; // e.g. session 2 was deleted meanwhile
    st.collections = newer;
    await p;
    expect(st.collections).toBe(newer);
  });
});

describe('ReplayState.filtered', () => {
  it('matches a term at the end of a long request target', async () => {
    const long = {
      ...s('1', 'renamed'),
      draft: btoa(`GET /${'x'.repeat(2000)}needle HTTP/1.1\r\n\r\n`),
    };
    const { client } = fake([{ ...col('a'), sessions: [long, s('2')] }]);
    const st = new ReplayState(client, 'p');
    await st.load();
    st.search = 'needle';
    expect(st.filtered[0]!.sessions.map((x) => x.id)).toEqual(['1']);
  });
});

describe('ReplayState.move overlap', () => {
  it('ignores a second move while one is in flight, so a rollback never restores a failed move', async () => {
    const tree = [col('a', '1', '2'), col('b')];
    const { client, calls } = fake(tree, { fail: true, failQuery: true });
    const st = new ReplayState(client, 'p');
    st.collections = tree;
    await Promise.all([st.move('1', 'b', 0), st.move('2', 'b', 0)]);
    expect(calls).toHaveLength(1);
    expect(st.collections).toBe(tree);
  });
});

describe('ReplayState.createFromUrl', () => {
  it('sends a non-Latin-1 path as UTF-8 bytes', async () => {
    const { client, calls } = fake([col('a')]);
    const st = new ReplayState(client, 'p');
    const err = await st.createFromUrl({ tls: false, host: 'x.test', port: '', path: '/東京' });
    expect(err).toBeNull();
    const raw = Uint8Array.from(atob((calls[0]!.vars.input as { raw: string }).raw), (c) =>
      c.charCodeAt(0),
    );
    expect(new TextDecoder().decode(raw)).toContain('GET /東京 HTTP/1.1');
  });
});

describe('ReplayState.createFromUrl long path', () => {
  it('encodes a 250k-character path without overflowing the call stack', async () => {
    const { client, calls } = fake([col('a')]);
    const st = new ReplayState(client, 'p');
    await st.createFromUrl({
      tls: false,
      host: 'x.test',
      port: '',
      path: '/' + 'a'.repeat(250_000),
    });
    expect(calls).toHaveLength(1);
  });
});

describe('ReplayState tabs', () => {
  it('closes others and tabs to the right', async () => {
    const { client } = fake([col('a', '1', '2', '3', '4')]);
    const st = new ReplayState(client, 'p');
    await st.load();
    for (const id of ['1', '2', '3', '4']) await st.open(id);
    st.closeToRight('2');
    expect(st.openIds).toEqual(['1', '2']);
    expect(st.activeId).toBe('2');
    st.closeOthers('1');
    expect(st.openIds).toEqual(['1']);
    expect(st.activeId).toBe('1');
  });

  it('a slow lookup of an unknown session does not override a newer selection', async () => {
    const { client } = fake([col('a', '1')]);
    const st = new ReplayState(client, 'p');
    await st.load();
    // The refetch for `x` is held until `1` has been selected; by then `x` exists on the server.
    let release = () => {};
    const gate = new Promise<void>((r) => (release = r));
    client.query = (() => ({
      toPromise: async () => {
        await gate;
        return { data: { replayCollections: [col('a', '1', 'x')] } };
      },
    })) as unknown as Client['query'];
    const link = st.open('x');
    await st.open('1');
    release();
    await link;
    expect(st.activeId).toBe('1');
    expect(st.openIds).toEqual(['1']);
  });
});
