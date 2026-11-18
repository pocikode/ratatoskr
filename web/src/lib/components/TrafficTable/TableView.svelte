<script lang="ts" generics="Row">
  import type { Snippet } from 'svelte';
  import type { Selection } from './selection.svelte';
  import type { ColumnConfig, RowColor, SortState, TrafficColumn } from './types';

  interface Props {
    /** Rows in display order; the table never sorts them itself. */
    rows: Row[];
    /** Visible columns in display order. */
    columns: readonly TrafficColumn<Row>[];
    rowKey: (row: Row) => string;
    /** Prefix for row ids, referenced by the grid's `aria-activedescendant`. */
    idPrefix: string;
    config: ColumnConfig;
    sort: SortState | null;
    /** Keyboard row, kept in view when it moves. */
    active: number;
    /** Rows per page for PageUp/PageDown; measured by this component. */
    pageRows?: number;
    selection?: Selection;
    color?: (row: Row) => RowColor | null;
    onsort: (column: TrafficColumn<Row>) => void;
    /** Fired on every drag step and once with `commit` when the drag ends. */
    onwidth: (id: string, width: number, commit: boolean) => void;
    empty?: Snippet;
  }

  let {
    rows,
    columns,
    rowKey,
    idPrefix,
    config,
    sort,
    active,
    pageRows = $bindable(10),
    selection,
    color,
    onsort,
    onwidth,
    empty,
  }: Props = $props();

  const OVERSCAN = 8;
  const DEFAULT_ROW_H = 26;

  let viewport = $state<HTMLDivElement | null>(null);
  let scrollTop = $state(0);
  let viewH = $state(0);
  let rowH = $state(DEFAULT_ROW_H);

  // jsdom has no ResizeObserver; tests then use the 600px fallback in the window maths.
  $effect(() => {
    const el = viewport;
    if (!el || typeof ResizeObserver === 'undefined') return;
    const observer = new ResizeObserver(() => (viewH = el.clientHeight));
    observer.observe(el);
    viewH = el.clientHeight;
    return () => observer.disconnect();
  });

  // Row height comes from the density token; re-read when the table remounts (theme/density).
  $effect(() => {
    if (viewport) {
      const token = parseFloat(getComputedStyle(viewport).getPropertyValue('--row-h'));
      rowH = token > 0 ? token : DEFAULT_ROW_H;
    }
  });

  const template = $derived(columns.map((c) => `${config.widths[c.id] ?? c.width}px`).join(' '));
  const totalWidth = $derived(
    columns.reduce((sum, c) => sum + (config.widths[c.id] ?? c.width), 0),
  );
  // The header sits inside the scroller (sticky), so the row window starts one row further down.
  const bodyHeight = $derived(Math.max(rowH, (viewH || 600) - rowH));
  const bodyScroll = $derived(Math.max(0, scrollTop - rowH));
  const start = $derived(Math.max(0, Math.floor(bodyScroll / rowH) - OVERSCAN));
  const end = $derived(
    Math.min(rows.length, Math.ceil((bodyScroll + bodyHeight) / rowH) + OVERSCAN),
  );

  $effect(() => {
    pageRows = Math.max(1, Math.floor(bodyHeight / rowH) - 1);
  });

  // Follow the keyboard row.
  $effect(() => {
    const index = active;
    const el = viewport;
    if (!el || index < 0) return;
    const top = rowH + index * rowH;
    if (top < el.scrollTop + rowH) el.scrollTop = top - rowH;
    else if (top + rowH > el.scrollTop + el.clientHeight)
      el.scrollTop = top + rowH - el.clientHeight;
  });

  function startResize(column: TrafficColumn<Row>, e: PointerEvent) {
    e.preventDefault();
    const handle = e.currentTarget as HTMLElement;
    const startX = e.clientX;
    const startWidth = config.widths[column.id] ?? column.width;
    let width = startWidth;
    const move = (ev: PointerEvent) => {
      width = startWidth + ev.clientX - startX;
      onwidth(column.id, width, false);
    };
    const stop = () => {
      handle.removeEventListener('pointermove', move);
      handle.removeEventListener('pointerup', stop);
      handle.removeEventListener('pointercancel', stop);
      onwidth(column.id, width, true);
    };
    handle.setPointerCapture?.(e.pointerId);
    handle.addEventListener('pointermove', move);
    handle.addEventListener('pointerup', stop);
    handle.addEventListener('pointercancel', stop);
  }
</script>

<div
  bind:this={viewport}
  class="min-h-0 flex-1 overflow-auto"
  onscroll={(e) => (scrollTop = e.currentTarget.scrollTop)}
>
  <div style:min-width="{totalWidth}px">
    <div
      role="row"
      class="sticky top-0 z-10 grid border-b border-line bg-elevated text-xs font-semibold text-muted select-none"
      style:grid-template-columns={template}
      style:height="{rowH}px"
    >
      {#each columns as column (column.id)}
        <div
          role="columnheader"
          aria-sort={sort?.columnId === column.id
            ? sort.direction === 'asc'
              ? 'ascending'
              : 'descending'
            : undefined}
          class="relative flex min-w-0 items-center border-r border-line/60 pr-2
            {column.align === 'end' ? 'justify-end' : ''}"
        >
          <button
            type="button"
            class="flex min-w-0 items-center gap-1 px-1.5 py-1 hover:text-fg disabled:hover:text-muted"
            disabled={column.sortable === false}
            onclick={() => onsort(column)}
          >
            <span class="truncate">{column.title}</span>
            {#if sort?.columnId === column.id}
              <span aria-hidden="true">{sort.direction === 'asc' ? '▲' : '▼'}</span>
            {/if}
          </button>
          <button
            type="button"
            aria-label="Resize {column.title} column"
            title="Drag to resize, arrow keys to nudge"
            class="absolute inset-y-0 right-0 w-1.5 cursor-col-resize touch-none border-x border-transparent hover:border-accent"
            onpointerdown={(e) => startResize(column, e)}
            onkeydown={(e) => {
              const width = config.widths[column.id] ?? column.width;
              if (e.key === 'ArrowLeft') onwidth(column.id, width - 16, true);
              else if (e.key === 'ArrowRight') onwidth(column.id, width + 16, true);
              else return;
              e.preventDefault();
            }}
          ></button>
        </div>
      {/each}
    </div>

    {#if rows.length === 0}
      <div class="p-4 text-muted">
        {#if empty}
          {@render empty()}
        {:else}
          No rows match.
        {/if}
      </div>
    {:else}
      <div style:height="{rows.length * rowH}px" class="relative">
        <div style:transform="translateY({start * rowH}px)">
          {#each rows.slice(start, end) as row, i (rowKey(row))}
            {@const index = start + i}
            {@const key = rowKey(row)}
            {@const selected = selection?.has(key) ?? false}
            {@const stripe = color?.(row)}
            <div
              role="row"
              id="{idPrefix}-row-{index}"
              aria-rowindex={index + 2}
              aria-selected={selection ? selected : undefined}
              data-index={index}
              tabindex="-1"
              class="grid cursor-default items-center border-l-[3px] border-l-transparent
                hover:bg-hover"
              class:bg-accent-soft={selected}
              class:text-accent={selected}
              style:grid-template-columns={template}
              style:height="{rowH}px"
              style:border-left-color={stripe ? `var(--row-${stripe})` : undefined}
            >
              {#each columns as column (column.id)}
                <div
                  role="gridcell"
                  class="truncate px-1.5"
                  class:text-right={column.align === 'end'}
                >
                  {column.value(row) ?? ''}
                </div>
              {/each}
            </div>
          {/each}
        </div>
      </div>
    {/if}
  </div>
</div>
