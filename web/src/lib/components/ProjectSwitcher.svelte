<script lang="ts">
  import { Combobox } from 'bits-ui';
  import { registry } from '$lib/commands';
  import { router } from '$lib/router/router.svelte';
  import {
    projectCommands,
    projects as defaultStore,
    type ProjectStore,
  } from '$lib/stores/project.svelte';
  import { iconFor } from '$lib/utils/icons';

  let { store = defaultStore, start = true }: { store?: ProjectStore; start?: boolean } = $props();

  const Folder = iconFor('folder')!;
  let search = $state('');
  let open = $state(false);

  $effect(() => (start ? store.start() : undefined));
  // Palette entries follow the project list.
  $effect(() => registry.register(...projectCommands(store, (id) => router.navigate(id))));

  const shown = $derived(
    store.projects.filter((p) => p.name.toLowerCase().includes(search.trim().toLowerCase())),
  );
</script>

<Combobox.Root
  type="single"
  bind:open
  value={store.current?.id ?? ''}
  inputValue={open ? search : (store.current?.name ?? '')}
  onValueChange={(id) => id && id !== store.current?.id && void store.select(id)}
  onOpenChange={(o) => {
    if (!o) search = '';
  }}
>
  <div class="relative flex items-center">
    <Folder
      size={14}
      class="pointer-events-none absolute left-2 {store.current?.isTemporary
        ? 'text-warn'
        : 'text-muted'}"
      aria-hidden="true"
    />
    <Combobox.Input
      aria-label="Project"
      placeholder="Project"
      class="w-44 rounded-token border border-line bg-subtle py-1 pr-2 pl-7 hover:bg-hover"
      oninput={(e) => (search = e.currentTarget.value)}
      onfocus={() => (open = true)}
    />
  </div>
  <Combobox.Portal>
    <Combobox.Content
      sideOffset={4}
      class="z-50 max-h-72 min-w-56 overflow-auto rounded-token border border-line bg-elevated p-1 shadow-lg"
    >
      {#each shown as p (p.id)}
        <Combobox.Item
          value={p.id}
          label={p.name}
          class="flex cursor-default items-center justify-between gap-3 rounded-token px-2 py-1 outline-none data-highlighted:bg-hover"
        >
          <span class="truncate">{p.name}</span>
          {#if p.isTemporary}
            <span class="rounded-token border border-warn px-1 text-xs text-warn">temporary</span>
          {:else if p.isSelected}<span aria-label="current">✓</span>{/if}
        </Combobox.Item>
      {:else}
        <p class="px-2 py-1 text-muted">No projects match</p>
      {/each}
    </Combobox.Content>
  </Combobox.Portal>
</Combobox.Root>
