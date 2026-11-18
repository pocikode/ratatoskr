<script lang="ts">
  import type { Client } from '@urql/core';
  import { onDestroy, untrack } from 'svelte';
  import { getApi } from '$lib/api/client';
  import { registry } from '$lib/commands';
  import { projects } from '$lib/stores/project.svelte';
  import QueryBar from '$lib/components/QueryBar/QueryBar.svelte';
  import ScopeSelector from '$lib/components/ScopeSelector.svelte';
  import SplitPane from '$lib/components/SplitPane/SplitPane.svelte';
  import DeleteNodeDialog from './DeleteNodeDialog.svelte';
  import SitemapTraffic from './SitemapTraffic.svelte';
  import SitemapTree from './SitemapTree.svelte';
  import { SitemapState } from './sitemap.svelte';

  let { client }: { client?: Client } = $props();
  const state = new SitemapState(untrack(() => client) ?? getApi().client);
  let scopeChoice = $derived(state.scopeId);
  $effect(() => untrack(() => state.start()));
  $effect(() => projects.onReset(() => state.reset()));
  onDestroy(
    registry.register(
      {
        id: 'sitemap.copy',
        title: 'Copy URL',
        group: 'Sitemap',
        when: () => !!state.target,
        run: () => state.copyUrl(),
      },
      {
        id: 'sitemap.scope',
        title: 'Add host to scope',
        group: 'Sitemap',
        when: () => !!state.target && !!state.scopeId,
        run: () => state.addHostToScope(),
      },
      {
        id: 'sitemap.delete',
        title: 'Delete node…',
        group: 'Sitemap',
        when: () => !!state.target,
        run: () => {
          if (state.target) {
            state.message = null;
            state.confirmation = { node: state.target, scopeId: state.scopeId };
          }
        },
      },
    ),
  );
</script>

<div class="flex h-[calc(100vh-140px)] flex-col gap-2">
  <QueryBar
    bind:value={state.query}
    label="HTTPQL query"
    error={state.error}
    onsubmit={(text) => {
      void state.apply(text);
    }}
    oncomplete={(text, cursor) => state.complete(text, cursor)}
  >
    {#snippet leading()}
      <ScopeSelector
        presets={state.scopes}
        bind:value={scopeChoice}
        onchange={async (id) => {
          await state.apply(state.applied, id);
          scopeChoice = state.scopeId;
        }}
      />
    {/snippet}
  </QueryBar>
  {#if state.message && !state.confirmation}<p role="alert" class="text-xs text-danger">
      {state.message}
    </p>{/if}
  <div class="min-h-0 flex-1">
    <SplitPane defaultSizes={[30, 70]} label="Resize sitemap and requests">
      {#snippet first()}<SitemapTree {state} />{/snippet}
      {#snippet second()}<SitemapTraffic {state} />{/snippet}
    </SplitPane>
  </div>
</div>
<DeleteNodeDialog {state} />
