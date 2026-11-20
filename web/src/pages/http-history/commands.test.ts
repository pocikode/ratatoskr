import { waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { ReplayCollectionsDocument } from '$lib/api/generated/graphql';
import { createMockClient } from '$lib/api/mocks/exchange';
import { CommandRegistry } from '$lib/commands/registry';
import { ShortcutManager } from '$lib/commands/shortcuts';
import { router } from '$lib/router/router.svelte';
import { historyCommands, type HistoryCommandDeps } from './commands';
import type { HistoryState } from './history.svelte';

let stop = () => {};
beforeEach(() => {
  window.location.hash = '#/http-history';
  stop = router.start();
});
afterEach(() => stop());

const deps: HistoryCommandDeps = { copy: () => {}, editNote: () => {} };

/** Only the parts `history.sendToReplay` reads; a real `HistoryState` needs runes. */
function state(client: ReturnType<typeof createMockClient>, ids: string[]) {
  return {
    client,
    rows: ids.map((id) => ({ id })),
    selection: { keys: new Set(ids), size: ids.length },
    message: null as string | null,
  } as unknown as HistoryState;
}

const sendToReplay = (s: HistoryState) =>
  historyCommands(s, deps).find((c) => c.id === 'history.sendToReplay')!;

/** Session ids of the mock project, to check that one was created. */
async function sessions(client: ReturnType<typeof createMockClient>) {
  const r = await client
    .query(ReplayCollectionsDocument, {}, { requestPolicy: 'network-only' })
    .toPromise();
  return r.data!.replayCollections.flatMap((c) => c.sessions);
}

describe('history.sendToReplay', () => {
  it('creates a session from the selected row and opens its tab', async () => {
    const client = createMockClient({ count: 5, liveIntervalMs: 0 });
    const s = state(client, ['2']);
    const command = sendToReplay(s);
    expect(command.when?.({})).toBe(true);

    await command.run({});

    await waitFor(() => expect(window.location.hash).toMatch(/^#\/replay\/session\/rs\d+$/));
    const id = window.location.hash.split('/').pop();
    const created = (await sessions(client)).find((session) => session.id === id);
    expect(created?.name).toMatch(/^[A-Z]+ \//);
  });

  it('is hidden and does nothing without a selection', async () => {
    const client = createMockClient({ count: 5, liveIntervalMs: 0 });
    const s = state(client, []);
    expect(sendToReplay(s).when?.({})).toBe(false);

    await sendToReplay(s).run({});
    expect(window.location.hash).toBe('#/http-history');
    expect(await sessions(client)).toHaveLength(3); // the seeded ones only
  });

  it('runs from the Ctrl+R binding', async () => {
    const client = createMockClient({ count: 5, liveIntervalMs: 0 });
    const registry = new CommandRegistry();
    registry.register(...historyCommands(state(client, ['1']), deps));
    const shortcuts = new ShortcutManager(registry);

    shortcuts.handle(new KeyboardEvent('keydown', { key: 'r', ctrlKey: true }));

    await waitFor(() => expect(window.location.hash).toMatch(/^#\/replay\/session\/rs\d+$/));
  });
});
