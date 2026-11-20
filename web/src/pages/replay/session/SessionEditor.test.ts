import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import {
  CancelReplaySendDocument,
  CreateReplaySessionDocument,
  ReplaySessionDetailDocument,
  SendReplaySessionDocument,
  UpdateReplaySessionDraftDocument,
} from '$lib/api/generated/graphql';
import { createMockClient, type MockExchangeOptions } from '$lib/api/mocks/exchange';
import { EnvironmentStore } from '$lib/stores/environment.svelte';
import SessionEditor from './SessionEditor.svelte';

// jsdom has no layout; CodeMirror measures text with Range#getClientRects.
Range.prototype.getClientRects = () => [] as unknown as DOMRectList;

const b64 = (text: string) => btoa(text);
const CONNECTION = { host: 'h', port: 80, tls: false };

interface Sent {
  raw: string;
  connection: Record<string, unknown>;
  options: Record<string, unknown>;
}

/** Renders the editor on the mock backend and records the mutations it runs. */
async function setup(sessionId = 'rs3', client = mockClient()) {
  const env = new EnvironmentStore(() => client);
  await env.refresh();
  const ops: string[] = [];
  const sent: Sent[] = [];
  const real = client.mutation.bind(client);
  client.mutation = ((query, vars, ctx) => {
    if (query === SendReplaySessionDocument) {
      ops.push('send');
      sent.push((vars as { input: Sent }).input);
    } else if (query === CancelReplaySendDocument) ops.push('cancel');
    else if (query === UpdateReplaySessionDraftDocument) ops.push('update');
    return real(query, vars, ctx);
  }) as typeof client.mutation;
  const result = render(SessionEditor, { sessionId, client, env });
  return { ...result, client, env, ops, sent };
}

const mockClient = (opts: MockExchangeOptions = {}) =>
  createMockClient({ count: 5, liveIntervalMs: 0, replaySendDelayMs: 1000, ...opts });

describe('SessionEditor', () => {
  it('sends the editor bytes with the connection and options', async () => {
    const { sent, ops } = await setup();
    await screen.findByRole('textbox', { name: 'Request' });

    await userEvent.click(screen.getByRole('button', { name: 'Send' }));
    // Progress while the send is in flight.
    await waitFor(() => expect(screen.getByRole('button', { name: 'Sending…' })).toBeDisabled());

    expect(ops).toEqual(['send']);
    expect(atob(sent[0]!.raw)).toBe(
      'GET /api/v1/users HTTP/1.1\r\nHost: api.example.com\r\nAccept: */*\r\n\r\n',
    );
    expect(sent[0]!.connection).toEqual({
      host: 'api.example.com',
      port: 443,
      tls: true,
      sni: null,
    });
    expect(sent[0]!.options).toEqual({
      followRedirects: false,
      updateContentLength: true,
      timeoutMs: 60000,
      applyMatchReplace: false,
    });

    // The new entry is selected and its response shown: three seeded entries plus this one.
    expect(await screen.findByText('1/4', {}, { timeout: 3000 })).toBeInTheDocument();
    expect(screen.getByText(/^200 · /)).toBeInTheDocument();
  });

  it('cancels an in-flight send and shows the failed entry', async () => {
    const { ops } = await setup();
    await screen.findByRole('textbox', { name: 'Request' });

    await userEvent.click(screen.getByRole('button', { name: 'Send' }));
    const cancel = screen.getByRole('button', { name: 'Cancel' });
    await waitFor(() => expect(cancel).toBeEnabled());
    await userEvent.click(cancel);

    expect(await screen.findByText('Send failed: cancelled', {}, { timeout: 3000 })).toBeVisible();
    expect(ops).toContain('cancel');
  });

  it('loads an entry request into the editor and its response into the viewer', async () => {
    const client = mockClient({ replaySendDelayMs: 0 });
    const created = await client
      .mutation(CreateReplaySessionDocument, {
        input: { connection: CONNECTION, raw: b64('GET /one HTTP/1.1\r\n\r\n') },
      })
      .toPromise();
    const id = created.data!.createReplaySession.id;
    for (const path of ['/two', '/three'])
      await client
        .mutation(SendReplaySessionDocument, {
          id,
          input: { raw: b64(`GET ${path} HTTP/1.1\r\n\r\n`), connection: CONNECTION },
        })
        .toPromise();

    const { ops } = await setup(id, client);
    const box = await screen.findByRole('textbox', { name: 'Request' });
    expect(box.textContent).toContain('GET /three');

    await userEvent.click(screen.getByRole('button', { name: 'Older entry' }));
    await waitFor(() => expect(box.textContent).toContain('GET /two'));
    expect(screen.getByText('2/2')).toBeInTheDocument();
    expect(ops).toEqual([]); // navigation is not an edit

    // Editing afterwards makes the edit the draft (saved on the next tick).
    await userEvent.click(box);
    await userEvent.keyboard('{Control>}{End}{/Control}X');
    await waitFor(() => expect(ops).toEqual(['update']), { timeout: 3000 });
    const detail = await client
      .query(ReplaySessionDetailDocument, { id }, { requestPolicy: 'network-only' })
      .toPromise();
    expect(atob(detail.data!.replaySession!.draft)).toMatch(/GET \/two[\s\S]*X$/);
  });

  it('shows the selected environment value on hover and switches the selection', async () => {
    const { container, env } = await setup('rs9');
    await waitFor(() => expect(container.querySelector('.cm-var-known')).not.toBeNull());
    expect(container.querySelector('.cm-var-known')).toHaveAttribute('title', 'token = ••••');

    await userEvent.selectOptions(screen.getByLabelText('Environment'), 'env2');
    await waitFor(() => expect(env.selected?.name).toBe('Production'));
  });

  it('saves the connection when the bar changes', async () => {
    const { ops, client } = await setup();
    await screen.findByRole('textbox', { name: 'Request' });

    await userEvent.selectOptions(screen.getByLabelText('Scheme'), 'http');

    await waitFor(() => expect(ops).toEqual(['update']));
    const detail = await client
      .query(ReplaySessionDetailDocument, { id: 'rs3' }, { requestPolicy: 'network-only' })
      .toPromise();
    expect(detail.data!.replaySession!.connection).toEqual({
      host: 'api.example.com',
      port: 443,
      tls: false,
      sni: null,
    });
  });

  it('shows a failed entry as an error banner instead of a response', async () => {
    await setup();
    await screen.findByRole('textbox', { name: 'Request' });
    await userEvent.click(screen.getByRole('button', { name: 'Older entry' }));
    expect(await screen.findByText('Send failed: timeout after 60000 ms')).toBeVisible();
  });
});
