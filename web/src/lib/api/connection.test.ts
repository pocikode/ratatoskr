import { describe, expect, it, vi } from 'vitest';
import type { Api } from './client';
import { ConnectionState } from './connection.svelte';

type Listener = (arg?: unknown) => void;

const SERVER_INFO = {
  version: 'v0.1.0',
  commit: 'abc123',
  goVersion: 'go1.27.4',
  startedAt: '2026-11-18T00:00:00Z',
  authRequired: false,
};

/** A fake client pair: ws events are fired by hand, ping results pushed by hand. */
function fakeApi(queryResult: unknown = { data: { serverInfo: SERVER_INFO } }) {
  const listeners = new Map<string, Set<Listener>>();
  let pingSink: ((r: unknown) => void) | undefined;
  const unsubscribe = vi.fn();
  const api = {
    ws: {
      on: (event: string, l: Listener) => {
        const set = listeners.get(event) ?? new Set();
        set.add(l);
        listeners.set(event, set);
        return () => set.delete(l);
      },
    },
    client: {
      subscription: () => ({
        subscribe: (sink: (r: unknown) => void) => {
          pingSink = sink;
          return { unsubscribe };
        },
      }),
      query: () => ({ toPromise: async () => queryResult }),
    },
  } as unknown as Api;
  return {
    api,
    unsubscribe,
    emit: (event: string, arg?: unknown) => listeners.get(event)?.forEach((l) => l(arg)),
    ping: (at: string) => pingSink?.({ data: { ping: { seq: '1', at } } }),
    listenerCount: () => [...listeners.values()].reduce((n, s) => n + s.size, 0),
  };
}

describe('ConnectionState', () => {
  it('follows the socket lifecycle and loads server info on connect', async () => {
    const fake = fakeApi();
    const conn = new ConnectionState();
    conn.start(fake.api);
    expect(conn.status).toBe('connecting');

    fake.emit('connected');
    expect(conn.status).toBe('connected');
    await vi.waitFor(() => expect(conn.server).toEqual(SERVER_INFO));

    fake.ping('2026-11-18T00:00:01Z');
    expect(conn.lastPing).toBe('2026-11-18T00:00:01Z');

    fake.emit('closed', { code: 1006 });
    expect(conn.status).toBe('disconnected');
    fake.emit('connecting');
    expect(conn.status).toBe('connecting');
  });

  it('reports a rejected token as unauthorized and keeps it while retrying', () => {
    const fake = fakeApi();
    const conn = new ConnectionState();
    conn.start(fake.api);

    fake.emit('closed', { code: 4401 });
    expect(conn.status).toBe('unauthorized');
    fake.emit('connecting');
    expect(conn.status).toBe('unauthorized');
  });

  it('reports an HTTP 401 on the server info query as unauthorized', async () => {
    const fake = fakeApi({ error: { response: { status: 401 } } });
    const conn = new ConnectionState();
    conn.start(fake.api);

    fake.emit('connected');
    await vi.waitFor(() => expect(conn.status).toBe('unauthorized'));
  });

  it('cleans up the subscription and listeners', () => {
    const fake = fakeApi();
    const stop = new ConnectionState().start(fake.api);
    expect(fake.listenerCount()).toBe(3);

    stop();
    expect(fake.unsubscribe).toHaveBeenCalledOnce();
    expect(fake.listenerCount()).toBe(0);
  });
});
