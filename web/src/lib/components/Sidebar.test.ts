import { render, screen, waitFor } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import { createMockClient } from '$lib/api/mocks/exchange';
import { InterceptStore } from '$lib/stores/intercept.svelte';
import Sidebar from './Sidebar.svelte';

describe('Sidebar intercept badge', () => {
  it('shows the held count with an accessible label, live, and hides at 0', async () => {
    const client = createMockClient();
    const store = new InterceptStore(
      () => client,
      () => undefined,
      { onReset: () => () => {} },
    );
    render(Sidebar, { store });

    expect(await screen.findByRole('status', { name: '3 held items' })).toHaveTextContent('3');
    await store.drop(store.queue[0]!.id);
    expect(await screen.findByRole('status', { name: '2 held items' })).toBeInTheDocument();

    await store.update({ queuing: false });
    await waitFor(() => expect(screen.queryByRole('status')).not.toBeInTheDocument());
  });
});
