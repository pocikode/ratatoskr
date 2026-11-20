<script lang="ts">
  // What gets held (F-INT-1): HTTPQL condition, scope preset and the status code of a drop.
  // Each control applies through the store and shows the server's error beside it.
  import { InterceptScopePresetsDocument } from '$lib/api/generated/graphql';
  import QueryBar from '$lib/components/QueryBar/QueryBar.svelte';
  import ScopeSelector from '$lib/components/ScopeSelector.svelte';
  import { byteToDoc, type QueryError } from '$lib/query/httpql';
  import type { InterceptStore } from '$lib/stores/intercept.svelte';

  let { store }: { store: InterceptStore } = $props();

  let queryText = $state('');
  let queryError = $state<QueryError | null>(null);
  const appliedQuery = $derived(store.settings?.query ?? '');
  $effect(() => {
    queryText = appliedQuery;
    queryError = null;
  });

  async function applyQuery(text: string) {
    const r = await store.update({ query: text });
    queryError = r.error
      ? {
          message: r.error,
          from: r.span ? byteToDoc(text, r.span.start) : 0,
          to: r.span ? byteToDoc(text, r.span.end) : text.length,
        }
      : null;
  }

  // Scope presets belong to the project: cleared and refetched on every project switch.
  let presets = $state.raw<{ id: string; name: string }[]>([]);
  let scopeError = $state<string | null>(null);
  $effect(() => {
    void store.epoch;
    presets = [];
    scopeError = null;
    let cancelled = false;
    // The store's query is torn down by a project switch, so an old project's list never lands.
    void store.query(InterceptScopePresetsDocument).then((r) => {
      if (!cancelled && r) presets = r.data?.scopePresets ?? [];
    });
    return () => {
      cancelled = true;
    };
  });

  async function applyScope(id: string | null) {
    scopeError = (await store.update({ scopeId: id })).error;
  }

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
</script>

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
          onchange={applyScope}
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
{#if scopeError}<p role="alert" class="text-xs text-danger">{scopeError}</p>{/if}
{#if dropError}<p role="alert" class="text-xs text-danger">{dropError}</p>{/if}
