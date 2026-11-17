<script lang="ts">
  import { Dialog } from 'bits-ui';
  import { registry } from '$lib/commands';
  import { filterCommands } from '$lib/commands/registry';
  import { ui } from '$lib/stores/ui.svelte';
  import { iconFor } from '$lib/utils/icons';

  let query = $state('');
  let active = $state(0);
  let input = $state<HTMLInputElement | null>(null);

  const results = $derived(
    ui.paletteOpen ? filterCommands(registry.available(), query).slice(0, 50) : [],
  );
  const activeIndex = $derived(Math.min(active, Math.max(results.length - 1, 0)));

  function close() {
    ui.paletteOpen = false;
    query = '';
    active = 0;
  }

  async function runAt(i: number) {
    const cmd = results[i];
    if (!cmd) return;
    close();
    await registry.run(cmd.id);
  }

  function onkeydown(e: KeyboardEvent) {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      active = (activeIndex + 1) % Math.max(results.length, 1);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      active = (activeIndex - 1 + results.length) % Math.max(results.length, 1);
    } else if (e.key === 'Enter' && !e.isComposing) {
      e.preventDefault();
      void runAt(activeIndex);
    }
  }
</script>

<Dialog.Root
  bind:open={ui.paletteOpen}
  onOpenChange={(o) => {
    if (!o) close();
  }}
>
  <Dialog.Portal>
    <Dialog.Overlay class="fixed inset-0 z-40" style="background: var(--scrim)" />
    <Dialog.Content
      class="fixed top-[15%] left-1/2 z-50 w-[560px] max-w-[92vw] -translate-x-1/2 overflow-hidden rounded-token border border-line bg-elevated"
      style="box-shadow: var(--shadow-overlay)"
      aria-describedby={undefined}
      onOpenAutoFocus={(e) => {
        e.preventDefault();
        input?.focus();
      }}
    >
      <Dialog.Title class="sr-only">Command palette</Dialog.Title>
      <input
        type="text"
        role="combobox"
        aria-label="Search commands"
        aria-expanded="true"
        aria-controls="palette-list"
        aria-activedescendant={results[activeIndex]
          ? `palette-${results[activeIndex].id}`
          : undefined}
        placeholder="Type a command…"
        autocomplete="off"
        class="w-full border-b border-line bg-transparent px-3 py-2.5 outline-none"
        bind:this={input}
        bind:value={query}
        oninput={() => (active = 0)}
        {onkeydown}
      />
      <ul
        id="palette-list"
        role="listbox"
        aria-label="Commands"
        class="max-h-80 overflow-y-auto p-1"
      >
        {#each results as cmd, i (cmd.id)}
          {@const Icon = iconFor(cmd.icon)}
          <!-- Options are driven from the combobox input (aria-activedescendant); clicks are a pointer convenience. -->
          <!-- svelte-ignore a11y_click_events_have_key_events -->
          <li
            id={`palette-${cmd.id}`}
            role="option"
            aria-selected={i === activeIndex}
            class="flex cursor-default items-center gap-2 rounded-token px-2 py-1.5
              {i === activeIndex ? 'bg-accent-soft text-accent' : ''}"
            onclick={() => runAt(i)}
            onmousemove={() => (active = i)}
          >
            {#if Icon}<Icon size={14} aria-hidden="true" />{/if}
            <span class="flex-1 truncate">{cmd.title}</span>
            {#if cmd.group}<span class="text-xs text-muted">{cmd.group}</span>{/if}
          </li>
        {:else}
          <li class="px-2 py-3 text-center text-muted" role="presentation">No matching commands</li>
        {/each}
      </ul>
    </Dialog.Content>
  </Dialog.Portal>
</Dialog.Root>
