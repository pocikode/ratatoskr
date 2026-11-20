import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it } from 'vitest';
import { createMockClient } from '$lib/api/mocks/exchange';
import { registry } from '$lib/commands';
import { ShortcutManager } from '$lib/commands/shortcuts';
import { InterceptStore } from '$lib/stores/intercept.svelte';
import TopBar from './TopBar.svelte';

function setup() {
  const client = createMockClient();
  const store = new InterceptStore(
    () => client,
    () => undefined,
    { onReset: () => () => {} },
  );
  // The shell registers the singleton's commands; point them at this test's store.
  const off = registry.register(
    { id: 'intercept.toggle', title: 'Toggle', run: () => void store.toggleQueuing() },
    {
      id: 'intercept.toggleRequests',
      title: 'Requests',
      run: () => void store.update({ requests: !store.settings?.requests }),
    },
    {
      id: 'intercept.toggleResponses',
      title: 'Responses',
      run: () => void store.update({ responses: !store.settings?.responses }),
    },
  );
  render(TopBar, { store });
  return { client, store, off };
}

describe('TopBar intercept toggle', () => {
  let off = () => {};
  afterEach(() => off());

  it('reflects queuing and flips it', async () => {
    const s = setup();
    off = s.off;
    const button = await screen.findByRole('button', { name: 'Intercept: Queuing' });
    expect(button).toHaveAttribute('aria-pressed', 'true');
    await userEvent.click(button);
    expect(
      await screen.findByRole('button', { name: 'Intercept: Forwarding' }),
    ).toBeInTheDocument();
  });

  it('follows a change from another client', async () => {
    const s = setup();
    off = s.off;
    await screen.findByRole('button', { name: 'Intercept: Queuing' });
    await new InterceptStore(() => s.client).update({ queuing: false });
    expect(
      await screen.findByRole('button', { name: 'Intercept: Forwarding' }),
    ).toBeInTheDocument();
  });

  it('toggles requests and responses from the dropdown', async () => {
    const s = setup();
    off = s.off;
    await waitFor(() => expect(s.store.settings).not.toBeNull());
    await userEvent.click(screen.getByRole('button', { name: 'Intercept options' }));
    // jsdom has no layout, so bits-ui leaves the floating menu `visibility: hidden`.
    await userEvent.click(await screen.findByText('Responses'));
    await waitFor(() => expect(s.store.settings?.responses).toBe(false));
    expect(s.store.settings?.requests).toBe(true);
  });

  it('Ctrl+I runs the toggle command', async () => {
    const s = setup();
    off = s.off;
    await screen.findByRole('button', { name: 'Intercept: Queuing' });
    const manager = new ShortcutManager(registry);
    manager.handle(new KeyboardEvent('keydown', { key: 'i', ctrlKey: true, metaKey: true }));
    expect(
      await screen.findByRole('button', { name: 'Intercept: Forwarding' }),
    ).toBeInTheDocument();
  });
});
