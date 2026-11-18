import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it } from 'vitest';
import { createMockClient } from '$lib/api/mocks/exchange';
import FiltersPage from './FiltersPage.svelte';

// jsdom has no layout; CodeMirror measures the query text with Range#getClientRects.
Range.prototype.getClientRects = () => [] as unknown as DOMRectList;

beforeEach(() => localStorage.clear());

const page = () =>
  render(FiltersPage, { client: createMockClient({ count: 20, liveIntervalMs: 0 }) });

describe('FiltersPage', () => {
  it('lists the presets with their aliases and counts the selected query', async () => {
    page();

    expect(await screen.findByRole('button', { name: /^No assets/ })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Errors · errors/ })).toBeInTheDocument();
    // The draft counter follows the query bar (debounced `requests(query){totalCount}`).
    expect(await screen.findByText(/Matches \d+ requests/)).toBeInTheDocument();
  });

  it('saves a new preset from the editor', async () => {
    page();
    await screen.findByRole('button', { name: /^No assets/ });

    await userEvent.click(screen.getByRole('button', { name: 'New' }));
    await userEvent.type(screen.getByLabelText('Name'), 'Server errors');
    await userEvent.type(screen.getByLabelText('Alias'), 'server-errors');
    await userEvent.click(screen.getByRole('textbox', { name: 'Preset query' }));
    await userEvent.keyboard('resp.code.gte:500');
    await userEvent.click(screen.getByRole('button', { name: 'Save' }));

    expect(
      await screen.findByRole('button', { name: /^Server errors · server-errors/ }),
    ).toBeInTheDocument();
    expect(await screen.findByText('Matches 0 requests')).toBeInTheDocument();
  });

  it('imports presets from a JSON file and reports how many were added', async () => {
    page();
    await screen.findByRole('button', { name: /^No assets/ });
    const file = new File(
      [
        JSON.stringify({
          version: 1,
          presets: [{ name: 'Imported', alias: 'imported', query: 'req.host.eq:"api"' }],
        }),
      ],
      'presets.json',
      { type: 'application/json' },
    );

    await userEvent.upload(screen.getByLabelText('Import filter presets'), file);

    expect(await screen.findByRole('status')).toHaveTextContent('Imported 1 preset');
    expect(await screen.findByRole('button', { name: /^Imported · imported/ })).toBeInTheDocument();
  });

  it('deletes the selected preset', async () => {
    page();
    await screen.findByRole('button', { name: /^No assets/ });

    await userEvent.click(screen.getByRole('button', { name: 'Delete' }));

    await waitFor(() =>
      expect(screen.queryByRole('button', { name: /^No assets/ })).not.toBeInTheDocument(),
    );
    expect(screen.getByRole('button', { name: /^Errors/ })).toBeInTheDocument();
  });
});
