<script lang="ts">
  // Intercept page (F-INT-1, docs/02 §3.4): held requests/responses, an editor for the selected
  // one, and the condition that decides what is held. Settings and queue live in the shared store.
  import type { Client } from '@urql/core';
  import { getApi } from '$lib/api/client';
  import { registry } from '$lib/commands';
  import SplitPane from '$lib/components/SplitPane/SplitPane.svelte';
  import { Selection } from '$lib/components/TrafficTable/selection.svelte';
  import TrafficTable from '$lib/components/TrafficTable/TrafficTable.svelte';
  import type { TrafficColumn } from '$lib/components/TrafficTable/types';
  import { bytesEqual } from '$lib/editor/bytes';
  import {
    fromBase64,
    intercept,
    type InterceptItem,
    type InterceptKind,
    type InterceptStore,
    type Outcome,
  } from '$lib/stores/intercept.svelte';
  import { requestCurl } from '../http-history/columns';
  import { openInReplay } from '../replay/openInReplay';
  import ConditionBar from './ConditionBar.svelte';
  import HeldEditor from './HeldEditor.svelte';
  import { interceptCommands } from './commands';

  interface Props {
    /** Overrides for tests; the page otherwise uses the shared store and client. */
    store?: InterceptStore;
    client?: Client;
  }

  let { store = intercept, client = getApi().client }: Props = $props();

  /** Largest message the server accepts as an edit (F-INT-1). */
  const MAX_EDIT_BYTES = 16 * 1024 * 1024;

  $effect(() => store.start());

  let kind = $state<InterceptKind>('REQUEST');
  const items = $derived(store.of(kind));
  const selection = new Selection();
  const selectedId = $derived(selection.keys.values().next().value ?? null);
  // Where the shown item sat in the queue: when it leaves (forwarded, dropped, released by
  // another client) the item that took its place is shown, which is the next one in FIFO order.
  let shownIndex = 0;
  const current = $derived(
    items.find((i) => i.id === selectedId) ?? items[Math.min(shownIndex, items.length - 1)] ?? null,
  );
  // Keeps the highlighted row on the item the editor shows.
  $effect(() => {
    if (current) {
      shownIndex = items.indexOf(current);
      if (!selection.has(current.id)) selection.click(current.id, []);
    } else if (selection.size) selection.clear();
  });

  // Unsent edits by item ID: switching rows keeps each draft, a release drops only its own.
  let edits = $state.raw<Record<string, Uint8Array>>({});
  const held = $derived(current ? fromBase64(current.raw) : null);
  const tooBig = $derived((held?.length ?? 0) > MAX_EDIT_BYTES);
  const shown = $derived((current && edits[current.id]) || held);
  const dirty = $derived(!!held && !!shown && !tooBig && !bytesEqual(shown, held));

  let busy = $state(false);
  let message = $state<string | null>(null);

  async function release(act: (item: InterceptItem) => Promise<Outcome>) {
    const item = current;
    if (!item || busy) return;
    busy = true;
    const r = await act(item);
    busy = false;
    message = r.error;
    if (r.error) return;
    const rest = { ...edits };
    delete rest[item.id];
    edits = rest;
  }

  /** Replacement bytes for a forward: only a draft that differs from what is held. */
  function replacement(item: InterceptItem): Uint8Array | null {
    const draft = edits[item.id];
    const original = fromBase64(item.raw);
    return draft && original.length <= MAX_EDIT_BYTES && !bytesEqual(draft, original)
      ? draft
      : null;
  }

  async function bulk(act: () => Promise<Outcome>) {
    if (busy) return;
    busy = true;
    message = (await act()).error;
    busy = false;
  }

  // Commands act on what is on screen; registered while the page is mounted.
  $effect(() =>
    registry.register(
      ...interceptCommands({
        hasItem: () => current !== null,
        forward: () => release((item) => store.forward(item.id, replacement(item))),
        drop: () => release((item) => store.drop(item.id)),
        forwardAll: () => bulk(() => store.forwardAll(kind)),
        dropAll: () => bulk(() => store.dropAll(kind)),
        undo: () => {
          if (!current) return;
          const rest = { ...edits };
          delete rest[current.id];
          edits = rest;
        },
        replay: async () => {
          if (current) message = await openInReplay(current.request.id, client);
        },
        copyCurl: async () => {
          if (!current) return;
          try {
            await navigator.clipboard.writeText(
              requestCurl(current.request as Parameters<typeof requestCurl>[0]),
            );
            message = null;
          } catch {
            message = 'Could not copy to the clipboard';
          }
        },
      }),
    ),
  );

  // Time held ticks once a second.
  let now = $state(Date.now());
  $effect(() => {
    const timer = setInterval(() => (now = Date.now()), 1000);
    return () => clearInterval(timer);
  });

  function heldFor(item: InterceptItem): string {
    const s = Math.max(0, Math.floor((now - Date.parse(item.heldAt)) / 1000));
    return s < 60 ? `${s}s` : `${Math.floor(s / 60)}m ${String(s % 60).padStart(2, '0')}s`;
  }

  // The queue is FIFO, so no column sorts.
  const columns: TrafficColumn<InterceptItem>[] = [
    { id: 'id', title: 'ID', width: 70, sortable: false, value: (i) => i.request.id },
    { id: 'host', title: 'Host', width: 200, sortable: false, value: (i) => i.request.host },
    { id: 'method', title: 'Method', width: 80, sortable: false, value: (i) => i.request.method },
    { id: 'path', title: 'Path', width: 280, sortable: false, value: (i) => i.request.path },
    { id: 'held', title: 'Time held', width: 90, align: 'end', sortable: false, value: heldFor },
  ];

  const TABS: { id: InterceptKind; label: string }[] = [
    { id: 'REQUEST', label: 'Requests' },
    { id: 'RESPONSE', label: 'Responses' },
  ];
  const btn =
    'rounded-token border border-line bg-subtle px-2 py-1 hover:bg-hover disabled:opacity-50';
</script>

<div class="flex h-[calc(100vh-140px)] min-h-0 flex-col gap-2">
  <ConditionBar {store} {client} />

  <div class="flex items-center gap-2">
    <div role="tablist" aria-label="Held kind" class="flex gap-1">
      {#each TABS as tab (tab.id)}
        <button
          type="button"
          role="tab"
          aria-selected={kind === tab.id}
          class="rounded-token px-2 py-1 hover:bg-hover {kind === tab.id
            ? 'bg-accent-soft text-accent'
            : 'text-muted'}"
          onclick={() => {
            kind = tab.id;
            shownIndex = 0;
          }}
        >
          {tab.label} ({store.of(tab.id).length})
        </button>
      {/each}
    </div>
    <div class="flex-1"></div>
    <button
      type="button"
      class={btn}
      disabled={!items.length || busy}
      onclick={() => registry.run('intercept.forwardAll')}>Forward all</button
    >
    <button
      type="button"
      class={btn}
      disabled={!items.length || busy}
      onclick={() => registry.run('intercept.dropAll')}>Drop all</button
    >
  </div>

  <div class="min-h-0 flex-1 rounded-token border border-line">
    <SplitPane direction="vertical" defaultSizes={[35, 65]} label="Resize queue and editor">
      {#snippet first()}
        <TrafficTable
          rows={items}
          {columns}
          rowKey={(i) => i.id}
          label="Held {kind === 'REQUEST' ? 'requests' : 'responses'}"
          {selection}
        >
          {#snippet empty()}
            <div class="p-4 text-muted">
              Nothing is held. In <strong>Forwarding</strong> mode traffic passes straight through;
              switch to <strong>Queuing</strong> in the top bar and matching requests or responses wait
              here until you forward or drop them.
            </div>
          {/snippet}
        </TrafficTable>
      {/snippet}
      {#snippet second()}
        {#if current && shown}
          <HeldEditor
            item={current}
            bytes={shown}
            {dirty}
            {tooBig}
            {busy}
            {message}
            onedit={(bytes) => (edits = { ...edits, [current.id]: bytes })}
          />
        {:else}
          <p class="p-4 text-muted">Select a held item to read or edit it.</p>
        {/if}
      {/snippet}
    </SplitPane>
  </div>
</div>
