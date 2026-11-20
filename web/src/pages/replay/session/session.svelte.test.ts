import { afterEach, describe, expect, it, vi } from 'vitest';
import { Client, cacheExchange } from '@urql/core';
import {
  CancelReplaySendDocument,
  CreateReplaySessionDocument,
  SendReplaySessionDocument,
  UpdateReplaySessionDraftDocument,
} from '$lib/api/generated/graphql';
import { createMockClient, mockHistoryExchange } from '$lib/api/mocks/exchange';
import { SessionEditorState, toBase64 } from './session.svelte';

const enc = (text: string) => new TextEncoder().encode(text);
const text = (bytes: Uint8Array) => new TextDecoder().decode(bytes);
const CONNECTION = { host: 'h', port: 80, tls: false };

/** Mock client that records which mutations ran, in order. */
function recorder() {
  const client = createMockClient({ count: 5, liveIntervalMs: 0, replaySendDelayMs: 0 });
  const ops: string[] = [];
  const real = client.mutation.bind(client);
  client.mutation = ((query, vars, ctx) => {
    ops.push(
      query === UpdateReplaySessionDraftDocument
        ? 'update'
        : query === SendReplaySessionDocument
          ? 'send'
          : query === CancelReplaySendDocument
            ? 'cancel'
            : 'other',
    );
    return real(query, vars, ctx);
  }) as typeof client.mutation;
  return { client, ops };
}

/** A session from scratch with one send per path, so its entries differ. */
async function sessionWithSends(client: ReturnType<typeof createMockClient>, paths: string[]) {
  const created = await client
    .mutation(CreateReplaySessionDocument, {
      input: { connection: CONNECTION, raw: toBase64(enc('GET /draft HTTP/1.1\r\n\r\n')) },
    })
    .toPromise();
  const id = created.data!.createReplaySession.id;
  for (const path of paths)
    await client
      .mutation(SendReplaySessionDocument, {
        id,
        input: { raw: toBase64(enc(`GET ${path} HTTP/1.1\r\n\r\n`)), connection: CONNECTION },
      })
      .toPromise();
  return id;
}

afterEach(() => vi.useRealTimers());

describe('SessionEditorState', () => {
  it('saves the draft at most once per second while typing', async () => {
    vi.useFakeTimers();
    const { client, ops } = recorder();
    const state = new SessionEditorState(client, 'rs3');
    await state.load();

    state.edit(enc('GET /a HTTP/1.1\r\n\r\n'));
    await vi.advanceTimersByTimeAsync(999);
    expect(ops).toEqual([]);

    state.edit(enc('GET /ab HTTP/1.1\r\n\r\n'));
    state.edit(enc('GET /abc HTTP/1.1\r\n\r\n'));
    await vi.advanceTimersByTimeAsync(1);
    expect(ops).toEqual(['update']);

    state.edit(enc('GET /abcd HTTP/1.1\r\n\r\n'));
    await vi.advanceTimersByTimeAsync(999);
    expect(ops).toEqual(['update']);
    await vi.advanceTimersByTimeAsync(1);
    expect(ops).toEqual(['update', 'update']);
  });

  it('saves the pending draft before sending', async () => {
    const { client, ops } = recorder();
    const state = new SessionEditorState(client, 'rs3');
    await state.load();

    state.edit(enc('GET /typed HTTP/1.1\r\n\r\n'));
    await state.send();

    expect(ops).toEqual(['update', 'send']);
  });

  it('saves the pending draft when the tab goes away', async () => {
    const { client, ops } = recorder();
    const state = new SessionEditorState(client, 'rs3');
    await state.load();

    state.edit(enc('GET /bye HTTP/1.1\r\n\r\n'));
    state.dispose();

    await vi.waitFor(() => expect(ops).toEqual(['update']));
  });

  it('loads an entry request without saving it, and edits become the draft', async () => {
    const { client, ops } = recorder();
    const id = await sessionWithSends(client, ['/two', '/three']);
    ops.length = 0;
    const state = new SessionEditorState(client, id);
    await state.load();
    expect(text(state.shown)).toBe('GET /three HTTP/1.1\r\n\r\n');
    expect(state.entries).toHaveLength(2);

    await state.selectEntry(state.entries[1]!.id);
    expect(text(state.shown)).toBe('GET /two HTTP/1.1\r\n\r\n');
    expect(ops).toEqual([]); // navigation is not an edit

    state.edit(enc('GET /edited HTTP/1.1\r\n\r\n'));
    await state.flush();
    expect(ops).toEqual(['update']);
    // The response pane keeps the entry that was open.
    expect(state.viewed?.id).toBe(state.entries[1]!.id);
  });

  it('mirrors an emptied draft instead of keeping the old bytes', async () => {
    const { client } = recorder();
    const state = new SessionEditorState(client, 'rs3');
    await state.load();
    expect(atob(state.session!.draft)).not.toBe('');

    state.edit(new Uint8Array());
    await state.flush();

    expect(state.session!.draft).toBe('');
  });

  it('sends inputs without the __typename the cache adds to loaded objects', async () => {
    // Same exchange chain as the real client (docs/06 §4): the cache adds `__typename` to every
    // selection set, and the server rejects it inside an input type.
    const client = new Client({
      url: '/mock',
      exchanges: [cacheExchange, mockHistoryExchange({ count: 5, liveIntervalMs: 0 })],
    });
    let sent: { connection: object; options: object } | undefined;
    const real = client.mutation.bind(client);
    client.mutation = ((query, vars, ctx) => {
      if (query === SendReplaySessionDocument)
        sent = (vars as { input: { connection: object; options: object } }).input;
      return real(query, vars, ctx);
    }) as typeof client.mutation;

    const state = new SessionEditorState(client, 'rs3');
    await state.load();
    // The regression setup: the loaded objects do carry the extra field.
    expect(state.session!.connection).toHaveProperty('__typename');

    await state.send();

    expect(sent!.connection).toEqual({ host: 'api.example.com', port: 443, tls: true, sni: null });
    expect(sent!.options).toEqual({
      followRedirects: false,
      updateContentLength: true,
      timeoutMs: 60000,
      applyMatchReplace: false,
    });
  });
});
