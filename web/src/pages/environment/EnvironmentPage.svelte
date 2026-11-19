<script lang="ts">
  // Environment page (F-ENV-1, docs/02 §3.12): env list on the left (Global pinned first),
  // variables grid on the right. The whole grid is saved with one `setEnvironmentVariables`.
  import { untrack } from 'svelte';
  import { SvelteSet } from 'svelte/reactivity';
  import {
    environment as defaultStore,
    type Environment,
    type EnvironmentStore,
    type Variable,
  } from '$lib/stores/environment.svelte';

  let { store = defaultStore }: { store?: EnvironmentStore } = $props();

  /** Environment whose variables are open; null shows the first (Global). */
  let editingId = $state<string | null>(null);
  let rows = $state<Variable[]>([]);
  /** Row indexes whose secret value is revealed; per row, and gone when the page unmounts. */
  const revealed = new SvelteSet<number>();
  /** Name quoted by the last `BAD_INPUT`, so every row with it is highlighted. */
  let errorName = $state<string | null>(null);
  let newName = $state<string | null>(null);
  let renaming = $state<string | null>(null);

  const active = $derived(
    store.environments.find((e) => e.id === editingId) ?? store.environments[0] ?? null,
  );

  $effect(() => store.start());

  // The grid follows the open environment and the server data after every refetch.
  $effect(() => {
    const env = active;
    untrack(() => {
      rows = env ? env.variables.map((v) => ({ ...v })) : [];
      revealed.clear();
      errorName = null;
    });
  });

  function label(row: Variable, index: number): string {
    return row.name || `variable ${index + 1}`;
  }

  function invalid(row: Variable): boolean {
    return errorName !== null && row.name === errorName;
  }

  function toggleReveal(index: number) {
    if (!revealed.delete(index)) revealed.add(index);
  }

  function addRow() {
    rows.push({ name: '', value: '', secret: false });
  }

  function removeRow(index: number) {
    rows.splice(index, 1);
    errorName = null;
  }

  async function save(env: Environment) {
    const saved = await store.saveVariables(env.id, rows);
    if (saved) return;
    // `BAD_INPUT` quotes the offending name; point at that row.
    errorName = /"([^"]*)"/.exec(store.error ?? '')?.[1] ?? null;
  }

  async function create(event: SubmitEvent) {
    event.preventDefault();
    const name = newName?.trim();
    if (!name) return;
    if (await store.create(name)) {
      editingId = store.environments.find((e) => e.name === name)?.id ?? null;
      newName = null;
    }
  }

  async function rename(event: SubmitEvent, env: Environment) {
    event.preventDefault();
    if (renaming !== null && (await store.rename(env.id, renaming))) renaming = null;
  }
</script>

<div class="flex h-[calc(100vh-140px)] flex-col gap-2">
  <div class="flex min-h-0 flex-1 gap-2">
    <section
      aria-label="Environments"
      class="flex w-56 shrink-0 flex-col rounded-token border border-line"
    >
      <div class="flex items-center justify-between gap-2 border-b border-line px-2 py-1">
        <h2 class="text-xs tracking-wide text-muted uppercase">Environments</h2>
        <button
          type="button"
          class="rounded-token border border-line px-2 py-0.5 text-xs hover:bg-hover"
          onclick={() => (newName = '')}
        >
          New
        </button>
      </div>
      {#if newName !== null}
        <form class="flex gap-1 border-b border-line p-1" onsubmit={create}>
          <input
            class="min-w-0 flex-1 rounded-token border border-line bg-elevated px-1 text-xs"
            aria-label="New environment name"
            placeholder="staging"
            bind:value={newName}
            onkeydown={(e) => e.key === 'Escape' && (newName = null)}
          />
          <button type="submit" class="rounded-token border border-line px-1 text-xs">Create</button
          >
        </form>
      {/if}
      <ul class="min-h-0 flex-1 overflow-auto py-1">
        {#each store.environments as env (env.id)}
          <li class="px-1">
            <button
              type="button"
              aria-current={active?.id === env.id ? 'true' : undefined}
              class="flex w-full items-center gap-1 rounded-token px-2 py-1 text-left text-xs hover:bg-hover
              {active?.id === env.id ? 'bg-accent-soft text-accent' : ''}"
              onclick={() => (editingId = env.id)}
            >
              <span class="min-w-0 flex-1 truncate">{env.name}</span>
              {#if env.selected}<span class="text-xs text-muted">selected</span>{/if}
            </button>
          </li>
        {:else}
          <li class="px-2 py-1 text-xs text-muted">
            {store.loading ? 'Loading…' : 'No environments yet.'}
          </li>
        {/each}
      </ul>
    </section>

    {#if active}
      <section
        aria-label="Variables"
        class="flex min-w-0 flex-1 flex-col gap-2 rounded-token border border-line p-2"
      >
        <header class="flex items-center gap-2">
          {#if renaming !== null && !active.isGlobal}
            <form class="flex items-center gap-1" onsubmit={(e) => void rename(e, active)}>
              <input
                class="rounded-token border border-line bg-elevated px-2 py-0.5 text-sm"
                aria-label="Environment name"
                bind:value={renaming}
              />
              <button type="submit" class="rounded-token border border-line px-2 py-0.5 text-xs">
                Save name
              </button>
              <button
                type="button"
                class="rounded-token border border-line px-2 py-0.5 text-xs hover:bg-hover"
                onclick={() => (renaming = null)}
              >
                Cancel
              </button>
            </form>
          {:else}
            <h2 class="text-sm font-medium">{active.name}</h2>
            {#if !active.isGlobal}
              <button
                type="button"
                class="rounded-token border border-line px-2 py-0.5 text-xs hover:bg-hover"
                onclick={() => (renaming = active.name)}
              >
                Rename
              </button>
              <button
                type="button"
                class="rounded-token border border-line px-2 py-0.5 text-xs text-danger hover:bg-hover"
                onclick={() => void store.remove(active.id)}
              >
                Delete
              </button>
            {/if}
          {/if}
        </header>

        <div class="min-h-0 flex-1 overflow-auto">
          <table class="w-full text-xs" aria-label="Variables of {active.name}">
            <thead class="text-left text-muted">
              <tr>
                <th class="w-1/3 font-normal">Name</th>
                <th class="font-normal">Value</th>
                <th class="w-16 font-normal">Secret</th>
                <th class="w-8"><span class="sr-only">Remove</span></th>
              </tr>
            </thead>
            <tbody>
              {#each rows as row, index (index)}
                <tr>
                  <td class="pr-1">
                    <input
                      class="w-full rounded-token border border-line bg-elevated px-2 py-0.5 font-mono
                      {invalid(row) ? 'border-danger' : ''}"
                      aria-label="Name of {label(row, index)}"
                      aria-invalid={invalid(row)}
                      bind:value={row.name}
                    />
                  </td>
                  <td class="pr-1">
                    <div class="flex items-center gap-1">
                      <input
                        type={row.secret && !revealed.has(index) ? 'password' : 'text'}
                        class="w-full rounded-token border border-line bg-elevated px-2 py-0.5 font-mono"
                        aria-label="Value of {label(row, index)}"
                        bind:value={row.value}
                      />
                      {#if row.secret}
                        <button
                          type="button"
                          class="rounded-token border border-line px-1 text-xs hover:bg-hover"
                          aria-label="{revealed.has(index) ? 'Hide' : 'Reveal'} value of {label(
                            row,
                            index,
                          )}"
                          onclick={() => toggleReveal(index)}
                        >
                          {revealed.has(index) ? 'Hide' : 'Reveal'}
                        </button>
                      {/if}
                    </div>
                  </td>
                  <td class="text-center">
                    <input
                      type="checkbox"
                      aria-label="Secret {label(row, index)}"
                      bind:checked={row.secret}
                    />
                  </td>
                  <td class="text-center">
                    <button
                      type="button"
                      class="rounded-token px-1 text-muted hover:bg-hover"
                      aria-label="Remove {label(row, index)}"
                      onclick={() => removeRow(index)}
                    >
                      ×
                    </button>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
          {#if rows.length === 0}
            <p class="px-1 py-2 text-xs text-muted">No variables. Use “Add variable”.</p>
          {/if}
        </div>

        <footer class="flex items-center gap-2">
          <button
            type="button"
            class="rounded-token border border-line px-3 py-1 text-xs hover:bg-hover"
            onclick={addRow}
          >
            Add variable
          </button>
          <button
            type="button"
            class="rounded-token border border-accent bg-accent-soft px-3 py-1 text-xs text-accent"
            onclick={() => void save(active)}
          >
            Save
          </button>
        </footer>
      </section>
    {/if}
  </div>
  {#if store.error}
    <p role="alert" class="text-xs text-danger">{store.error}</p>
  {/if}
</div>
