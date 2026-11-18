/** Row stripe colours (docs/02 §10); `null` keeps the default transparent stripe. */
export type RowColor = 'red' | 'orange' | 'yellow' | 'green' | 'teal' | 'blue' | 'purple' | 'pink';

export type SortDirection = 'asc' | 'desc';

export interface SortState {
  columnId: string;
  direction: SortDirection;
}

/**
 * One column of a `TrafficTable`. The table stays data-source agnostic: rows can be any shape as
 * long as `value` extracts the text for the cell, sorting and copying.
 */
export interface TrafficColumn<Row> {
  id: string;
  title: string;
  /** Default width in px; users can resize it. */
  width: number;
  align?: 'start' | 'end';
  value: (row: Row) => string | number | null;
  /** Set `false` for columns that make no sense to sort by (colour, note). */
  sortable?: boolean;
}

/** User column layout. Persist via `onconfigchange` into backend `ui_state` (docs/06 §4). */
export interface ColumnConfig {
  /** Column ids, left to right. */
  order: string[];
  hidden: string[];
  widths: Record<string, number>;
}
