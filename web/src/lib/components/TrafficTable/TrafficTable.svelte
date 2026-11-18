<script lang="ts" generics="Row">
  import { untrack } from 'svelte';
  import type { Snippet } from 'svelte';
  import ColumnMenu from './ColumnMenu.svelte';
  import { normalizeConfig, setColumnWidth } from './columns';
  import type { Selection } from './selection.svelte';
  import TableView from './TableView.svelte';
  import type { ColumnConfig, RowColor, SortState, TrafficColumn } from './types';

  interface Props {
    /** Already filtered and sorted by the page: the table never reorders rows itself. */
    rows: Row[];
    columns: readonly TrafficColumn<Row>[];
    rowKey: (row: Row) => string;
    label?: string;
    selection?: Selection;
    /** Initial layout; changes are reported through `onconfigchange`. */
    config?: ColumnConfig | null;
    onconfigchange?: (config: ColumnConfig) => void;
    sort?: SortState | null;
    onsortchange?: (sort: SortState | null) => void;
    onrowopen?: (row: Row) => void;
    onrowcontextmenu?: (row: Row, event: MouseEvent | KeyboardEvent) => void;
    color?: (row: Row) => RowColor | null;
    empty?: Snippet;
  }

  let {
    rows,
    columns,
    rowKey,
    label = 'Traffic table',
    selection,
    config,
    onconfigchange,
    sort = $bindable(null),
    onsortchange,
    onrowopen,
    onrowcontextmenu,
    color,
    empty,
  }: Props = $props();

  const uid = $props.id();
  let cfg = $state(
    normalizeConfig(
      untrack(() => columns),
      untrack(() => config),
    ),
  );
  let active = $state(-1);
  let pageRows = $state(10);

  const visible = $derived(
    cfg.order
      .map((id) => columns.find((c) => c.id === id))
      .filter((c): c is TrafficColumn<Row> => c !== undefined && !cfg.hidden.includes(c.id)),
  );
  const keys = $derived(rows.map(rowKey));

  function setWidth(id: string, width: number, commit: boolean) {
    cfg = setColumnWidth(cfg, id, width);
    if (commit) onconfigchange?.(cfg);
  }

  function toggleSort(column: TrafficColumn<Row>) {
    if (column.sortable === false) return;
    const next: SortState | null =
      sort?.columnId !== column.id
        ? { columnId: column.id, direction: 'asc' }
        : sort.direction === 'asc'
          ? { columnId: column.id, direction: 'desc' }
          : null;
    sort = next;
    onsortchange?.(next);
  }

  function setActive(index: number, extend = false) {
    const row = rows[index];
    if (!row) return;
    active = index;
    selection?.click(rowKey(row), keys, { range: extend });
  }

  function onkeydown(e: KeyboardEvent) {
    if (e.target !== e.currentTarget || !rows.length) return; // header/resize buttons own their keys
    const from = Math.min(Math.max(active, 0), rows.length - 1); // rows can shrink under a filter
    const actions: Record<string, () => void> = {
      ArrowDown: () => setActive(active < 0 ? 0 : Math.min(rows.length - 1, from + 1), e.shiftKey),
      ArrowUp: () => setActive(active < 0 ? 0 : Math.max(0, from - 1), e.shiftKey),
      Home: () => setActive(0, e.shiftKey),
      End: () => setActive(rows.length - 1, e.shiftKey),
      PageDown: () => setActive(Math.min(rows.length - 1, from + pageRows), e.shiftKey),
      PageUp: () => setActive(Math.max(0, from - pageRows), e.shiftKey),
      ' ': () => {
        const row = rows[from];
        if (row && selection) selection.click(rowKey(row), keys, { toggle: true });
        active = from;
      },
      Enter: () => {
        const row = rows[from];
        if (row) onrowopen?.(row);
      },
      Escape: () => selection?.clear(),
      a: () => {
        if (e.ctrlKey || e.metaKey) {
          e.preventDefault(); // do not also select the page text
          selection?.selectAll(keys);
        }
      },
      ContextMenu: () => {
        const row = rows[from];
        if (row) onrowcontextmenu?.(row, e);
      },
    };
    const action =
      actions[e.key] ?? (e.key === 'F10' && e.shiftKey ? actions['ContextMenu'] : undefined);
    if (!action) return;
    if (e.key === ' ') e.preventDefault(); // keep the grid from scrolling
    action();
  }

  function rowIndexAt(e: Event): number {
    const cell = (e.target as HTMLElement | null)?.closest<HTMLElement>('[data-index]');
    return cell ? Number(cell.dataset.index) : -1;
  }

  function ongridclick(e: MouseEvent) {
    const index = rowIndexAt(e);
    const row = rows[index];
    if (!row) return;
    grid?.focus();
    active = index;
    selection?.click(rowKey(row), keys, { toggle: e.ctrlKey || e.metaKey, range: e.shiftKey });
  }

  function ongriddblclick(e: MouseEvent) {
    const row = rows[rowIndexAt(e)];
    if (row) onrowopen?.(row);
  }

  function ongridcontextmenu(e: MouseEvent) {
    const index = rowIndexAt(e);
    const row = rows[index];
    if (!row) return;
    e.preventDefault();
    grid?.focus();
    active = index;
    if (selection && !selection.has(rowKey(row))) selection.click(rowKey(row), keys);
    onrowcontextmenu?.(row, e);
  }

  let grid = $state<HTMLDivElement | null>(null);
</script>

<div
  bind:this={grid}
  role="grid"
  aria-label={label}
  aria-rowcount={rows.length + 1}
  aria-colcount={visible.length}
  aria-multiselectable={selection ? true : undefined}
  aria-activedescendant={active >= 0 && active < rows.length ? `${uid}-row-${active}` : undefined}
  tabindex="0"
  class="flex h-full min-h-0 flex-col"
  onclick={ongridclick}
  ondblclick={ongriddblclick}
  oncontextmenu={ongridcontextmenu}
  {onkeydown}
>
  <div class="flex h-7 shrink-0 items-center gap-2 border-b border-line px-2 text-xs text-muted">
    <span aria-live="polite">
      {rows.length}
      {rows.length === 1 ? 'row' : 'rows'}{selection && selection.size > 0
        ? ` · ${selection.size} selected`
        : ''}
    </span>
    <span class="flex-1"></span>
    {#if onconfigchange}
      <ColumnMenu
        {columns}
        config={cfg}
        onconfig={(next) => {
          cfg = next;
          onconfigchange(next);
        }}
      />
    {/if}
  </div>

  <TableView
    {rows}
    columns={visible}
    {rowKey}
    idPrefix={uid}
    config={cfg}
    {sort}
    active={active < rows.length ? active : -1}
    bind:pageRows
    {selection}
    {color}
    onsort={toggleSort}
    onwidth={setWidth}
    {empty}
  />
</div>
