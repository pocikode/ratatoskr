import { fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it } from 'vitest';
import { createMockClient } from '$lib/api/mocks/exchange';
import HttpHistoryPage from './HttpHistoryPage.svelte';

beforeEach(() => localStorage.clear());

describe('HttpHistoryPage', () => {
  it('renders the query bar and the history rows from the client', async () => {
    render(HttpHistoryPage, { client: createMockClient({ count: 12, liveIntervalMs: 0 }) });

    await screen.findByText('12 rows');
    expect(screen.getByRole('textbox', { name: 'HTTPQL query' })).toBeInTheDocument();
    expect(screen.getByRole('grid', { name: 'HTTP History' })).toBeInTheDocument();
    expect(screen.getByText('12 matching')).toBeInTheDocument();
  });

  it('loads another page when the history table scrolls near the end', async () => {
    render(HttpHistoryPage, { client: createMockClient({ count: 1001, liveIntervalMs: 0 }) });
    await screen.findByText('1000 rows');
    const grid = screen.getByRole('grid', { name: 'HTTP History' });
    const viewport = grid.querySelector<HTMLElement>('.overflow-auto')!;

    viewport.scrollTop = 950 * 26;
    await fireEvent.scroll(viewport);

    await screen.findByText('1001 rows');
    expect(within(grid).getAllByRole('row').length).toBeLessThan(80);
  });

  it('keeps header focus and the active request after sorting by keyboard', async () => {
    const user = userEvent.setup();
    render(HttpHistoryPage, { client: createMockClient({ count: 12, liveIntervalMs: 0 }) });
    await screen.findByText('12 rows');
    const grid = screen.getByRole('grid', { name: 'HTTP History' });
    grid.focus();
    await user.keyboard('{ArrowDown}');

    const header = within(grid).getByRole('button', { name: 'Host' });
    header.focus();
    await user.keyboard('{Enter}');
    await screen.findByText('12 matching');

    expect(header).toHaveFocus();
    expect(within(grid).getByRole('columnheader', { name: 'Host' })).toHaveAttribute(
      'aria-sort',
      'ascending',
    );
    const active = document.getElementById(grid.getAttribute('aria-activedescendant') ?? '');
    expect(active).toBe(within(grid).getByRole('row', { selected: true }));
    expect(within(active!).getByRole('gridcell', { name: '12' })).toBeInTheDocument();
  });

  it('shows the selected messages and updates the response preview when selection changes', async () => {
    const user = userEvent.setup();
    render(HttpHistoryPage, { client: createMockClient({ count: 12, liveIntervalMs: 0 }) });
    await screen.findByText('12 rows');
    expect(screen.getAllByText('Select a request to view it.').length).toBeGreaterThan(0);

    const grid = within(screen.getByRole('grid', { name: 'HTTP History' }));
    await user.click(grid.getByRole('gridcell', { name: '11' }));

    const request = await screen.findByRole('region', { name: 'Request' });
    expect(request).toHaveTextContent(/HTTP\/1\.1/);
    const requestPane = within(request);
    const responsePane = within(await screen.findByRole('region', { name: 'Response' }));
    const textModes = ['Raw', 'Pretty', 'Hex', 'Table'];
    expect(requestPane.getAllByRole('tab').map((tab) => tab.textContent?.trim())).toEqual(
      textModes,
    );
    expect(responsePane.getAllByRole('tab').map((tab) => tab.textContent?.trim())).toEqual([
      ...textModes,
      'Preview',
    ]);
    expect(requestPane.queryByRole('link', { name: 'Open in browser' })).not.toBeInTheDocument();

    const link = responsePane.getByRole('link', { name: 'Open in browser' });
    expect(link).toHaveAttribute('href', '/api/rest/preview/11');
    expect(link).toHaveAttribute('target', '_blank');
    expect(link).toHaveAttribute('rel', 'noopener noreferrer');
    responsePane.getByRole('button', { name: 'Copy message' }).focus();
    await user.tab();
    expect(link).toHaveFocus();

    await user.click(responsePane.getByRole('tab', { name: 'Preview' }));
    const frame = responsePane.getByTitle('Response preview');
    expect(frame).toHaveAttribute('src', '/api/rest/preview/11');
    expect(frame).toHaveAttribute('sandbox', '');

    await user.click(grid.getByRole('gridcell', { name: '10' }));
    await waitFor(() => expect(frame).toHaveAttribute('src', '/api/rest/preview/10'));
    expect(link).toHaveAttribute('href', '/api/rest/preview/10');

    await user.click(grid.getByRole('gridcell', { name: '12' }));
    await screen.findByText('No response for this request.');
    expect(screen.queryByRole('link', { name: 'Open in browser' })).not.toBeInTheDocument();
  });

  it('offers the row actions through the context menu command registry', async () => {
    render(HttpHistoryPage, { client: createMockClient({ count: 5, liveIntervalMs: 0 }) });
    await screen.findByText('5 rows');

    const row = screen.getAllByRole('row')[1]!;
    await fireEvent.contextMenu(row, { clientX: 40, clientY: 80 });

    const menu = await screen.findByRole('menu', { name: 'Request actions' });
    const items = screen.getAllByRole('menuitem');
    expect(items.length).toBeGreaterThan(5);
    expect(menu).toHaveTextContent('Copy URL');
    expect(menu).toHaveTextContent('Filter by this host');
    expect(menu).toHaveTextContent('Edit note…');

    await userEvent.click(screen.getByRole('menuitem', { name: 'Filter by this host' }));

    expect(screen.queryByRole('menu')).not.toBeInTheDocument();
    // The command wrote the filter into the query bar.
    expect(screen.getByRole('textbox', { name: 'HTTPQL query' })).toHaveTextContent(
      /req\.host\.eq:"/,
    );
  });

  it('applies a filter preset chip to the query bar', async () => {
    render(HttpHistoryPage, { client: createMockClient({ count: 5, liveIntervalMs: 0 }) });
    await screen.findByText('5 rows');

    const chips = await screen.findByRole('group', { name: 'Filter presets' });
    await userEvent.click(within(chips).getByRole('button', { name: 'Errors' }));

    expect(screen.getByRole('textbox', { name: 'HTTPQL query' })).toHaveTextContent(
      'preset:"Errors"',
    );
  });

  it('selects a scope next to the query bar', async () => {
    render(HttpHistoryPage, { client: createMockClient({ count: 5, liveIntervalMs: 0 }) });
    await screen.findByText('5 rows');

    const select = await screen.findByRole('combobox', { name: 'Scope' });
    await userEvent.selectOptions(select, '2');

    expect(select).toHaveValue('2');
  });

  it('offers the scope row actions only while a scope is active', async () => {
    render(HttpHistoryPage, { client: createMockClient({ count: 5, liveIntervalMs: 0 }) });
    await screen.findByText('5 rows');
    const row = screen.getAllByRole('row')[1]!;

    await fireEvent.contextMenu(row, { clientX: 10, clientY: 10 });
    expect(screen.queryByRole('menuitem', { name: 'Add host to scope' })).not.toBeInTheDocument();
    await userEvent.keyboard('{Escape}');

    const select = await screen.findByRole('combobox', { name: 'Scope' });
    await userEvent.selectOptions(select, '1');
    await fireEvent.contextMenu(screen.getAllByRole('row')[1]!, { clientX: 10, clientY: 10 });

    expect(screen.getByRole('menuitem', { name: 'Add host to scope' })).toBeInTheDocument();
    expect(screen.getByRole('menuitem', { name: 'Exclude host from scope' })).toBeInTheDocument();
  });
});
