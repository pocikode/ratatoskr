<script lang="ts" module>
  import type { CommandContext } from '$lib/commands/registry';

  export interface Tab {
    id: string;
    label: string;
    /** `--row-<name>` token for the colour tag. */
    color?: string | null;
  }
  export interface TabMenu {
    ids: string[];
    context?: CommandContext;
  }
</script>

<script lang="ts">
  import { DropdownMenu } from 'bits-ui';
  import ChevronDownIcon from '@lucide/svelte/icons/chevron-down';
  import PlusIcon from '@lucide/svelte/icons/plus';
  import XIcon from '@lucide/svelte/icons/x';
  import ContextMenu from '$lib/components/ContextMenu/ContextMenu.svelte';

  interface Props {
    tabs: Tab[];
    activeId: string | null;
    /** Tab in inline rename (double-click or the rename command). */
    renamingId?: string | null;
    label?: string;
    onselect: (id: string) => void;
    onclose: (id: string) => void;
    onrename: (id: string, name: string) => void;
    onstartrename: (id: string) => void;
    oncancelrename: () => void;
    /** Shows a "+" button. */
    onnew?: () => void;
    menu?: (id: string) => TabMenu;
  }

  let {
    tabs,
    activeId,
    renamingId = null,
    label = 'Open sessions',
    onselect,
    onclose,
    onrename,
    onstartrename,
    oncancelrename,
    onnew,
    menu,
  }: Props = $props();

  let list = $state<HTMLElement | null>(null);

  function onkeydown(e: KeyboardEvent) {
    const i = tabs.findIndex((t) => t.id === activeId);
    const to =
      e.key === 'ArrowRight'
        ? (i + 1) % tabs.length
        : e.key === 'ArrowLeft'
          ? (i - 1 + tabs.length) % tabs.length
          : e.key === 'Home'
            ? 0
            : e.key === 'End'
              ? tabs.length - 1
              : -1;
    const tab = tabs[to];
    if (!tab || renamingId) return;
    e.preventDefault();
    onselect(tab.id);
    queueMicrotask(() =>
      list?.querySelector<HTMLElement>(`[data-tab="${CSS.escape(tab.id)}"]`)?.focus(),
    );
  }

  // Deferred: a context menu closing right after the command returns focus to its trigger.
  function focusOnMount(node: HTMLInputElement) {
    const timer = setTimeout(() => {
      node.focus();
      node.select();
    });
    return { destroy: () => clearTimeout(timer) };
  }
</script>

<div class="flex items-center border-b border-line">
  <div
    bind:this={list}
    role="tablist"
    aria-label={label}
    tabindex="-1"
    class="flex min-w-0 flex-1 overflow-x-auto"
    {onkeydown}
  >
    {#each tabs as tab (tab.id)}
      {#snippet item()}
        <div
          class="group flex shrink-0 items-center gap-1 border-r border-line pr-1 pl-2
            {tab.id === activeId ? 'bg-hover' : ''}"
        >
          {#if tab.color}
            <span
              aria-hidden="true"
              class="size-2 rounded-full"
              style="background: var(--row-{tab.color})"
            ></span>
          {/if}
          {#if renamingId === tab.id}
            <input
              use:focusOnMount
              aria-label="Name"
              class="w-32 rounded-token border border-accent bg-subtle px-1"
              value={tab.label}
              onkeydown={(e) => {
                e.stopPropagation();
                if (e.key === 'Enter') onrename(tab.id, e.currentTarget.value);
                else if (e.key === 'Escape') oncancelrename();
              }}
              onblur={(e) => onrename(tab.id, e.currentTarget.value)}
            />
          {:else}
            <button
              type="button"
              role="tab"
              data-tab={tab.id}
              aria-selected={tab.id === activeId}
              tabindex={tab.id === activeId ? 0 : -1}
              class="max-w-48 truncate py-1 focus-visible:outline-2 focus-visible:outline-accent"
              onclick={() => onselect(tab.id)}
              ondblclick={() => onstartrename(tab.id)}
              onauxclick={(e) => e.button === 1 && onclose(tab.id)}
            >
              {tab.label}
            </button>
          {/if}
          <button
            type="button"
            tabindex="-1"
            aria-label="Close {tab.label}"
            class="rounded-token p-0.5 hover:bg-line"
            onclick={() => onclose(tab.id)}
          >
            <XIcon size={12} aria-hidden="true" />
          </button>
        </div>
      {/snippet}
      {#if menu}
        {@const m = menu(tab.id)}
        <ContextMenu ids={m.ids} context={m.context}>{@render item()}</ContextMenu>
      {:else}
        {@render item()}
      {/if}
    {/each}
  </div>
  {#if onnew}
    <button type="button" aria-label="New session" class="p-1.5 hover:bg-hover" onclick={onnew}>
      <PlusIcon size={14} aria-hidden="true" />
    </button>
  {/if}
  <DropdownMenu.Root>
    <DropdownMenu.Trigger
      aria-label="All tabs"
      disabled={tabs.length === 0}
      class="p-1.5 hover:bg-hover disabled:opacity-40"
    >
      <ChevronDownIcon size={14} aria-hidden="true" />
    </DropdownMenu.Trigger>
    <DropdownMenu.Portal>
      <DropdownMenu.Content
        align="end"
        class="z-50 max-h-80 min-w-40 overflow-y-auto rounded-token border border-line bg-elevated p-1"
        style="box-shadow: var(--shadow-overlay)"
      >
        {#each tabs as tab (tab.id)}
          <DropdownMenu.Item
            class="cursor-default truncate rounded-token px-2 py-1 outline-none data-highlighted:bg-hover"
            onSelect={() => onselect(tab.id)}
          >
            {tab.label}
          </DropdownMenu.Item>
        {/each}
      </DropdownMenu.Content>
    </DropdownMenu.Portal>
  </DropdownMenu.Root>
</div>
