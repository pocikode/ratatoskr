import { waitFor } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { createMockClient } from '$lib/api/mocks/exchange';
import {
  HistoryDeleteRequestsDocument,
  HistorySetNoteDocument,
  SitemapNodesDocument,
  SitemapRequestsDocument,
  UpdateScopePresetDocument,
} from '$lib/api/generated/graphql';
import { SitemapState } from './sitemap.svelte';

afterEach(() => vi.useRealTimers());

describe('SitemapState', () => {
  it('refreshes roots and expanded nodes at most once a second during live capture and stops on cleanup', async () => {
    vi.useFakeTimers();
    const client = createMockClient({ count: 100, liveIntervalMs: 25 });
    const query = vi.spyOn(client, 'query');
    const state = new SitemapState(client);
    const stop = state.start();
    try {
      await vi.waitFor(() => expect(state.children['']?.length).toBeGreaterThan(0));
      const host = state.children['']!.find((node) => node.hasChildren)!;
      await state.toggle(host);
      expect(state.expanded.has(host.id)).toBe(true);
      const callsBefore = query.mock.calls.length;
      await vi.advanceTimersByTimeAsync(2500);
      const calls = query.mock.calls
        .slice(callsBefore)
        .filter(([document]) => document === SitemapNodesDocument);
      const roots = calls.filter(([, variables]) => variables?.parentId === null);
      const expanded = calls.filter(([, variables]) => variables?.parentId === host.id);
      expect(roots.length).toBeGreaterThanOrEqual(2);
      expect(roots.length).toBeLessThanOrEqual(3);
      expect(expanded).toHaveLength(roots.length);
    } finally {
      stop();
    }
    const callsAfterStop = query.mock.calls.length;
    await vi.advanceTimersByTimeAsync(2000);
    expect(query).toHaveBeenCalledTimes(callsAfterStop);
  });

  it('keeps the current tree for an invalid query and passes the server selector unchanged', async () => {
    const client = createMockClient({ count: 300, liveIntervalMs: 0 });
    const query = vi.spyOn(client, 'query');
    const state = new SitemapState(client);
    const stop = state.start();
    try {
      await waitFor(() => expect(state.children['']?.length).toBeGreaterThan(0));
      await state.apply('"static" // comment', '1');
      const node = state.children['']![0]!;
      state.select(node);
      await waitFor(() => expect(state.rows.length).toBeGreaterThan(0));
      expect(state.total).toBe(node.requestCount);
      expect(
        query.mock.calls.findLast(([document]) => document === SitemapRequestsDocument)?.[1],
      ).toMatchObject({ query: node.query, scopeId: '1' });
      const children = state.children;
      await state.apply('(');
      expect(state.error).not.toBeNull();
      expect(state.children).toBe(children);
      expect(state.selected).toBe(node);
    } finally {
      stop();
    }
  });

  it('batches live recursive subtrees, skips overlapping roots and refreshes deletion counts', async () => {
    vi.useFakeTimers();
    const client = createMockClient({ count: 500, liveIntervalMs: 25 });
    const query = vi.spyOn(client, 'query');
    const state = new SitemapState(client);
    const stop = state.start();
    try {
      await vi.waitFor(() => expect(state.children['']?.length).toBeGreaterThan(0));
      const [host, other] = state.children['']!.filter((node) => node.hasChildren);
      await state.toggle(host!, true);
      const directory = state.children[host!.id]!.find((node) => node.hasChildren)!;
      await state.toggle(directory, true);
      await state.toggle(other!);
      expect(state.expanded.size).toBeGreaterThan(3);
      const leaf = state.children[directory.id]![0]!;
      state.select(leaf);
      await vi.waitFor(() => expect(state.rows.length).toBeGreaterThan(0));
      state.confirmation = { node: host!, scopeId: null };
      const visible = state.visible.map(({ node }) => node.id);
      query.mockClear();

      await vi.advanceTimersByTimeAsync(1100);

      const calls = query.mock.calls.filter(([document]) => document === SitemapNodesDocument);
      expect(calls).toHaveLength(3);
      expect(calls.map(([, variables]) => variables)).toEqual([
        expect.objectContaining({ parentId: null, recursive: false }),
        expect.objectContaining({ parentId: host!.id, recursive: true }),
        expect.objectContaining({ parentId: other!.id, recursive: false }),
      ]);
      expect(state.visible.map(({ node }) => node.id)).toEqual(visible);
      expect(state.children[directory.id]!.every((node) => node.parentId === directory.id)).toBe(
        true,
      );
      expect(state.selected?.id).toBe(leaf.id);
      expect(state.confirmation?.node.requestCount).toBeGreaterThan(host!.requestCount);
      expect(state.confirmation?.scopeId).toBeNull();

      await state.toggle(host!);
      state.select(other!);
      query.mockClear();
      await state.refresh();
      expect(
        query.mock.calls
          .filter(([document]) => document === SitemapNodesDocument)
          .map(([, variables]) => variables?.parentId),
      ).toEqual([null, other!.id]);

      await state.apply();
      await state.toggle(state.children['']!.find((node) => node.id === host!.id)!);
      query.mockClear();
      await state.refresh();
      expect(
        query.mock.calls
          .filter(([document]) => document === SitemapNodesDocument)
          .every(([, variables]) => variables?.recursive === false),
      ).toBe(true);
    } finally {
      stop();
    }
  });

  it('drops a stale expansion result when filters change and supports loading beyond the first page', async () => {
    const client = createMockClient({ count: 12000, liveIntervalMs: 0 });
    const query = vi.spyOn(client, 'query');
    const state = new SitemapState(client);
    const stop = state.start();
    try {
      await waitFor(() => expect(state.children['']?.length).toBeGreaterThan(0));
      const host = state.children['']!.find((node) => node.requestCount > 1000)!;
      expect(host).toBeDefined();
      const expansion = state.toggle(host);
      await state.apply('"static"');
      await expansion;
      expect(state.expanded.size).toBe(0);
      expect(state.busy.size).toBe(0);
      await state.apply('');
      state.select(state.children['']!.find((node) => node.id === host.id)!);
      await waitFor(() => expect(state.rows).toHaveLength(1000));
      expect(state.cursor).not.toBeNull();
      while (state.cursor) await state.loadRows(true);
      expect(state.rows).toHaveLength(host.requestCount);
      expect(state.cursor).toBeNull();
      expect(new Set(state.rows.map((row) => row.id)).size).toBe(host.requestCount);

      const rows = state.rows;
      query.mockClear();
      await state.refresh();
      expect(state.rows).toBe(rows);
      expect(
        query.mock.calls.filter(([document]) => document === SitemapRequestsDocument),
      ).toHaveLength(0);

      const updated = state.rows[0]!.id;
      await client
        .mutation(HistorySetNoteDocument, { id: updated, note: 'updated during capture' })
        .toPromise();
      await waitFor(() => expect(state.rows[0]?.note).toBe('updated during capture'), {
        timeout: 2000,
      });
      expect(state.rows).toHaveLength(host.requestCount);
      query.mockClear();

      const deleted = state.rows.at(-1)!.id;
      await client.mutation(HistoryDeleteRequestsDocument, { ids: [deleted] }).toPromise();
      await state.refresh();
      expect(state.rows).toHaveLength(host.requestCount - 1);
      expect(state.total).toBe(host.requestCount - 1);
      expect(state.cursor).toBeNull();
      expect(state.rows.some((row) => row.id === deleted)).toBe(false);
      expect(new Set(state.rows.map((row) => row.id)).size).toBe(state.rows.length);
      expect(
        query.mock.calls.filter(([document]) => document === SitemapRequestsDocument),
      ).toHaveLength(Math.ceil(state.rows.length / 1000));
    } finally {
      stop();
    }
  });

  it('brackets IPv6 scope rules and avoids adding them twice', async () => {
    const client = createMockClient({ count: 100, liveIntervalMs: 0 });
    const mutation = vi.spyOn(client, 'mutation');
    const state = new SitemapState(client);
    const stop = state.start();
    try {
      await waitFor(() => expect(state.children['']?.length).toBeGreaterThan(0));
      await waitFor(() => expect(state.scopes.length).toBeGreaterThan(0));
      state.scopeId = '1';
      const scope = state.scopes.find((scope) => scope.id === '1')!;
      const host = { ...state.children['']![0]!, host: '::1' };
      state.target = host;

      await state.addHostToScope();

      expect(mutation).toHaveBeenCalledWith(UpdateScopePresetDocument, {
        id: scope.id,
        name: scope.name,
        allow: [...scope.allow, '[::1]'],
        deny: scope.deny,
      });
      expect(state.scopes.find((scope) => scope.id === '1')?.allow).toContain('[::1]');
      state.target = host;
      await state.addHostToScope();
      expect(mutation).toHaveBeenCalledTimes(1);
    } finally {
      stop();
    }
  });
  it('clears project data and pending deletion before loading the next project', async () => {
    const client = createMockClient({ count: 100, liveIntervalMs: 0 });
    const state = new SitemapState(client);
    const stop = state.start();
    try {
      await waitFor(() => expect(state.children['']?.length).toBeGreaterThan(0));
      const host = state.children['']![0]!;
      state.select(host);
      state.confirmation = { node: host, scopeId: null };
      state.scopeId = '1';
      state.reset();
      expect(state.selected).toBeNull();
      expect(state.target).toBeNull();
      expect(state.confirmation).toBeNull();
      expect(state.scopeId).toBeNull();
      expect(state.rows).toEqual([]);
      expect(state.children).toEqual({});
      await waitFor(() => expect(state.children['']?.length).toBeGreaterThan(0));
      expect(state.selected).toBeNull();
    } finally {
      stop();
    }
  });
});
