import { fireEvent, render, screen } from '@testing-library/svelte';
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

  it('shows the selected request and response in the viewers', async () => {
    render(HttpHistoryPage, { client: createMockClient({ count: 12, liveIntervalMs: 0 }) });
    await screen.findByText('12 rows');
    expect(screen.getAllByText('Select a request to view it.').length).toBeGreaterThan(0);

    const rows = screen.getAllByRole('row');
    await userEvent.click(rows[1]!);

    const request = await screen.findByRole('region', { name: 'Request' });
    expect(request).toHaveTextContent(/HTTP\/1\.1/);
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
});
