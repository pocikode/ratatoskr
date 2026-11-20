<script lang="ts">
  // Intercept page (F-INT-1, docs/02 §3.4): held requests/responses, an editor for the selected
  // one, and the condition that decides what is held. Settings and queue live in the shared store.
  import type { Client } from '@urql/core';
  import { getApi } from '$lib/api/client';
  import { ScopePresetsDocument } from '$lib/api/generated/graphql';
  import { registry } from '$lib/commands';
  import type { Command } from '$lib/commands/registry';
  import QueryBar from '$lib/components/QueryBar/QueryBar.svelte';
  import ScopeSelector from '$lib/components/ScopeSelector.svelte';
  import SplitPane from '$lib/components/SplitPane/SplitPane.svelte';
  import { Selection } from '$lib/components/TrafficTable/selection.svelte';
  import TrafficTable from '$lib/components/TrafficTable/TrafficTable.svelte';
  import type { TrafficColumn } from '$lib/components/TrafficTable/types';
  import { bytesEqual } from '$lib/editor/bytes';
  import RawEditor from '$lib/editor/RawEditor.svelte';
  import { byteToDoc, type QueryError } from '$lib/query/httpql';
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
  const current = $derived(items.find((i) => i.id === selectedId) ?? items[0] ?? null);
  // Keeps the highlighted row on the item the editor shows (first item, or after a removal).
  $effect(() => {
    if (current && !selection.has(current.id)) selection.click(current.id, []);
    else if (!current && selection.size) selection.clear();
  });

  // Edit state: the bytes of one held item; any other item shows its held bytes.
  let edit = $state.raw<{ id: string; bytes: Uint8Array } | null>(null);
  const held = $derived(current ? fromBase64(current.raw) : null);
  const tooBig = $derived((held?.length ?? 0) > MAX_EDIT_BYTES);
  const shown = $derived(edit && edit.id === current?.id ? edit.bytes : held);
  const dirty = $derived(!!held && !!shown && !tooBig && !bytesEqual(shown, held));

  let busy = $state(false);
  let message = $state<string | null>(null);

  async function release(act: (item: InterceptItem) => Promise<Outcome>) {
    const item = current;
    if (!item || busy) return;
    const index = items.indexOf(item);
    const next = items[index + 1] ?? items[index - 1] ?? null;
    busy = true;
    const r = await act(item);
    busy = false;
    message = r.error;
    if (r.error) return;
    edit = null;
    if (next) selection.click(next.id, []);
  }

  const forward = () =>
    release((item) => store.forward(item.id, dirty && edit?.id === item.id ? edit.bytes : null));
  const drop = () => release((item) => store.drop(item.id));
  const undo = () => void (edit = null);

  async function bulk(act: () => Promise<Outcome>) {
    if (busy) return;
    busy = true;
    message = (await act()).error;
    busy = false;
  }

  async function sendToReplay() {
    if (current) message = await openInReplay(current.request.id, client);
  }

  async function copyCurl() {
    if (!current) return;
    try {
      await navigator.clipboard.writeText(
        requestCurl(current.request as Parameters<typeof requestCurl>[0]),
      );
      message = null;
    } catch {
      message = 'Could not copy to the clipboard';
    }
  }

  // Commands act on what is on screen; registered while the page is mounted (docs/06 §5).
  $effect(() => {
    const group = 'Intercept';
    const has = () => current !== null;
    const cmds: Command[] = [
      { id: 'intercept.forward', title: 'Forward held item', group, when: has, run: forward },
      { id: 'intercept.drop', title: 'Drop held item', group, when: has, run: drop },
      {
        id: 'intercept.forwardAll',
        title: 'Forward all held items',
        group,
        run: () => bulk(() => store.forwardAll(kind)),
      },
      {
        id: 'intercept.dropAll',
        title: 'Drop all held items',
        group,
        run: () => bulk(() => store.dropAll(kind)),
      },
      { id: 'intercept.undo', title: 'Undo edits of held item', group, when: has, run: undo },
      {
        id: 'intercept.replay',
        title: 'Send held item to Replay',
        group,
        when: has,
        run: sendToReplay,
      },
      {
        id: 'intercept.copy.curl',
        title: 'Copy held item as cURL',
        group,
        when: has,
        run: copyCurl,
      },
    ];
    return registry.register(...cmds);
  });

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

  const columns: TrafficColumn<InterceptItem>[] = [
    { id: 'id', title: 'ID', width: 70, value: (i) => i.request.id },
    { id: 'host', title: 'Host', width: 200, value: (i) => i.request.host },
    { id: 'method', title: 'Method', width: 80, value: (i) => i.request.method },
    { id: 'path', title: 'Path', width: 280, value: (i) => i.request.path },
    { id: 'held', title: 'Time held', width: 90, align: 'end', sortable: false, value: heldFor },
  ];

  // Condition bar: HTTPQL, scope and drop status are applied on submit/change.
  let queryText = $state('');
  let queryError = $state<QueryError | null>(null);
  const appliedQuery = $derived(store.settings?.query ?? '');
  $effect(() => {
    queryText = appliedQuery;
    queryError = null;
  });

  async function applyQuery(text: string) {
    const r = await store.update({ query: text });
    if (!r.error) {
      queryError = null;
      return;
    }
    queryError = {
      message: r.error,
      from: r.span ? byteToDoc(text, r.span.start) : 0,
      to: r.span ? byteToDoc(text, r.span.end) : text.length,
    };
  }

  let presets = $state.raw<{ id: string; name: string }[]>([]);
  $effect(() => {
    let cancelled = false;
    void client
      .query(ScopePresetsDocument, {}, { requestPolicy: 'network-only' })
      .toPromise()
      .then((r) => {
        if (!cancelled) presets = r.data?.scopePresets ?? [];
      });
    return () => {
      cancelled = true;
    };
  });

  let dropText = $state('502');
  let dropError = $state<string | null>(null);
  const appliedDrop = $derived(store.settings?.dropStatusCode ?? 502);
  $effect(() => {
    dropText = String(appliedDrop);
    dropError = null;
  });

  async function applyDrop() {
    // The server owns the 100-599 rule; a non-number is the only thing checked here.
    const code = Number(dropText);
    if (!Number.isInteger(code)) {
      dropError = 'Enter a whole number';
      return;
    }
    dropError = (await store.update({ dropStatusCode: code })).error;
  }

  const TABS: { id: InterceptKind; label: string }[] = [
    { id: 'REQUEST', label: 'Requests' },
    { id: 'RESPONSE', label: 'Responses' },
  ];
  const btn =
    'rounded-token border border-line bg-subtle px-2 py-1 hover:bg-hover disabled:opacity-50';
</script>

<div class="flex h-[calc(100vh-140px)] min-h-0 flex-col gap-2">
  <div class="flex items-start gap-2">
    <div class="min-w-0 flex-1">
      <QueryBar
        bind:value={queryText}
        label="Hold condition"
        placeholder="Hold only traffic matching, e.g. req.host.cont:&quot;api&quot;"
        error={queryError}
        onsubmit={applyQuery}
      >
        {#snippet trailing()}
          <ScopeSelector
            {presets}
            value={store.settings?.scopeId ?? null}
            label="Hold scope"
            onchange={(id) => void store.update({ scopeId: id })}
          />
        {/snippet}
      </QueryBar>
    </div>
    <label class="flex items-center gap-1 text-xs text-muted">
      Drop status
      <input
        class="w-16 rounded-token border border-line bg-elevated px-1.5 py-0.5 text-fg"
        inputmode="numeric"
        aria-label="Drop status code"
        aria-invalid={dropError ? 'true' : undefined}
        bind:value={dropText}
        onchange={applyDrop}
      />
    </label>
  </div>
  {#if dropError}<p role="alert" class="text-xs text-danger">{dropError}</p>{/if}

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
          onclick={() => (kind = tab.id)}
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
          <div class="flex flex-wrap items-center gap-2 border-b border-line px-2 py-1">
            <button
              type="button"
              class={btn}
              disabled={busy}
              title="Forward (⌘/Ctrl+Enter)"
              onclick={() => registry.run('intercept.forward')}>Forward</button
            >
            <button
              type="button"
              class={btn}
              disabled={busy}
              title="Drop (⌘/Ctrl+Backspace)"
              onclick={() => registry.run('intercept.drop')}>Drop</button
            >
            <button
              type="button"
              class={btn}
              disabled={!dirty}
              onclick={() => registry.run('intercept.undo')}>Undo</button
            >
            <button type="button" class={btn} onclick={() => registry.run('intercept.replay')}
              >Send to Replay</button
            >
            <button type="button" class={btn} onclick={() => registry.run('intercept.copy.curl')}
              >Copy as cURL</button
            >
            {#if message}<span role="alert" class="text-xs text-danger">{message}</span>{/if}
          </div>
          {#if tooBig}
            <p class="border-b border-line px-2 py-1 text-xs text-muted" role="note">
              This message is over 16 MiB, so it is read-only: it can only be forwarded unchanged or
              dropped.
            </p>
          {/if}
          <div class="min-h-0 flex-1">
            <RawEditor
              value={shown}
              identity={current.id}
              readonly={tooBig}
              label="Held {current.kind === 'REQUEST' ? 'request' : 'response'}"
              onchange={(bytes) => (edit = { id: current.id, bytes })}
            />
          </div>
        {:else}
          <p class="p-4 text-muted">Select a held item to read or edit it.</p>
        {/if}
      {/snippet}
    </SplitPane>
  </div>
</div>
