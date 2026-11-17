import { render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import StatusBar from './StatusBar.svelte';
import { ConnectionState } from '$lib/api/connection.svelte';

function renderWith(setup: (c: ConnectionState) => void) {
  const conn = new ConnectionState();
  setup(conn);
  const stop = vi.fn();
  const start = vi.fn(() => stop);
  const view = render(StatusBar, { conn, start });
  return { conn, start, stop, view };
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
});
