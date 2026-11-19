import { render, screen, waitFor, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { createMockClient } from '$lib/api/mocks/exchange';
import { EnvironmentStore } from '$lib/stores/environment.svelte';
import EnvironmentPage from './EnvironmentPage.svelte';

function page() {
  const client = createMockClient();
  const store = new EnvironmentStore(() => client);
  render(EnvironmentPage, { store });
  return store;
}

/** The left list, once the environments are loaded. */
async function envList() {
  const list = screen.getByRole('region', { name: 'Environments' });
  await within(list).findByRole('button', { name: /^Staging/ });
  return list;
}

describe('EnvironmentPage', () => {
  it('lists Global first without rename or delete actions', async () => {
    page();
    const list = await envList();
    const names = within(list)
      .getAllByRole('listitem')
      .map((item) => item.textContent?.trim());
    expect(names[0]).toMatch(/^Global/);

    await userEvent.click(within(list).getByRole('button', { name: 'Global' }));
    expect(screen.getByRole('region', { name: 'Variables' })).toHaveTextContent('Global');
    expect(screen.queryByRole('button', { name: 'Rename' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Delete' })).toBeNull();
  });

  it('saves grid edits together', async () => {
    const store = page();
    const list = await envList();
    await userEvent.click(within(list).getByRole('button', { name: /^Staging/ }));

    const value = screen.getByLabelText('Value of host');
    await userEvent.clear(value);
    await userEvent.type(value, 'qa.example.test');
    await userEvent.click(screen.getByRole('button', { name: 'Save' }));

    await waitFor(() =>
      expect(store.selected?.variables).toEqual([
        { name: 'host', value: 'qa.example.test', secret: false },
        { name: 'token', value: 'mock-secret-token', secret: true },
      ]),
    );
  });

  it('masks secret values until the row is revealed', async () => {
    page();
    const list = await envList();
    await userEvent.click(within(list).getByRole('button', { name: /^Staging/ }));

    expect(screen.getByLabelText('Value of token')).toHaveAttribute('type', 'password');
    await userEvent.click(screen.getByRole('button', { name: 'Reveal value of token' }));
    const revealed = screen.getByLabelText('Value of token');
    expect(revealed).toHaveAttribute('type', 'text');
    expect(revealed).toHaveValue('mock-secret-token');
  });

  it('highlights the row a BAD_INPUT error names', async () => {
    page();
    const list = await envList();
    await userEvent.click(within(list).getByRole('button', { name: /^Staging/ }));

    await userEvent.click(screen.getByRole('button', { name: 'Add variable' }));
    await userEvent.type(screen.getByLabelText('Name of variable 3'), 'host');
    await userEvent.click(screen.getByRole('button', { name: 'Save' }));

    expect(await screen.findByRole('alert')).toHaveTextContent('duplicate variable name "host"');
    const names = screen.getAllByLabelText('Name of host');
    expect(names).toHaveLength(2);
    for (const name of names) expect(name).toHaveAttribute('aria-invalid', 'true');
  });

  it('creates, renames and deletes a project environment', async () => {
    page();
    const list = await envList();

    await userEvent.click(screen.getByRole('button', { name: 'New' }));
    await userEvent.type(screen.getByLabelText('New environment name'), 'QA');
    await userEvent.click(screen.getByRole('button', { name: 'Create' }));
    expect(await within(list).findByRole('button', { name: /^QA/ })).toBeInTheDocument();

    await userEvent.click(screen.getByRole('button', { name: 'Rename' }));
    const name = screen.getByLabelText('Environment name');
    await userEvent.clear(name);
    await userEvent.type(name, 'QA 2');
    await userEvent.click(screen.getByRole('button', { name: 'Save name' }));
    expect(await within(list).findByRole('button', { name: /^QA 2/ })).toBeInTheDocument();

    await userEvent.click(screen.getByRole('button', { name: 'Delete' }));
    await waitFor(() => expect(within(list).queryByRole('button', { name: /^QA 2/ })).toBeNull());
  });
});
