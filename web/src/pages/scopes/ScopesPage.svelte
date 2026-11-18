<script lang="ts">
  // Scopes page (F-SCOPE-1, docs/02 §3.3): preset list on the left, editor on the right.
  // The editor saves through the contract's create/update mutation; the "matches N
  // requests" counter is `requests(scopeId){totalCount}` for the saved preset (T-091).
  import type { Client } from '@urql/core';
  import { untrack } from 'svelte';
  import { getApi } from '$lib/api/client';
  import { ScopesState, type RuleKind } from './scopes.svelte';

  interface Props {
    /** GraphQL client override for tests; the page otherwise uses the shared client. */
    client?: Client;
  }

  let { client }: Props = $props();

  // Dev-only mock backend (T-013): `?mock=scopes` or `localStorage['scopes.mock'] = '1'`.
  function mockRequested(): boolean {
    try {
      return (
        new URLSearchParams(window.location.search).get('mock') === 'scopes' ||
        localStorage.getItem('scopes.mock') === '1'
      );
    } catch {
      return false;
    }
  }

  const scopes = new ScopesState(untrack(() => client) ?? getApi().client);
  let adds = $state<Record<RuleKind, string>>({ allow: '', deny: '' });

  $effect(() => {
    let cancelled = false;
    void (async () => {
      if (!client && import.meta.env.DEV && mockRequested()) {
        const { createMockClient } = await import('$lib/api/mocks/exchange');
        if (cancelled) return;
        scopes.useClient(createMockClient());
      }
      if (!cancelled) await scopes.start();
    })();
    return () => {
      cancelled = true;
    };
  });

  function addRule(kind: RuleKind) {
    scopes.addRule(kind, adds[kind]);
    adds[kind] = '';
  }
</script>

<div class="flex h-[calc(100vh-140px)] gap-2">
  <section
    aria-label="Scope presets"
    class="flex w-64 shrink-0 flex-col rounded-token border border-line"
  >
    <div class="flex items-center justify-between gap-2 border-b border-line px-2 py-1">
      <h2 class="text-xs tracking-wide text-muted uppercase">Presets</h2>
      <button
        type="button"
        class="rounded-token border border-line px-2 py-0.5 text-xs hover:bg-hover"
        onclick={() => scopes.newPreset()}
      >
        New
      </button>
    </div>
    <ul class="min-h-0 flex-1 overflow-auto py-1">
      {#each scopes.presets as preset, index (preset.id)}
        <li class="flex items-center gap-1 px-1">
          <button
            type="button"
            aria-current={scopes.draft.id === preset.id ? 'true' : undefined}
            class="min-w-0 flex-1 truncate rounded-token px-2 py-1 text-left text-xs hover:bg-hover
              {scopes.draft.id === preset.id ? 'bg-accent-soft text-accent' : ''}"
            onclick={() => scopes.select(preset.id)}
          >
            {preset.name}
          </button>
          <button
            type="button"
            aria-label="Move {preset.name} up"
            disabled={index === 0}
            class="rounded-token px-1 text-xs text-muted hover:bg-hover disabled:opacity-30"
            onclick={() => void scopes.move(preset.id, -1)}
          >
            ↑
          </button>
          <button
            type="button"
            aria-label="Move {preset.name} down"
            disabled={index === scopes.presets.length - 1}
            class="rounded-token px-1 text-xs text-muted hover:bg-hover disabled:opacity-30"
            onclick={() => void scopes.move(preset.id, 1)}
          >
            ↓
          </button>
        </li>
      {:else}
        <li class="px-2 py-1 text-xs text-muted">No scope presets yet.</li>
      {/each}
    </ul>
  </section>

  <form
    aria-label="Scope editor"
    class="flex min-w-0 flex-1 flex-col gap-3 overflow-auto rounded-token border border-line p-3"
    onsubmit={(event) => {
      event.preventDefault();
      void scopes.save();
    }}
  >
    <label class="flex max-w-md flex-col gap-1">
      <span class="text-xs text-muted">Name</span>
      <input
        class="rounded-token border border-line bg-elevated px-2 py-1 text-sm"
        placeholder="Example app"
        required
        bind:value={scopes.draft.name}
      />
    </label>

    {@render ruleList('allow', 'Allow')}
    {@render ruleList('deny', 'Deny')}

    <div class="mt-auto flex flex-wrap items-center gap-2">
      <button
        type="submit"
        disabled={scopes.busy}
        class="rounded-token border border-accent bg-accent-soft px-3 py-1 text-xs text-accent
          disabled:opacity-50"
      >
        Save
      </button>
      <button
        type="button"
        disabled={!scopes.draft.id}
        class="rounded-token border border-line px-3 py-1 text-xs hover:bg-hover disabled:opacity-30"
        onclick={() => void scopes.duplicate()}
      >
        Duplicate
      </button>
      <button
        type="button"
        disabled={!scopes.draft.id}
        class="rounded-token border border-line px-3 py-1 text-xs text-danger hover:bg-hover
          disabled:opacity-30"
        onclick={() => void scopes.remove()}
      >
        Delete
      </button>
      {#if scopes.count !== null}
        <p class="text-xs text-muted" aria-live="polite">Matches {scopes.count} requests</p>
      {/if}
    </div>

    {#if scopes.message}
      <p role="alert" class="text-xs text-danger">{scopes.message}</p>
    {/if}
  </form>
</div>

{#snippet ruleList(kind: RuleKind, label: string)}
  <fieldset class="flex flex-col gap-1">
    <legend class="text-xs text-muted">{label} list</legend>
    <ul class="flex flex-wrap gap-1">
      {#each scopes.draft[kind] as rule, index (index)}
        <li
          class="flex items-center gap-1 rounded-token border border-line bg-subtle px-1.5 py-0.5
            font-mono text-xs"
        >
          {rule}
          <button
            type="button"
            aria-label="Remove {rule}"
            class="text-muted hover:text-danger"
            onclick={() => scopes.removeRule(kind, index)}
          >
            ×
          </button>
        </li>
      {:else}
        <li class="text-xs text-muted">
          {kind === 'allow' ? 'Empty: every host is in scope.' : 'Nothing is excluded.'}
        </li>
      {/each}
    </ul>
    <input
      class="max-w-md rounded-token border border-line bg-elevated px-2 py-1 font-mono text-xs"
      aria-label="Add to the {label.toLowerCase()} list"
      placeholder="host[:port][/path]"
      value={adds[kind]}
      oninput={(event) => (adds[kind] = event.currentTarget.value)}
      onkeydown={(event) => {
        if (event.key !== 'Enter') return;
        event.preventDefault();
        addRule(kind);
      }}
    />
  </fieldset>
{/snippet}
