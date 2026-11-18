<script lang="ts">
  import type { Client } from '@urql/core';
  import { HistoryCompletionsDocument } from '$lib/api/generated/graphql';
  import { getApi } from '$lib/api/client';
  import { untrack } from 'svelte';
  import { registry } from '$lib/commands';
  import MessageViewer from '$lib/components/MessageViewer/MessageViewer.svelte';
  import PresetChips from '$lib/components/PresetChips.svelte';
  import QueryBar from '$lib/components/QueryBar/QueryBar.svelte';
  import ScopeSelector from '$lib/components/ScopeSelector.svelte';
  import SplitPane from '$lib/components/SplitPane/SplitPane.svelte';
  import TrafficTable from '$lib/components/TrafficTable/TrafficTable.svelte';
  import { COLORS, historyColumns, rowColor, type HistoryRow } from './columns';
  import { historyCommands } from './commands';
  import { HistoryState } from './history.svelte';
  import NoteDialog from './NoteDialog.svelte';
  import RowContextMenu from './RowContextMenu.svelte';

  interface Props {
    /** GraphQL client override for tests; the page otherwise uses the shared client. */
    client?: Client;
  }

  let { client }: Props = $props();

  // Dev-only mock backend (T-013): `?mock=history` or `localStorage['history.mock'] = '1'`.
  function mockRequested(): boolean {
    try {
      return (
        new URLSearchParams(window.location.search).get('mock') === 'history' ||
        localStorage.getItem('history.mock') === '1'
      );
    } catch {
      return false;
    }
  }

  const history = new HistoryState(untrack(() => client) ?? getApi().client);

  const MENU_COMMANDS = [
    'history.copy.url',
    'history.copy.curl',
    ...COLORS.map((color) => `history.color.${color.toLowerCase()}`),
    'history.color.none',
    'history.note.edit',
    'history.filter.host',
    'history.scope.add',
    'history.scope.exclude',
    'history.delete',
  ];

  let menu = $state<{ x: number; y: number; items: { id: string; title: string }[] } | null>(null);
  let noteRow = $state<HistoryRow | null>(null);

  const requestTitle = $derived(
    history.detail?.requestTruncated ? 'Request · first 4 MiB' : 'Request',
  );
  const responseTitle = $derived(
    history.detail?.responseTruncated ? 'Response · first 4 MiB' : 'Response',
  );

  $effect(() => {
    let stop = () => {};
    let cancelled = false;
    void (async () => {
      if (!client && import.meta.env.DEV && mockRequested()) {
        const { createMockClient } = await import('$lib/api/mocks/exchange');
        if (cancelled) return;
        history.useClient(createMockClient({ count: 500 }));
      }
      if (!cancelled) stop = untrack(() => history.start());
    })();
    return () => {
      cancelled = true;
      stop();
      history.stop();
    };
  });

  $effect(() =>
    registry.register(
      ...historyCommands(history, {
        copy,
        editNote: (row) => (noteRow = row),
      }),
    ),
  );

  // The viewers follow the row last clicked or moved to (Selection.anchor).
  $effect(() => {
    void history.showDetail(history.selection.anchor);
  });

  async function complete(text: string, cursor: number) {
    const result = await history.client
      .query(HistoryCompletionsDocument, { text, cursor }, { requestPolicy: 'network-only' })
      .toPromise();
    return result.data?.queryCompletions ?? null;
  }

  async function copy(text: string) {
    if (!navigator.clipboard) {
      history.message = 'Clipboard unavailable';
      return;
    }
    try {
      await navigator.clipboard.writeText(text);
      history.message = null;
    } catch {
      history.message = 'Clipboard unavailable';
    }
  }

  function menuPoint(event: MouseEvent | KeyboardEvent): { x: number; y: number } {
    if (event instanceof MouseEvent) return { x: event.clientX, y: event.clientY };
    const rect = (event.target as HTMLElement | null)?.getBoundingClientRect();
    return { x: rect?.left ?? 0, y: rect?.bottom ?? 0 };
  }

  function openMenu(row: HistoryRow, event: MouseEvent | KeyboardEvent) {
    const ids = history.rows.map((other) => other.id);
    if (!history.selection.has(row.id)) history.selection.click(row.id, ids);
    const items = MENU_COMMANDS.map((id) => registry.get(id)).flatMap((command) =>
      command && (command.when?.({}) ?? true) ? [{ id: command.id, title: command.title }] : [],
    );
    menu = { ...menuPoint(event), items };
  }
</script>

<div class="flex h-[calc(100vh-140px)] flex-col gap-2">
  <div class="flex items-start gap-2">
    <div class="min-w-0 flex-1">
      <QueryBar
        bind:value={history.query}
        label="HTTPQL query"
        error={history.error}
        history={history.history}
        onsubmit={(text) => history.apply(text)}
        oncomplete={complete}
      >
        {#snippet leading()}
          <ScopeSelector
            presets={history.scopes}
            value={history.scopeId}
            onchange={(id) => history.setScope(id)}
          />
        {/snippet}
        {#snippet trailing()}
          <PresetChips
            presets={history.presets}
            active={history.activePreset}
            onapply={(preset) => history.applyPreset(preset.name)}
          />
        {/snippet}
      </QueryBar>
    </div>
    <button
      type="button"
      aria-pressed={history.layout === 'right'}
      class="shrink-0 rounded-token border border-line px-2 py-1 text-xs text-muted
        hover:bg-hover"
      onclick={() => history.toggleLayout()}
    >
      Viewers {history.layout === 'right' ? 'right' : 'below'}
    </button>
    <p class="shrink-0 py-1.5 text-xs text-muted" aria-live="polite">
      {history.loading ? 'Loading…' : `${history.total} matching`}
    </p>
  </div>

  {#if history.message}
    <p role="alert" class="text-xs text-danger">{history.message}</p>
  {/if}

  <div class="min-h-0 flex-1">
    {#key history.layout}
      <SplitPane
        direction={history.layout === 'right' ? 'horizontal' : 'vertical'}
        defaultSizes={[55, 45]}
        label="Resize table and viewers"
      >
        {#snippet first()}
          <div class="relative flex min-h-0 flex-col">
            <TrafficTable
              rows={history.rows}
              columns={historyColumns}
              rowKey={(row) => row.id}
              selection={history.selection}
              config={history.config}
              sort={history.sort}
              label="HTTP History"
              color={rowColor}
              onconfigchange={(config) => history.setConfig(config)}
              onsortchange={(sort) => history.setSort(sort)}
              onrowopen={(row) =>
                history.selection.click(
                  row.id,
                  history.rows.map((other) => other.id),
                )}
              onrowcontextmenu={openMenu}
            />
            <div aria-live="polite">
              {#if history.pending.length > 0}
                <button
                  type="button"
                  class="absolute top-8 left-1/2 z-20 -translate-x-1/2 rounded-full border
                  border-accent bg-elevated px-2.5 py-0.5 text-xs text-accent"
                  onclick={() => history.applyPending()}
                >
                  ↓ {history.pending.length} new
                </button>
              {/if}
            </div>
          </div>
        {/snippet}

        {#snippet second()}
          <SplitPane direction="horizontal" label="Resize request and response">
            {#snippet first()}
              {#if history.detail}
                <MessageViewer
                  raw={history.detail.request}
                  title={requestTitle}
                  modes={['raw', 'pretty', 'hex', 'table']}
                />
              {:else}
                <p class="flex h-full items-center justify-center border border-line text-muted">
                  {history.detailLoading ? 'Loading…' : 'Select a request to view it.'}
                </p>
              {/if}
            {/snippet}
            {#snippet second()}
              {#if history.detail?.response}
                <MessageViewer
                  raw={history.detail.response}
                  title={responseTitle}
                  modes={['raw', 'pretty', 'hex', 'table']}
                />
              {:else}
                <p class="flex h-full items-center justify-center border border-line text-muted">
                  {history.detail
                    ? 'No response for this request.'
                    : 'Select a request to view it.'}
                </p>
              {/if}
            {/snippet}
          </SplitPane>
        {/snippet}
      </SplitPane>
    {/key}
  </div>
</div>

<RowContextMenu
  open={menu !== null}
  x={menu?.x ?? 0}
  y={menu?.y ?? 0}
  items={menu?.items ?? []}
  onrun={(id) => void registry.run(id)}
  onclose={() => (menu = null)}
/>

<NoteDialog
  row={noteRow}
  onsave={(note) => {
    const id = noteRow?.id;
    noteRow = null;
    if (id) void history.setNote(id, note);
  }}
  onclose={() => (noteRow = null)}
/>
