import { fireEvent, render, screen, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import type { Component } from 'svelte';
import type { ColumnConfig, SortState, TrafficColumn } from './types';
import { Selection } from './selection.svelte';
import TrafficTable from './TrafficTable.svelte';

interface Row {
  id: number;
  method: string;
  host: string;
  path: string;
}

const columns: TrafficColumn<Row>[] = [
  { id: 'id', title: 'ID', width: 60, align: 'end', value: (r) => r.id },
  { id: 'method', title: 'Method', width: 70, value: (r) => r.method },
  { id: 'host', title: 'Host', width: 160, value: (r) => r.host },
  { id: 'path', title: 'Path', width: 220, value: (r) => r.path },
];

interface TableProps {
  rows: Row[];
  columns: TrafficColumn<Row>[];
  rowKey: (row: Row) => string;
  selection?: Selection;
  config?: ColumnConfig;
  onconfigchange?: (config: ColumnConfig) => void;
  sort?: SortState | null;
  onsortchange?: (sort: SortState | null) => void;
  onrowopen?: (row: Row) => void;
  onrowcontextmenu?: (row: Row, event: MouseEvent) => void;
}

// `bind:this`-free, prop-only view of the generic component for the test renderer.
const Table = TrafficTable as unknown as Component<TableProps>;

function makeRows(count: number): Row[] {
  return Array.from({ length: count }, (_, i) => ({
    id: count - i,
    method: i % 2 ? 'POST' : 'GET',
    host: `host-${i % 5}.test`,
    path: `/item/${i}`,
  }));
}

function setup(props: Record<string, unknown> = {}) {
  const rows = (props.rows as Row[] | undefined) ?? makeRows(1000);
  const selection = (props.selection as Selection | undefined) ?? new Selection();
  const result = render(Table, {
    props: { rows, columns, rowKey: (r: Row) => String(r.id), selection, ...props },
  });
  return { ...result, rows, selection };
}

describe('TrafficTable', () => {
  it('renders only a window of a large data set', () => {
    setup({ rows: makeRows(100_000) });
    const rendered = screen.getAllByRole('row');
    expect(rendered.length).toBeGreaterThan(2);
    expect(rendered.length).toBeLessThan(80);
    expect(screen.getByText(/100000 rows/)).toBeVisible();
  });

  it('shows an empty state', () => {
    setup({ rows: [] });
    expect(screen.getByText('No rows match.')).toBeVisible();
  });

  it('reports sorting through the callback and marks the header', async () => {
    const user = userEvent.setup();
    const onsortchange = vi.fn<(sort: SortState | null) => void>();
    setup({ onsortchange });

    await user.click(screen.getByRole('button', { name: 'Host' }));
    expect(onsortchange).toHaveBeenLastCalledWith({ columnId: 'host', direction: 'asc' });
    expect(screen.getByRole('columnheader', { name: 'Host' })).toHaveAttribute(
      'aria-sort',
      'ascending',
    );

    await user.click(screen.getByRole('button', { name: 'Host' }));
    expect(onsortchange).toHaveBeenLastCalledWith({ columnId: 'host', direction: 'desc' });

    await user.click(screen.getByRole('button', { name: 'Host' }));
    expect(onsortchange).toHaveBeenLastCalledWith(null);
  });

  it('moves the active row and selects with the keyboard', async () => {
    const user = userEvent.setup();
    const { selection } = setup();
    const grid = screen.getByRole('grid', { name: 'Traffic table' });

    await user.click(within(grid).getAllByRole('row')[1]!);
    expect(selection.size).toBe(1);
    expect(grid).toHaveAttribute('aria-activedescendant');

    await user.keyboard('{ArrowDown}');
    expect(selection.size).toBe(1);
    await user.keyboard('{Shift>}{ArrowDown}{/Shift}');
    expect(selection.size).toBe(2);
    await user.keyboard(' ');
    expect(selection.size).toBe(1);
  });

  it('starts on the first row when there is no active row', async () => {
    const user = userEvent.setup();
    const { selection } = setup();
    const grid = screen.getByRole('grid', { name: 'Traffic table' });
    grid.focus();

    await user.keyboard('{ArrowDown}');
    expect(selection.has('1000')).toBe(true); // first displayed row
    await user.keyboard('{ArrowUp}');
    expect(selection.has('1000')).toBe(true);
    expect(grid).toHaveAttribute('aria-activedescendant');
  });

  it('does not act on keys from the header controls', async () => {
    const user = userEvent.setup();
    const onrowopen = vi.fn<(row: Row) => void>();
    const { selection } = setup({ onrowopen });

    const sortButton = screen.getByRole('button', { name: 'Host' });
    sortButton.focus();
    await user.keyboard('{Enter}');
    expect(onrowopen).not.toHaveBeenCalled();
    expect(selection.size).toBe(0);
  });

  it('selects all with Ctrl+A', async () => {
    const { selection } = setup();
    const grid = screen.getByRole('grid', { name: 'Traffic table' });

    await fireEvent.keyDown(grid, { key: 'a', ctrlKey: true });
    expect(selection.size).toBe(1000);
  });

  it('calls onrowopen on double click and onrowcontextmenu on right click', async () => {
    const user = userEvent.setup();
    const onrowopen = vi.fn<(row: Row) => void>();
    const onrowcontextmenu = vi.fn<(row: Row, event: MouseEvent) => void>();
    setup({ onrowopen, onrowcontextmenu });

    const row = screen.getByText('/item/0').closest('[role="row"]')!;
    await user.dblClick(row);
    expect(onrowopen).toHaveBeenCalledWith(expect.objectContaining({ path: '/item/0' }));

    await fireEvent.contextMenu(row);
    expect(onrowcontextmenu).toHaveBeenCalledWith(
      expect.objectContaining({ path: '/item/0' }),
      expect.any(MouseEvent),
    );
  });

  it('hides a column through the column menu and reports the new config', async () => {
    const user = userEvent.setup();
    const onconfigchange = vi.fn();
    setup({ onconfigchange });

    expect(screen.getByRole('columnheader', { name: 'Path' })).toBeVisible();
    await user.click(screen.getByRole('button', { name: 'Columns' }));
    // The popover is visibility:hidden in jsdom, so match the aria-label directly.
    await user.click(await screen.findByLabelText('Path'));

    expect(onconfigchange).toHaveBeenCalledTimes(1);
    expect(onconfigchange.mock.calls[0]![0].hidden).toEqual(['path']);
    expect(screen.queryByRole('columnheader', { name: /Path/ })).toBeNull();
  });

  it('reorders a column through the column menu', async () => {
    const user = userEvent.setup();
    const onconfigchange = vi.fn();
    setup({ onconfigchange });

    await user.click(screen.getByRole('button', { name: 'Columns' }));
    await user.click(await screen.findByLabelText('Move Method left'));

    expect(onconfigchange.mock.calls[0]![0].order).toEqual(['method', 'id', 'host', 'path']);
  });
});
