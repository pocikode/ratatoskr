import { Client, fetchExchange, subscriptionExchange } from '@urql/core';
import { fromPromise, makeSubject } from 'wonka';
import { describe, expect, it, vi } from 'vitest';
import {
  InterceptQueueChangedDocument,
  InterceptQueueDocument,
  InterceptSettingsDocument,
  UpdateInterceptSettingsDocument,
  type InterceptItemFieldsFragment,
  type InterceptSettingsFieldsFragment,
} from '$lib/api/generated/graphql';
import { createMockClient } from '$lib/api/mocks/exchange';
import { fromBase64, InterceptStore, toBase64 } from './intercept.svelte';

function setup() {
  const client = createMockClient();
  const resets: (() => void)[] = [];
  const store = new InterceptStore(
    () => client,
    () => undefined,
    { onReset: (fn) => (resets.push(fn), () => {}) },
  );
  return { client, store, resets };
}

describe('InterceptStore', () => {
  it('loads settings and the queue in FIFO order', async () => {
    const { store } = setup();
    const stop = store.start();
    await vi.waitFor(() => expect(store.count).toBe(3));
    expect(store.settings?.queuing).toBe(true);
    expect(store.of('REQUEST')).toHaveLength(2);
    expect(store.of('RESPONSE')).toHaveLength(1);
    stop();
  });

  it('follows queue changes and settings made by another client', async () => {
    const { client, store } = setup();
    const other = new InterceptStore(() => client);
    const stop = store.start();
    await vi.waitFor(() => expect(store.count).toBe(3));

    const first = store.queue[0]!;
    await other.drop(first.id);
    await vi.waitFor(() => expect(store.queue.some((i) => i.id === first.id)).toBe(false));

    await other.update({ queuing: false });
    await vi.waitFor(() => expect(store.settings?.queuing).toBe(false));
    await vi.waitFor(() => expect(store.count).toBe(0));
    stop();
  });

  it('refetches after a project switch', async () => {
    const { store, resets } = setup();
    const stop = store.start();
    await vi.waitFor(() => expect(store.count).toBe(3));
    resets.forEach((fn) => fn());
    expect(store.count).toBe(0);
    await vi.waitFor(() => expect(store.count).toBe(3));
    stop();
  });

  it('reports a BAD_QUERY span and BAD_INPUT message', async () => {
    const { store } = setup();
    const bad = await store.update({ query: 'req.host.eq:"x' });
    expect(bad.span).toMatchObject({ start: expect.any(Number) });
    const input = await store.update({ dropStatusCode: 700 });
    expect(input.error).toMatch(/100-599/);
  });

  it('does nothing when flipping before the settings are known', async () => {
    const { store } = setup();
    expect(await store.flip('queuing')).toEqual({ error: null });
    expect(store.settings).toBeNull();
  });

  it('round-trips base64', () => {
    const bytes = Uint8Array.from({ length: 70000 }, (_, i) => i % 256);
    expect(fromBase64(toBase64(bytes))).toEqual(bytes);
  });
});

// Out-of-order delivery: subscription events and mutation results racing the refetch snapshots.
const item = (id: string): InterceptItemFieldsFragment => ({
  id,
  kind: 'REQUEST',
  raw: '',
  heldAt: '2026-11-20T10:00:00Z',
  request: { id, method: 'GET', host: 'example.test', port: 80, tls: false, path: '/', query: '' },
});
const settings = (queuing: boolean): InterceptSettingsFieldsFragment => ({
  queuing,
  requests: true,
  responses: false,
  query: '',
  scopeId: null,
  dropStatusCode: 502,
});

function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((r) => (resolve = r));
  return { promise, resolve };
}

function manual() {
  const queryDocs = {
    queue: [deferred<{ data: unknown }>(), deferred<{ data: unknown }>()],
    settings: [deferred<{ data: unknown }>(), deferred<{ data: unknown }>()],
  };
  const calls = { queue: 0, settings: 0 };
  const queueEvents = makeSubject<{ data: unknown }>();
  const settingsEvents = makeSubject<{ data: unknown }>();
  const mutation = deferred<{ data: unknown }>();
  const client = {
    query: (doc: unknown) =>
      fromPromise(
        doc === InterceptQueueDocument
          ? queryDocs.queue[calls.queue++]!.promise
          : doc === InterceptSettingsDocument
            ? queryDocs.settings[calls.settings++]!.promise
            : Promise.resolve({ data: null }),
      ),
    subscription: (doc: unknown) =>
      doc === InterceptQueueChangedDocument ? queueEvents.source : settingsEvents.source,
    mutation: (doc: unknown) => ({
      toPromise: () => (doc === UpdateInterceptSettingsDocument ? mutation.promise : null),
    }),
  } as unknown as Client;
  let reset = () => {};
  const store = new InterceptStore(
    () => client,
    () => undefined,
    { onReset: (fn) => ((reset = fn), () => {}) },
  );
  const stop = store.start();
  return {
    store,
    stop,
    queryDocs,
    mutation,
    reset: () => reset(),
    queueEvent: (kind: 'ADDED' | 'REMOVED', i: InterceptItemFieldsFragment) =>
      queueEvents.next({ data: { interceptQueueChanged: { kind, item: i } } }),
    settingsEvent: (s: InterceptSettingsFieldsFragment) =>
      settingsEvents.next({ data: { interceptSettingsChanged: s } }),
  };
}

const tick = () => new Promise((r) => setTimeout(r, 0));

describe('InterceptStore with the real urql exchanges', () => {
  it('does not let a project switch share the old project’s in-flight queries', async () => {
    const pending: { op: string; respond: (data: unknown) => void }[] = [];
    const fetchImpl = vi.fn(
      (_url: unknown, init?: RequestInit) =>
        new Promise<Response>((resolve) => {
          const { query } = JSON.parse(String(init?.body)) as { query: string };
          const op = query.includes('InterceptQueue') ? 'queue' : 'settings';
          pending.push({
            op,
            respond: (data) =>
              resolve(
                new Response(JSON.stringify({ data }), {
                  headers: { 'content-type': 'application/json' },
                }),
              ),
          });
        }),
    );
    const client = new Client({
      url: 'http://localhost/graphql',
      preferGetMethod: false, // as the app's client: the server only serves POST
      fetch: fetchImpl as unknown as typeof fetch,
      exchanges: [
        fetchExchange,
        subscriptionExchange({
          forwardSubscription: () => ({ subscribe: () => ({ unsubscribe() {} }) }),
        }),
      ],
    });
    let reset = () => {};
    const store = new InterceptStore(
      () => client,
      () => undefined,
      { onReset: (fn) => ((reset = fn), () => {}) },
    );
    const stop = store.start();
    await vi.waitFor(() => expect(pending).toHaveLength(2)); // old project: settings + queue

    reset();
    // Torn down: the refetch issues its own requests instead of joining the old ones.
    await vi.waitFor(() => expect(pending).toHaveLength(4));

    const answer = (index: number, queuing: boolean, id: string) =>
      pending[index]!.respond(
        pending[index]!.op === 'queue'
          ? { interceptQueue: [{ ...item(id), __typename: 'InterceptItem' }] }
          : { interceptSettings: { ...settings(queuing), __typename: 'InterceptSettings' } },
      );
    answer(0, true, 'old');
    answer(1, true, 'old');
    await tick();
    expect(store.settings).toBeNull();
    expect(store.queue).toEqual([]);

    answer(2, false, 'new');
    answer(3, false, 'new');
    await vi.waitFor(() => expect(store.settings?.queuing).toBe(false));
    expect(store.queue.map((i) => i.id)).toEqual(['new']);
    stop();
  });
});

describe('InterceptStore ordering', () => {
  it('keeps an item that was added while the queue snapshot was in flight', async () => {
    const t = manual();
    t.queueEvent('ADDED', item('c'));
    t.queryDocs.settings[0]!.resolve({ data: { interceptSettings: settings(true) } });
    t.queryDocs.queue[0]!.resolve({ data: { interceptQueue: [item('a'), item('b')] } });
    await tick();
    expect(t.store.queue.map((i) => i.id)).toEqual(['a', 'b', 'c']);
    t.stop();
  });

  it('keeps an item gone that was removed while the queue snapshot was in flight', async () => {
    const t = manual();
    t.queueEvent('ADDED', item('a'));
    t.queueEvent('REMOVED', item('a'));
    t.queryDocs.settings[0]!.resolve({ data: { interceptSettings: settings(true) } });
    t.queryDocs.queue[0]!.resolve({ data: { interceptQueue: [item('a'), item('b')] } });
    await tick();
    expect(t.store.queue.map((i) => i.id)).toEqual(['b']);
    t.stop();
  });

  it('does not let an older settings snapshot overwrite a newer event', async () => {
    const t = manual();
    t.settingsEvent(settings(false));
    t.queryDocs.settings[0]!.resolve({ data: { interceptSettings: settings(true) } });
    t.queryDocs.queue[0]!.resolve({ data: { interceptQueue: [] } });
    await tick();
    expect(t.store.settings?.queuing).toBe(false);
    t.stop();
  });

  it('does not let an older mutation result overwrite a newer event', async () => {
    const t = manual();
    t.queryDocs.settings[0]!.resolve({ data: { interceptSettings: settings(true) } });
    t.queryDocs.queue[0]!.resolve({ data: { interceptQueue: [] } });
    await tick();
    const pending = t.store.update({ queuing: false });
    t.settingsEvent({ ...settings(true), requests: false });
    t.mutation.resolve({ data: { updateInterceptSettings: settings(false) } });
    await pending;
    expect(t.store.settings).toMatchObject({ queuing: true, requests: false });
    t.stop();
  });

  it('clears settings on a project switch and ignores the old project in flight', async () => {
    const t = manual();
    t.queryDocs.settings[0]!.resolve({ data: { interceptSettings: settings(true) } });
    t.queryDocs.queue[0]!.resolve({ data: { interceptQueue: [item('a')] } });
    await tick();
    const pending = t.store.update({ queuing: true });

    t.reset();
    expect(t.store.settings).toBeNull();
    expect(t.store.queue).toEqual([]);
    expect(t.store.epoch).toBe(1);
    // The old project's mutation result must not come back as the new project's settings.
    t.mutation.resolve({ data: { updateInterceptSettings: settings(true) } });
    await pending;
    expect(t.store.settings).toBeNull();
    // Nothing is flipped from stale settings either.
    expect(await t.store.flip('queuing')).toEqual({ error: null });

    t.queryDocs.settings[1]!.resolve({ data: { interceptSettings: settings(false) } });
    t.queryDocs.queue[1]!.resolve({ data: { interceptQueue: [] } });
    await tick();
    expect(t.store.settings?.queuing).toBe(false);
    t.stop();
  });
});
