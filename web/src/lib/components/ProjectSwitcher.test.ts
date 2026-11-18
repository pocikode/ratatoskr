import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { ProjectStore, type Project } from '$lib/stores/project.svelte';
import ProjectSwitcher from './ProjectSwitcher.svelte';

const proj = (id: string, name: string, o: Partial<Project> = {}): Project => ({
  id,
  name,
  isTemporary: false,
  isDefault: false,
  isSelected: false,
  createdAt: '2026-11-18T00:00:00Z',
  lastOpenedAt: null,
  dbSizeBytes: '0',
  ...o,
});

describe('ProjectSwitcher', () => {
  it('filters, shows temporary distinctly and selects by keyboard', async () => {
    const calls: unknown[] = [];
    const client = {
      query: () => ({ toPromise: async () => ({ data: { projects: store.projects } }) }),
      mutation: (_doc: unknown, vars: unknown) => {
        calls.push(vars);
        return { toPromise: async () => ({ data: {} }) };
      },
      subscription: () => ({ subscribe: () => ({ unsubscribe: vi.fn() }) }),
    };
    const store = new ProjectStore(() => client as never);
    store.projects = [
      proj('1', 'Alpha', { isSelected: true, isTemporary: true }),
      proj('2', 'Beta'),
    ];
    render(ProjectSwitcher, { store, start: false });
    const user = userEvent.setup();

    const input = screen.getByRole('combobox', { name: 'Project' });
    expect(input).toHaveValue('Alpha');
    expect(screen.getByText('temporary')).toBeInTheDocument(); // text marker while closed
    await user.click(input);
    expect(await screen.findByText('Alpha')).toBeInTheDocument();
    expect(screen.getByText('New project…')).toBeInTheDocument();
    expect(screen.getByText('Manage projects')).toBeInTheDocument();
    await user.keyboard('bet');
    expect(screen.queryByRole('option', { name: /Alpha/ })).toBeNull();
    await user.keyboard('{Enter}');
    await waitFor(() => expect(calls).toEqual([{ id: '2' }]));
  });

  describe('fixed entries', () => {
    async function pick(label: string) {
      const client = {
        query: () => ({ toPromise: async () => ({ data: { projects: [] } }) }),
        mutation: vi.fn(),
        subscription: () => ({ subscribe: () => ({ unsubscribe: vi.fn() }) }),
      };
      const store = new ProjectStore(() => client as never);
      store.projects = [proj('1', 'Alpha', { isSelected: true })];
      render(ProjectSwitcher, { store, start: false });
      const user = userEvent.setup();
      await user.click(screen.getByRole('combobox', { name: 'Project' }));
      await user.click(await screen.findByText(label));
      return store;
    }

    it('New project… runs project.create', async () => {
      const store = await pick('New project…');
      await waitFor(() => expect(store.dialog).toEqual({ kind: 'create' }));
    });

    it('New project… works repeatedly', async () => {
      const store = await pick('New project…');
      await waitFor(() => expect(store.dialog).toEqual({ kind: 'create' }));
      store.dialog = null;
      const input = screen.getByRole('combobox', { name: 'Project' });
      await userEvent.click(input);
      await userEvent.click(await screen.findByText('New project…'));
      await waitFor(() => expect(store.dialog).toEqual({ kind: 'create' }));
    });

    it('Manage projects navigates to Workspace', async () => {
      await pick('Manage projects');
      await waitFor(() => expect(window.location.hash).toBe('#/workspace'));
    });
  });
});
