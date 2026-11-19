import { fireEvent, render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import StatusBar from './StatusBar.svelte';
import { ConnectionState } from '$lib/api/connection.svelte';
import { createMockClient } from '$lib/api/mocks/exchange';
import { EnvironmentStore } from '$lib/stores/environment.svelte';

function renderWith(setup: (c: ConnectionState) => void) {
  const conn = new ConnectionState();
  setup(conn);
  const stop = vi.fn();
  const start = vi.fn(() => stop);
  const client = createMockClient();
  const mutation = vi.spyOn(client, 'mutation');
  const env = new EnvironmentStore(() => client);
  const view = render(StatusBar, { conn, start, env });
  return { conn, start, stop, view, env, mutation };
}

describe('StatusBar', () => {
  it('starts the connection on mount and stops it on unmount', () => {
    const { start, stop, view } = renderWith(() => {});
    expect(start).toHaveBeenCalledOnce();
    view.unmount();
    expect(stop).toHaveBeenCalledOnce();
  });

  it.each([
    ['connecting', 'Connecting…'],
    ['connected', 'Connected'],
    ['disconnected', 'Disconnected, retrying'],
    ['unauthorized', 'Not authorized'],
  ] as const)('shows the %s state', (status, label) => {
    renderWith((c) => (c.status = status));
    expect(screen.getByRole('status', { name: 'Server connection' })).toHaveTextContent(label);
  });

  it('shows the server version once known', async () => {
    const { conn } = renderWith(() => {});
    expect(screen.queryByTestId('server-version')).toBeNull();
    conn.server = {
      version: 'v0.1.0',
      commit: 'abc123',
      goVersion: 'go1.27.4',
      startedAt: '2026-11-18T00:00:00Z',
      authRequired: false,
    };
    await Promise.resolve();
    expect(await screen.findByTestId('server-version')).toHaveTextContent('Ratatoskr v0.1.0');
  });

  it('shows the selected environment and switches it, including none', async () => {
    const { env, mutation } = renderWith(() => {});
    const trigger = await screen.findByRole('button', { name: 'Environment' });
    await vi.waitFor(() => expect(trigger).toHaveTextContent('env: Staging'));

    // The menu content is `visibility: hidden` in jsdom (no layout), so match it by text.
    await userEvent.click(trigger);
    await fireEvent.click(await screen.findByText('Production'));
    expect(mutation).toHaveBeenCalledWith(expect.anything(), { id: 'env2' });
    await vi.waitFor(() => expect(trigger).toHaveTextContent('env: Production'));

    await userEvent.click(trigger);
    await fireEvent.click(await screen.findByText('No environment'));
    expect(mutation).toHaveBeenCalledWith(expect.anything(), { id: null });
    await vi.waitFor(() => expect(trigger).toHaveTextContent('env: No environment'));
    expect(env.selected).toBeNull();
  });

  it('shows the error when a switch fails and keeps the old selection', async () => {
    const { env } = renderWith(() => {});
    const trigger = await screen.findByRole('button', { name: 'Environment' });
    await vi.waitFor(() => expect(trigger).toHaveTextContent('env: Staging'));

    vi.spyOn(env, 'select').mockImplementation(async () => {
      env.error = 'environment env2 not found';
      return false;
    });
    await userEvent.click(trigger);
    await fireEvent.click(await screen.findByText('Production'));

    expect(await screen.findByRole('alert')).toHaveTextContent('environment env2 not found');
    expect(trigger).toHaveTextContent('env: Staging');
  });
});
