import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { createMockClient } from '$lib/api/mocks/exchange';
import { InterceptStore } from '$lib/stores/intercept.svelte';
import InterceptPage from './InterceptPage.svelte';

// jsdom has no layout; CodeMirror measures text with Range#getClientRects.
Range.prototype.getClientRects ??= () => [] as unknown as DOMRectList;

function setup() {
  const client = createMockClient();
  const store = new InterceptStore(
    () => client,
    () => undefined,
    { onReset: () => () => {} },
  );
  const view = render(InterceptPage, { store, client });
  return { client, store, view };
}

const forwardBtn = () => screen.getByRole('button', { name: 'Forward' });

describe('InterceptPage', () => {
  it('lists held requests FIFO and opens the first in the editor', async () => {
    const { store } = setup();
    await waitFor(() => expect(store.count).toBe(3));
    expect(screen.getByRole('tab', { name: 'Requests (2)' })).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: 'Responses (1)' })).toBeInTheDocument();
    expect(await screen.findByLabelText('Held request')).toBeInTheDocument();
  });

  it('forwards unchanged (raw null) and selects the next item', async () => {
    const { client, store } = setup();
    await waitFor(() => expect(store.count).toBe(3));
    const mutation = vi.spyOn(client, 'mutation');
    const second = store.queue[1]!.id;
    await userEvent.click(forwardBtn());
    await waitFor(() => expect(store.of('REQUEST')).toHaveLength(1));
    expect(mutation.mock.calls[0]![1]).toMatchObject({ raw: null });
    expect(store.of('REQUEST')[0]!.id).toBe(second);
    expect(screen.getByRole('tab', { name: 'Requests (1)' })).toBeInTheDocument();
  });

  it('sends edited bytes only when they differ, and Undo restores the held bytes', async () => {
    const { client, store } = setup();
    await waitFor(() => expect(store.count).toBe(3));
    const mutation = vi.spyOn(client, 'mutation');
    const editor = await screen.findByRole('textbox', { name: 'Held request' });
    const undo = screen.getByRole('button', { name: 'Undo' });
    expect(undo).toBeDisabled();

    await userEvent.click(editor);
    await userEvent.keyboard('X-Edited: 1{Enter}');
    expect(undo).toBeEnabled();
    await userEvent.click(undo);
    expect(undo).toBeDisabled();
    expect(editor).not.toHaveTextContent('X-Edited');

    await userEvent.click(editor);
    await userEvent.keyboard('X-Edited: 1{Enter}');
    await userEvent.click(forwardBtn());
    await waitFor(() => expect(mutation).toHaveBeenCalled());
    expect(typeof mutation.mock.calls[0]![1]!.raw).toBe('string');
  });

  it('opens a message over 16 MiB read-only and forwards it unchanged', async () => {
    const { client, store } = setup();
    await waitFor(() => expect(store.count).toBe(3));
    const big = btoa('GET / HTTP/1.1\r\n\r\n' + 'a'.repeat(16 * 1024 * 1024));
    store.queue = [{ ...store.queue[0]!, raw: big }];
    expect(await screen.findByRole('note')).toHaveTextContent('over 16 MiB');
    const mutation = vi.spyOn(client, 'mutation');
    await userEvent.click(forwardBtn());
    await waitFor(() => expect(mutation).toHaveBeenCalled());
    expect(mutation.mock.calls[0]![1]).toMatchObject({ raw: null });
  }, 30_000); // a 16 MiB base64 round trip is slow under a loaded CI box

  it('drops the selected item', async () => {
    const { store } = setup();
    await waitFor(() => expect(store.count).toBe(3));
    await userEvent.click(screen.getByRole('button', { name: 'Drop' }));
    await waitFor(() => expect(store.count).toBe(2));
  });

  it('forward all releases the visible kind only', async () => {
    const { store } = setup();
    await waitFor(() => expect(store.count).toBe(3));
    await userEvent.click(screen.getByRole('button', { name: 'Forward all' }));
    await waitFor(() => expect(store.count).toBe(1));
    expect(store.of('RESPONSE')).toHaveLength(1);
  });

  it('shows the explanation when nothing is held', async () => {
    const { store } = setup();
    await waitFor(() => expect(store.count).toBe(3));
    await store.update({ queuing: false });
    expect(await screen.findByText(/Nothing is held/)).toBeInTheDocument();
  });

  it('shows the BAD_QUERY error inline', async () => {
    setup();
    const bar = await screen.findByRole('textbox', { name: 'Hold condition' });
    await userEvent.click(bar);
    await userEvent.keyboard('req.host.eq:"x{Enter}');
    expect(await screen.findByText('unterminated string')).toBeInTheDocument();
  });

  it('shows the server message for a bad drop status', async () => {
    setup();
    const input = await screen.findByLabelText('Drop status code');
    await userEvent.clear(input);
    await userEvent.type(input, '700');
    await userEvent.tab();
    expect(await screen.findByText(/100-599/)).toBeInTheDocument();
  });
});
