import { render, screen, waitFor, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it } from 'vitest';
import { createMockClient } from '$lib/api/mocks/exchange';
import ScopesPage from './ScopesPage.svelte';

beforeEach(() => localStorage.clear());

const page = () =>
  render(ScopesPage, { client: createMockClient({ count: 20, liveIntervalMs: 0 }) });
const list = () => screen.getByRole('region', { name: 'Scope presets' });

describe('ScopesPage', () => {
  it('lists the presets and shows the match count of the selected one', async () => {
    page();

    expect(await screen.findByRole('button', { name: 'Example app' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'API only' })).toBeInTheDocument();
    expect(await screen.findByText('Matches 20 requests')).toBeInTheDocument();
  });

  it('creates a scope with rules from the editor', async () => {
    page();
    await screen.findByRole('button', { name: 'Example app' });

    await userEvent.click(screen.getByRole('button', { name: 'New' }));
    await userEvent.type(screen.getByLabelText('Name'), 'Internal');
    await userEvent.type(screen.getByLabelText('Add to the allow list'), 'internal.test{Enter}');
    await userEvent.click(screen.getByRole('button', { name: 'Save' }));

    expect(await screen.findByRole('button', { name: 'Internal' })).toBeInTheDocument();
    // The saved preset is reopened, so its rule is a chip in the editor.
    expect(
      within(screen.getByRole('form', { name: 'Scope editor' })).getByText('internal.test'),
    ).toBeInTheDocument();
  });

  it('removes a rule from the selected preset', async () => {
    page();
    await screen.findByRole('button', { name: 'Example app' });

    await userEvent.click(screen.getByRole('button', { name: 'Remove *.example.com' }));

    expect(screen.queryByRole('button', { name: 'Remove *.example.com' })).not.toBeInTheDocument();
  });

  it('duplicates and deletes a preset', async () => {
    page();
    await screen.findByRole('button', { name: 'Example app' });

    await userEvent.click(screen.getByRole('button', { name: 'Duplicate' }));
    expect(await screen.findByRole('button', { name: 'Example app (copy)' })).toBeInTheDocument();

    await userEvent.click(screen.getByRole('button', { name: 'Delete' }));
    await waitFor(() =>
      expect(screen.queryByRole('button', { name: 'Example app (copy)' })).not.toBeInTheDocument(),
    );
    expect(screen.getByRole('button', { name: 'Example app' })).toBeInTheDocument();
  });

  it('reorders the presets', async () => {
    page();
    await screen.findByRole('button', { name: 'API only' });
    const order = () =>
      within(list())
        .getAllByRole('listitem')
        .map((item) => item.textContent?.replace(/[↑↓]/g, '').trim());
    expect(order()).toEqual(['Example app', 'API only', 'Shop (TLS)']);

    await userEvent.click(screen.getByRole('button', { name: 'Move API only up' }));

    await waitFor(() => expect(order()).toEqual(['API only', 'Example app', 'Shop (TLS)']));
  });
});
