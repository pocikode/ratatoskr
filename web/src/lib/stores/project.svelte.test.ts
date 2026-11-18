import { makeSubject } from 'wonka';
import { describe, expect, it, onTestFinished, vi } from 'vitest';
import { ProjectStore } from './project.svelte';

function fake() {
  const subjects: ReturnType<typeof makeSubject<unknown>>[] = [];
  const query = vi.fn(() => ({ toPromise: async () => ({ data: { projects: [] } }) }));
  const client = {
    query,
    subscription: () => {
      const s = makeSubject<unknown>();
      subjects.push(s);
      return s.source;
    },
  };
  let connected = () => {};
  const ws = { on: (_: string, cb: () => void) => ((connected = cb), vi.fn()) };
  const store = new ProjectStore(
    () => client as never,
    () => ws as never,
  );
  return { store, subjects, query, connect: () => connected() };
}

const ev = (kind: string) => ({ data: { projectChanged: { kind } } });

describe('ProjectStore subscription', () => {
  it('SELECTED bumps epoch and runs resets; other kinds only refetch', async () => {
    const { store, subjects, query } = fake();
    const reset = vi.fn();
    store.onReset(reset);
    store.start();
    await vi.waitFor(() => expect(query).toHaveBeenCalledTimes(1));

    subjects[0]!.next(ev('UPDATED'));
    expect(store.epoch).toBe(0);
    expect(reset).not.toHaveBeenCalled();
    subjects[0]!.next(ev('SELECTED'));
    expect(store.epoch).toBe(1);
    expect(reset).toHaveBeenCalledTimes(1);
    await vi.waitFor(() => expect(query).toHaveBeenCalledTimes(3));
  });

  it('resubscribes and refetches when the stream completes, and cleans up', async () => {
    vi.useFakeTimers();
    onTestFinished(() => {
      vi.useRealTimers();
    });
    const { store, subjects, query } = fake();
    const stop = store.start();
    subjects[0]!.complete();
    expect(subjects).toHaveLength(1); // delayed, not immediate
    vi.advanceTimersByTime(1000);
    expect(subjects).toHaveLength(2);
    await vi.waitFor(() => expect(query).toHaveBeenCalledTimes(2));
    stop();
    subjects[1]!.complete();
    vi.advanceTimersByTime(2000);
    expect(subjects).toHaveLength(2);
  });

  it('refetches after a socket reconnect but not on first connect', async () => {
    const { store, query, connect } = fake();
    store.start();
    connect();
    await Promise.resolve();
    expect(query).toHaveBeenCalledTimes(1);
    connect();
    await vi.waitFor(() => expect(query).toHaveBeenCalledTimes(2));
  });

  it('ignores a stale slower refresh', async () => {
    let first: (v: unknown) => void = () => {};
    let n = 0;
    const client = {
      query: () => ({
        toPromise: () =>
          ++n === 1
            ? new Promise((r) => (first = r))
            : Promise.resolve({ data: { projects: [{ id: 'new' }] } }),
      }),
    };
    const store = new ProjectStore(
      () => client as never,
      () => undefined,
    );
    const a = store.refresh();
    await store.refresh();
    first({ data: { projects: [{ id: 'old' }] } });
    await a;
    expect(store.projects.map((p) => p.id)).toEqual(['new']);
  });
});
