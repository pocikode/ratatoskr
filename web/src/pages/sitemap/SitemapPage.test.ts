import { fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
import { createMockClient } from '$lib/api/mocks/exchange';
import {
  SitemapDeleteDocument,
  SitemapNodesDocument,
  SitemapRequestsDocument,
} from '$lib/api/generated/graphql';
import SitemapPage from './SitemapPage.svelte';

// jsdom omits Range geometry; CodeMirror measures edited text on animation frames.
beforeAll(() => {
  const createRange = document.createRange.bind(document);
  vi.spyOn(document, 'createRange').mockImplementation(() => {
    const range = createRange();
    range.getClientRects = () => Object.assign([], { item: () => null });
    range.getBoundingClientRect = () => new DOMRect();
    return range;
  });
});
afterAll(() => vi.restoreAllMocks());

function setup() {
  const client = createMockClient({ count: 300, liveIntervalMs: 0 });
  const query = vi.spyOn(client, 'query');
  const mutation = vi.spyOn(client, 'mutation');
  render(SitemapPage, { client });
  return { client, query, mutation };
}

describe('SitemapPage', () => {
  it('loads roots lazily, uses the node query unchanged and shows the selected message', async () => {
    const { query } = setup();
    const hosts = await screen.findAllByRole('treeitem');
    expect(query.mock.calls.filter(([document]) => document === SitemapNodesDocument)).toHaveLength(
      1,
    );
    expect(hosts.every((host) => host.getAttribute('aria-level') === '1')).toBe(true);
    await userEvent.click(hosts[0]!);
    await waitFor(() =>
      expect(
        screen.getAllByRole('treeitem').some((item) => item.getAttribute('aria-level') === '2'),
      ).toBe(true),
    );
    const table = screen.getByRole('grid', { name: 'Sitemap requests' });
    await waitFor(() => expect(within(table).getAllByRole('row').length).toBeGreaterThan(1));
    const treeCall = query.mock.calls.find(([document]) => document === SitemapNodesDocument)!;
    expect(treeCall[1]).toMatchObject({ recursive: false });
    const tableCall = query.mock.calls.find(([document]) => document === SitemapRequestsDocument)!;
    expect(tableCall[1]).toMatchObject({ scopeId: null });
    expect((tableCall[1] as { query: string }).query).toMatch(/req\.host\.eq:/);
    await userEvent.click(within(table).getAllByRole('row')[1]!);
    expect(await screen.findByRole('region', { name: 'Request' })).toHaveTextContent('HTTP/1.1');
  });

  it('shift-click loads descendants in one call and supports keyboard navigation', async () => {
    const { query } = setup();
    const [host] = await screen.findAllByRole('treeitem');
    await fireEvent.click(host!, { shiftKey: true });
    await waitFor(() =>
      expect(
        screen
          .getAllByRole('treeitem')
          .some((item) => Number(item.getAttribute('aria-level')) >= 3),
      ).toBe(true),
    );
    const calls = query.mock.calls.filter(([document]) => document === SitemapNodesDocument);
    expect(calls).toHaveLength(2);
    expect(calls[1]![1]).toMatchObject({ recursive: true });
    const tree = screen.getByRole('tree', { name: 'Sitemap' });
    await fireEvent.keyDown(tree, { key: 'ArrowDown' });
    await fireEvent.keyDown(tree, { key: 'Enter' });
    expect(
      screen
        .getAllByRole('treeitem')
        .filter((node) => node.getAttribute('aria-selected') === 'true'),
    ).toHaveLength(1);
  });

  it('confirms deletion and deletes exactly the node table query, then refreshes the tree', async () => {
    const { query, mutation } = setup();
    const [host] = await screen.findAllByRole('treeitem');
    const label = host!.getAttribute('aria-label')!;
    await userEvent.click(host!);
    await waitFor(() =>
      expect(query.mock.calls.some(([document]) => document === SitemapRequestsDocument)).toBe(
        true,
      ),
    );
    const tableVariables = query.mock.calls.find(
      ([document]) => document === SitemapRequestsDocument,
    )![1];
    await userEvent.pointer({ keys: '[MouseRight]', target: host! });
    await fireEvent.click(await screen.findByRole('menuitem', { name: 'Delete node…' }));
    const dialog = await screen.findByRole('dialog', { name: 'Delete sitemap node?' });
    expect(dialog).toHaveTextContent(label.split(' · ')[0]!);
    expect(mutation).not.toHaveBeenCalled();
    await userEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }));
    expect(mutation).not.toHaveBeenCalled();
    await userEvent.pointer({ keys: '[MouseRight]', target: host! });
    await fireEvent.click(await screen.findByRole('menuitem', { name: 'Delete node…' }));
    await userEvent.click(await screen.findByRole('button', { name: 'Delete requests' }));
    await waitFor(() =>
      expect(mutation).toHaveBeenCalledWith(SitemapDeleteDocument, {
        query: (tableVariables as { query: string }).query,
        scopeId: null,
      }),
    );
    await waitFor(
      () => expect(screen.queryByRole('treeitem', { name: label })).not.toBeInTheDocument(),
      { timeout: 2000 },
    );
  });

  it('refilters roots with the selected scope and the query bar', async () => {
    const { query } = setup();
    await screen.findAllByRole('treeitem');
    const select = await screen.findByRole('combobox', { name: 'Scope' });
    await userEvent.selectOptions(select, '1');
    await waitFor(() =>
      expect(
        query.mock.calls.some(
          ([document, variables]) =>
            document === SitemapNodesDocument && variables?.scopeId === '1',
        ),
      ).toBe(true),
    );
    const textbox = screen.getByRole('textbox', { name: 'HTTPQL query' });
    await userEvent.click(textbox);
    await userEvent.setup().paste('req.method.eq:"GET"');
    await userEvent.keyboard('{Enter}');
    await waitFor(() =>
      expect(
        query.mock.calls.some(
          ([document, variables]) =>
            document === SitemapNodesDocument &&
            variables?.query === 'req.method.eq:"GET"' &&
            variables?.scopeId === '1',
        ),
      ).toBe(true),
    );
  });
});
