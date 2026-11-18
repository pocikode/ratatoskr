import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, onTestFinished, vi } from 'vitest';
import { registry } from '$lib/commands';
import { ProjectStore, projectCommands, type Project } from '$lib/stores/project.svelte';
import WorkspacePage from './WorkspacePage.svelte';

const proj = (id: string, name: string, o: Partial<Project> = {}): Project => ({
  id,
  name,
  isTemporary: false,
  isDefault: false,
  isSelected: false,
  createdAt: '2026-11-18T00:00:00Z',
  lastOpenedAt: null,
  dbSizeBytes: '2048',
  ...o,
});

/** Fake urql client: records mutation variables, serves a fixed project list. */
function setup(list: Project[]) {
  const mutations: unknown[] = [];
  const client = {
    query: () => ({ toPromise: async () => ({ data: { projects: list } }) }),
    mutation: (_doc: unknown, vars: unknown) => {
      mutations.push(vars);
      return { toPromise: async () => ({ data: {} }) };
    },
    subscription: () => ({ subscribe: () => ({ unsubscribe: vi.fn() }) }),
  };
  const store = new ProjectStore(() => client as never);
  onTestFinished(registry.register(...projectCommands(store, () => {}))); // normally registered by the top-bar switcher
  render(WorkspacePage, { store });
  return { store, mutations, user: userEvent.setup() };
}

describe('WorkspacePage', () => {
  const list = [
    proj('1', 'Alpha', { isSelected: true }),
    proj('2', 'Beta'),
    proj('3', 'Tmp', { isTemporary: true }),
  ];

  async function ready() {
    const s = setup(list);
    await s.store.refresh();
    await screen.findByText('Beta');
    return s;
  }

  it('marks current and temporary projects', async () => {
    await ready();
    expect(screen.getByText('(current)')).toBeInTheDocument();
    expect(screen.getByText('(temporary)')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Set Tmp as default' })).toBeNull();
  });

  it('creates a project', async () => {
    const { user, mutations } = await ready();
    await user.click(screen.getByRole('button', { name: 'New project' }));
    await user.type(screen.getByLabelText('Project name'), 'Gamma');
    await user.click(screen.getByRole('button', { name: 'Create' }));
    await waitFor(() => expect(mutations).toContainEqual({ name: 'Gamma', select: false }));
  });

  it('renames, duplicates, selects and sets default', async () => {
    const { user, mutations } = await ready();
    await user.click(screen.getByRole('button', { name: 'Rename Beta' }));
    const input = screen.getByLabelText('Project name');
    await user.clear(input);
    await user.type(input, 'Beta2');
    await user.click(screen.getByRole('button', { name: 'Rename' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    await user.click(screen.getByRole('button', { name: 'Duplicate Beta' }));
    await user.click(screen.getByRole('button', { name: 'Select Beta' }));
    await user.click(screen.getByRole('button', { name: 'Set Beta as default' }));
    await waitFor(() => expect(mutations).toHaveLength(4));
    expect(mutations).toEqual([
      { id: '2', name: 'Beta2' },
      { id: '2', name: undefined },
      { id: '2' },
      { id: '2' },
    ]);
  });

  it('deletes only after typing the exact name', async () => {
    const { user, mutations } = await ready();
    await user.click(screen.getByRole('button', { name: 'Delete Beta' }));
    const confirm = screen.getByRole('button', { name: 'Delete' });
    expect(confirm).toBeDisabled();
    await user.type(screen.getByLabelText('Confirm project name'), 'Bet');
    expect(confirm).toBeDisabled();
    await user.type(screen.getByLabelText('Confirm project name'), 'a');
    await user.click(confirm);
    await waitFor(() => expect(mutations).toEqual([{ id: '2' }]));
  });
});
