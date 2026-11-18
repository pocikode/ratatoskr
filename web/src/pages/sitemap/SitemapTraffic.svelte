<script lang="ts">
  import MessageViewer from '$lib/components/MessageViewer/MessageViewer.svelte';
  import SplitPane from '$lib/components/SplitPane/SplitPane.svelte';
  import TrafficTable from '$lib/components/TrafficTable/TrafficTable.svelte';
  import { historyColumns, rowColor } from '../http-history/columns';
  import type { SitemapState } from './sitemap.svelte';

  let { state }: { state: SitemapState } = $props();
  $effect(() => {
    void state.showDetail(state.selection.anchor);
  });
</script>

<SplitPane direction="vertical" defaultSizes={[55, 45]} label="Resize table and viewers">
  {#snippet first()}
    <div class="flex min-h-0 flex-1 flex-col">
      <p class="px-2 py-1 text-xs text-muted" aria-live="polite">
        {state.tableLoading ? 'Loading…' : `${state.total} matching requests`}
      </p>
      <div class="min-h-0 flex-1">
        <TrafficTable
          rows={state.rows}
          columns={historyColumns}
          rowKey={(row) => row.id}
          selection={state.selection}
          sort={state.sort}
          color={rowColor}
          label="Sitemap requests"
          onsortchange={(sort) => {
            state.sort = sort;
            void state.loadRows();
          }}
        >
          {#snippet empty()}<span
              >{state.selected ? 'No requests match.' : 'Select a sitemap node.'}</span
            >{/snippet}
        </TrafficTable>
      </div>
      {#if state.cursor}
        <button
          type="button"
          class="border-t border-line px-2 py-1 text-xs hover:bg-hover"
          disabled={state.tableLoading}
          onclick={() => state.loadRows(true)}>Load more requests</button
        >
      {/if}
    </div>
  {/snippet}
  {#snippet second()}
    <SplitPane direction="horizontal" label="Resize request and response">
      {#snippet first()}
        {#if state.detail}
          <MessageViewer
            raw={state.detail.request}
            title={state.detail.requestTruncated ? 'Request · first 4 MiB' : 'Request'}
            modes={['raw', 'pretty', 'hex', 'table']}
          />
        {:else}
          <p class="p-3 text-xs text-muted">
            {state.detailLoading ? 'Loading…' : 'Select a request to view it.'}
          </p>
        {/if}
      {/snippet}
      {#snippet second()}
        {#if state.detail?.response}
          <MessageViewer
            raw={state.detail.response}
            title={state.detail.responseTruncated ? 'Response · first 4 MiB' : 'Response'}
            modes={['raw', 'pretty', 'hex', 'table']}
          />
        {:else}
          <p class="p-3 text-xs text-muted">
            {state.detail ? 'No response for this request.' : 'Select a request to view it.'}
          </p>
        {/if}
      {/snippet}
    </SplitPane>
  {/snippet}
</SplitPane>
