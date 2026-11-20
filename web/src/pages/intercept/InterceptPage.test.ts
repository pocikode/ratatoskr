import { render, screen, waitFor, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { ScopePresetsDocument } from '$lib/api/generated/graphql';
import { createMockClient } from '$lib/api/mocks/exchange';
import { InterceptStore } from '$lib/stores/intercept.svelte';
import InterceptPage from './InterceptPage.svelte';

// jsdom has no layout; CodeMirror measures text with Range#getClientRects.
Range.prototype.getClientRects ??= () => [] as unknown as DOMRectList;

function setup(options: Parameters<typeof createMockClient>[0] = {}) {
  const client = createMockClient(options);
  let reset = () => {};
  const store = new InterceptStore(
    () => client,
    () => undefined,
    { onReset: (fn) => ((reset = fn), () => {}) },
  );
  const view = render(InterceptPage, { store, client });
  return { client, store, view, reset: () => reset() };
}

const forwardBtn = () => screen.getByRole('button', { name: 'Forward' });
/** Body rows of the queue table, in rendered order. */
const rows = () => screen.getAllByRole('row').slice(1);
const idOf = (row: HTMLElement) => within(row).getAllByRole('gridcell')[0]!.textContent;
const heldCell = (row: HTMLElement) => within(row).getAllByRole('gridcell')[4]!.textContent;

describe('InterceptPage', () => {
  it('lists held requests FIFO and opens the first in the editor', async () => {
    const { store } = setup();
    await waitFor(() => expect(store.count).toBe(3));
    expect(screen.getByRole('tab', { name: 'Requests (2)' })).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: 'Responses (1)' })).toBeInTheDocument();
    expect(await screen.findByLabelText('Held request')).toBeInTheDocument();
    const [first, second] = store.of('REQUEST');
    expect(rows().map(idOf)).toEqual([first!.request.id, second!.request.id]);
    expect(rows()[0]).toHaveAttribute('aria-selected', 'true');
    expect(rows()[1]).toHaveAttribute('aria-selected', 'false');
  });

  it.each(['Forward', 'Drop'])(
    'selects the next item and shows it in the editor after %s',
    async (action) => {
      const { store } = setup();
      await waitFor(() => expect(store.count).toBe(3));
      await screen.findAllByRole('row');
      const second = store.of('REQUEST')[1]!;
      await userEvent.click(screen.getByRole('button', { name: action }));
      await waitFor(() => expect(rows()).toHaveLength(1));
      expect(idOf(rows()[0]!)).toBe(second.request.id);
      expect(rows()[0]).toHaveAttribute('aria-selected', 'true');
      expect(screen.getByRole('textbox', { name: 'Held request' })).toHaveTextContent(
        second.request.path,
      );
    },
  );

  it('forgets the draft of an item that left the queue', async () => {
    const { store } = setup();
    await waitFor(() => expect(store.count).toBe(3));
    const editor = await screen.findByRole('textbox', { name: 'Held request' });
    await userEvent.click(editor);
    await userEvent.keyboard('X-Edited: 1{Enter}');
    expect(editor).toHaveTextContent('X-Edited: 1');

    // Removed by another client (bulk release, forwarding mode, disconnect), then held again.
    const all = store.queue;
    store.queue = all.slice(1);
    await waitFor(() => expect(rows()).toHaveLength(1));
    store.queue = all;
    await waitFor(() => expect(rows()).toHaveLength(2));
    expect(screen.getByRole('textbox', { name: 'Held request' })).not.toHaveTextContent('X-Edited');
  });

  it('appends a new item without moving the selection', async () => {
    const { store } = setup({ interceptLiveIntervalMs: 50 });
    await waitFor(() => expect(store.of('REQUEST')).toHaveLength(2));
    await userEvent.click(within(rows()[1]!).getAllByRole('gridcell')[1]!);
    const selected = store.of('REQUEST')[1]!.request.id;
    await waitFor(() => expect(store.of('REQUEST').length).toBeGreaterThan(2));
    expect(rows().map(idOf).slice(0, 2)).toEqual(
      store
        .of('REQUEST')
        .slice(0, 2)
        .map((i) => i.request.id),
    );
    const row = rows().find((r) => r.getAttribute('aria-selected') === 'true')!;
    expect(idOf(row)).toBe(selected);
  });

  it('ticks the time-held column', async () => {
    const { store } = setup();
    await waitFor(() => expect(store.count).toBe(3));
    await screen.findAllByRole('row');
    vi.useFakeTimers({ toFake: ['Date', 'setInterval', 'clearInterval'] });
    try {
      expect(heldCell(rows()[0]!)).toMatch(/^\d+s$/);
      vi.setSystemTime(Date.now() + 125_000);
      vi.advanceTimersByTime(1000);
      await waitFor(() => expect(heldCell(rows()[0]!)).toMatch(/^2m \d\ds$/));
    } finally {
      vi.useRealTimers();
    }
  });

  it('keeps edits of another row when a release finishes', async () => {
    const { client, store } = setup();
    await waitFor(() => expect(store.count).toBe(3));
    await screen.findAllByRole('row');
    let open!: () => void;
    const gate = new Promise<void>((r) => (open = r));
    const real = client.mutation.bind(client);
    vi.spyOn(client, 'mutation').mockImplementation((...args) => {
      const op = real(...args);
      return { ...op, toPromise: async () => (await gate, op.toPromise()) } as typeof op;
    });

    await userEvent.click(forwardBtn()); // first row, still pending
    await userEvent.click(within(rows()[1]!).getAllByRole('gridcell')[1]!);
    const editor = screen.getByRole('textbox', { name: 'Held request' });
    await userEvent.click(editor);
    await userEvent.keyboard('X-Edited: 1{Enter}');

    open();
    await waitFor(() => expect(rows()).toHaveLength(1));
    expect(screen.getByRole('textbox', { name: 'Held request' })).toHaveTextContent('X-Edited: 1');
    expect(rows()[0]).toHaveAttribute('aria-selected', 'true');
  });

  it('refetches the scope presets on a project switch and shows scope errors', async () => {
    const { client, store, reset } = setup();
    const query = vi.spyOn(client, 'query');
    await waitFor(() => expect(store.settings).not.toBeNull());
    const presetCalls = () =>
      query.mock.calls.filter(([doc]) => doc === ScopePresetsDocument).length;
    const scope = await screen.findByRole('combobox', { name: 'Hold scope' });
    await waitFor(() => expect(scope).toHaveTextContent('Example app'));

    reset();
    // The spy was installed after the first load, so this is the refetch.
    await waitFor(() => expect(presetCalls()).toBe(1));

    vi.spyOn(store, 'update').mockResolvedValue({ error: 'scope preset 9 not found' });
    await userEvent.selectOptions(scope, 'Example app');
    expect(await screen.findByText('scope preset 9 not found')).toBeInTheDocument();
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
