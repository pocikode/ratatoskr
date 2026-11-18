<script lang="ts">
  // Row context menu (F-HIST-3). Page-local v1: T-022 replaces it with the shared
  // `ContextMenu` built from the same registry entries.
  import { tick } from 'svelte';

  interface Props {
    open: boolean;
    x: number;
    y: number;
    items: readonly { id: string; title: string }[];
    onrun: (id: string) => void;
    onclose: () => void;
  }

  let { open, x, y, items, onrun, onclose }: Props = $props();

  let menu = $state<HTMLDivElement | null>(null);
  let index = 0;

  $effect(() => {
    if (!open) return;
    const restore = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    index = 0;
    void tick().then(() => menuItems()[0]?.focus());
    const outside = (event: PointerEvent) => {
      if (!menu?.contains(event.target as Node)) onclose();
    };
    document.addEventListener('pointerdown', outside, true);
    return () => {
      document.removeEventListener('pointerdown', outside, true);
      restore?.focus();
    };
  });

  function menuItems(): HTMLElement[] {
    return [...(menu?.querySelectorAll<HTMLElement>('[role="menuitem"]') ?? [])];
  }

  function onkeydown(event: KeyboardEvent) {
    if (event.key === 'Escape') {
      event.preventDefault();
      onclose();
    } else if (event.key === 'Tab') {
      onclose();
    } else if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault();
      const all = menuItems();
      if (all.length === 0) return;
      index = (index + (event.key === 'ArrowDown' ? 1 : all.length - 1)) % all.length;
      all[index]?.focus();
    }
  }
</script>

{#if open}
  <div
    bind:this={menu}
    role="menu"
    aria-label="Request actions"
    tabindex="-1"
    class="fixed z-50 w-56 overflow-y-auto rounded-token border border-line bg-elevated p-1
      text-xs"
    style:left="{Math.min(x, window.innerWidth - 232)}px"
    style:top="{Math.min(y, window.innerHeight - 320)}px"
    style:max-height="60vh"
    style:box-shadow="var(--shadow-overlay)"
    {onkeydown}
  >
    {#each items as item (item.id)}
      <button
        type="button"
        role="menuitem"
        tabindex="-1"
        class="flex w-full items-center rounded-token px-2 py-1 text-left hover:bg-hover
          focus:bg-hover focus:outline-none"
        onclick={() => {
          onrun(item.id);
          onclose();
        }}
      >
        {item.title}
      </button>
    {/each}
  </div>
{/if}
