import { Client, makeResult, type Exchange } from '@urql/core';
import { GraphQLError } from 'graphql';
import { filter, fromValue, mergeMap, pipe } from 'wonka';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createMockClient } from '$lib/api/mocks/exchange';
import { historyCommands } from './commands';
import { HistoryState } from './history.svelte';
import { loadColumnConfig } from './ui-state';

/** A client whose queries fail with a server error; subscriptions stay silent. */
function failingClient(error: GraphQLError): Client {
  const exchange: Exchange = () => (ops$) =>
    pipe(
      ops$,
      filter((op) => op.kind === 'query'),
      mergeMap((op) => fromValue(makeResult(op, { errors: [error] }))),
    );
  return new Client({ url: '/test', exchanges: [exchange] });
}

const mock = (count = 40) => createMockClient({ count, liveIntervalMs: 0 });

beforeEach(() => localStorage.clear());

describe('HistoryState', () => {
  it('loads the first page and filters on the applied query', async () => {
    const state = new HistoryState(mock());
    const stop = state.start();

    await vi.waitFor(() => expect(state.loading).toBe(false));
    expect(state.rows).toHaveLength(40);
    expect(state.total).toBe(40);
    expect(state.rows[0]?.id).toBe('40'); // default sort: newest first

    state.apply('req.host.cont:"api"');
    await vi.waitFor(() => expect(state.rows.length).toBeLessThan(40));

    expect(state.applied).toBe('req.host.cont:"api"');
    expect(state.history).toEqual(['req.host.cont:"api"']);
    // The mock matches the literal against method, host, path and query (its documented shortcut).
    expect(
      state.rows.every((row) =>
        `${row.method} ${row.host}${row.path}?${row.query}`.includes('api'),
      ),
    ).toBe(true);
    stop();
  });

  it('keeps the rows when the query does not parse', async () => {
    const state = new HistoryState(mock());
    const stop = state.start();
    await vi.waitFor(() => expect(state.rows).toHaveLength(40));

    state.apply('req.host.eq:"api');

    expect(state.error).toMatchObject({ message: 'Unterminated string', from: 12, to: 16 });
    expect(state.rows).toHaveLength(40); // no clearing (T-017 AC)
    expect(state.applied).toBe(''); // the bad query was not applied
    stop();
  });

  it('shows the server BAD_QUERY span inline', async () => {
    const state = new HistoryState(
      failingClient(
        new GraphQLError('resp.code.cont is a type error', {
          extensions: { code: 'BAD_QUERY', span: { start: 5, end: 12 } },
        }),
      ),
    );
    const stop = state.start();
    state.apply('resp.code.cont:"2"');

    await vi.waitFor(() =>
      expect(state.error).toEqual({ message: 'resp.code.cont is a type error', from: 5, to: 12 }),
    );

    expect(state.applied).toBe(''); // rolled back: a later sort must not resend it
    stop();
  });

  it('keeps the in-flight query when the sort changes before it resolves', async () => {
    const state = new HistoryState(mock());
    const stop = state.start();
    await vi.waitFor(() => expect(state.rows).toHaveLength(40));

    state.apply('req.host.cont:"api"');
    state.setSort({ columnId: 'id', direction: 'asc' });

    await vi.waitFor(() => expect(state.loading).toBe(false));
    expect(state.applied).toBe('req.host.cont:"api"');
    expect(state.rows.length).toBeLessThan(40);
    stop();
  });

  it('"Filter by this host" drops a trailing // comment before wrapping', async () => {
    const state = new HistoryState(mock());
    const stop = state.start();
    await vi.waitFor(() => expect(state.rows).toHaveLength(40));
    state.apply('req.method.eq:"GET" // x');
    await vi.waitFor(() => expect(state.applied).toBe('req.method.eq:"GET" // x'));
    state.selection.click(state.rows[0]!.id, [state.rows[0]!.id]);

    const command = historyCommands(state, { copy: () => {}, editNote: () => {} }).find(
      (c) => c.id === 'history.filter.host',
    );
    command?.run({} as never);

    expect(state.query).toMatch(/^\(req\.method\.eq:"GET"\) AND req\.host\.eq:"/);
    stop();
  });

  it('holds live rows behind the pill while a row is selected', async () => {
    const state = new HistoryState(createMockClient({ count: 10, liveIntervalMs: 15 }));
    // Select before the first event can arrive; the selection only needs a key.
    state.selection.click('1', ['1']);
    const stop = state.start();
    await vi.waitFor(() => expect(state.pending.length).toBeGreaterThan(0));

    expect(state.rows).toHaveLength(10);

    const buffered = state.pending.length;
    state.applyPending();

    expect(state.pending).toHaveLength(0);
    expect(state.rows).toHaveLength(10 + buffered);
    expect(Number(state.rows[0]?.id)).toBeGreaterThan(10); // newest first
    stop();
  });

  it('inserts live rows at the top while nothing is selected', async () => {
    const state = new HistoryState(createMockClient({ count: 5, liveIntervalMs: 15 }));
    const stop = state.start();

    await vi.waitFor(() => expect(state.rows.length).toBeGreaterThan(5));

    expect(state.pending).toHaveLength(0);
    expect(Number(state.rows[0]?.id)).toBeGreaterThan(5);
    stop();
  });

  it('sets colours and trims notes', async () => {
    const state = new HistoryState(mock(5));
    const stop = state.start();
    await vi.waitFor(() => expect(state.rows).toHaveLength(5));
    const id = state.rows[0]!.id;

    await state.setColor([id], 'RED');
    expect(state.rows.find((row) => row.id === id)?.color).toBe('RED');

    await state.setNote(id, '  needs a second look ');
    expect(state.rows.find((row) => row.id === id)?.note).toBe('needs a second look');

    await state.setNote(id, '   ');
    expect(state.rows.find((row) => row.id === id)?.note).toBeNull();

    await state.setColor([id], null);
    expect(state.rows.find((row) => row.id === id)?.color).toBeNull();
    stop();
  });

  it('drops deleted rows and keeps the total in step', async () => {
    const state = new HistoryState(mock(5));
    const stop = state.start();
    await vi.waitFor(() => expect(state.rows).toHaveLength(5));
    const id = state.rows[1]!.id;

    state.selection.click(
      id,
      state.rows.map((row) => row.id),
    );
    await state.remove([id]);

    expect(state.rows.map((row) => row.id)).not.toContain(id);
    expect(state.selection.size).toBe(0);
    expect(state.total).toBe(4);
    stop();
  });

  it('persists the column layout', () => {
    const state = new HistoryState(mock());

    state.setConfig({ order: ['id', 'host'], hidden: ['host'], widths: { id: 90 } });

    expect(state.config).toEqual({ order: ['id', 'host'], hidden: ['host'], widths: { id: 90 } });
    expect(loadColumnConfig()).toEqual({
      order: ['id', 'host'],
      hidden: ['host'],
      widths: { id: 90 },
    });
  });

  it('fetches the selected message and ignores superseded selections', async () => {
    const state = new HistoryState(mock(5));
    const stop = state.start();
    await vi.waitFor(() => expect(state.rows).toHaveLength(5));
    const first = state.rows[0]!;
    const second = state.rows[1]!;

    state.selection.click(first.id, [first.id, second.id]);
    void state.showDetail(first.id);
    state.selection.click(second.id, [first.id, second.id]);
    await state.showDetail(second.id);

    expect(state.detail?.id).toBe(second.id);
    expect(state.detail?.request).toContain(`${second.method} `);
    expect(state.detail?.request).toContain(`Host: ${second.host}`);
    stop();
  });
});
