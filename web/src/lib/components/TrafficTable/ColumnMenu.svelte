<script lang="ts">
  import { Popover } from 'bits-ui';
  import ChevronDownIcon from '@lucide/svelte/icons/chevron-down';
  import ChevronUpIcon from '@lucide/svelte/icons/chevron-up';
  import ColumnsIcon from '@lucide/svelte/icons/columns-3';
  import { moveColumn, toggleColumn } from './columns';
  import type { ColumnConfig } from './types';

  let {
    columns,
    config,
    onconfig,
  }: {
    columns: readonly { id: string; title: string }[];
    config: ColumnConfig;
    onconfig: (config: ColumnConfig) => void;
  } = $props();

  const titles = $derived(new Map(columns.map((c) => [c.id, c.title])));
  const visibleCount = $derived(config.order.length - config.hidden.length);
  const lastVisible = (id: string) => visibleCount === 1 && !config.hidden.includes(id);
</script>

<Popover.Root>
  <Popover.Trigger
    aria-label="Columns"
    title="Show, hide or reorder columns"
    class="flex items-center gap-1 rounded-token px-1.5 py-0.5 text-muted hover:bg-hover hover:text-fg"
  >
    <ColumnsIcon size={14} aria-hidden="true" />
    <span class="text-xs">Columns</span>
  </Popover.Trigger>
  <Popover.Portal>
    <Popover.Content
      sideOffset={4}
      align="end"
      class="z-50 w-64 rounded-token border border-line bg-elevated p-1"
      style="box-shadow: var(--shadow-overlay)"
    >
      <p class="px-2 py-1 text-xs tracking-wider text-muted uppercase">Columns</p>
      <ul>
        {#each config.order as id, i (id)}
          <li class="flex items-center gap-1 rounded-token px-2 py-1 hover:bg-hover">
            <input
              type="checkbox"
              class="accent-accent"
              aria-label={titles.get(id) ?? id}
              checked={!config.hidden.includes(id)}
              disabled={lastVisible(id)}
              onchange={() => onconfig(toggleColumn(config, id))}
            />
            <span class="flex-1 truncate">{titles.get(id) ?? id}</span>
            <button
              type="button"
              aria-label={`Move ${titles.get(id) ?? id} left`}
              disabled={i === 0}
              class="rounded-token p-0.5 text-muted hover:bg-subtle disabled:opacity-30"
              onclick={() => onconfig(moveColumn(config, id, -1))}
            >
              <ChevronUpIcon size={12} aria-hidden="true" class="-rotate-90" />
            </button>
            <button
              type="button"
              aria-label={`Move ${titles.get(id) ?? id} right`}
              disabled={i === config.order.length - 1}
              class="rounded-token p-0.5 text-muted hover:bg-subtle disabled:opacity-30"
              onclick={() => onconfig(moveColumn(config, id, 1))}
            >
              <ChevronDownIcon size={12} aria-hidden="true" class="-rotate-90" />
            </button>
          </li>
        {/each}
      </ul>
    </Popover.Content>
  </Popover.Portal>
</Popover.Root>
