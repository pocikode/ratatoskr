import { describe, expect, it, vi } from 'vitest';
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

  it('round-trips base64', () => {
    const bytes = Uint8Array.from({ length: 70000 }, (_, i) => i % 256);
    expect(fromBase64(toBase64(bytes))).toEqual(bytes);
  });
});
