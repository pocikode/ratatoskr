// Table columns, row colours and sort mapping for HTTP History (F-HIST-1).
import type {
  HistoryRowFragment,
  RequestColor,
  RequestSort,
  RequestSortField,
} from '$lib/api/generated/graphql';
import type { RowColor, SortState, TrafficColumn } from '$lib/components/TrafficTable/types';

export type HistoryRow = HistoryRowFragment;

/** Schema colour → row stripe token (docs/02 §10 names the `CYAN` shade "teal"). */
const ROW_COLORS: Record<RequestColor, RowColor> = {
  RED: 'red',
  ORANGE: 'orange',
  YELLOW: 'yellow',
  GREEN: 'green',
  CYAN: 'teal',
  BLUE: 'blue',
  PURPLE: 'purple',
  PINK: 'pink',
};

export const COLORS = Object.keys(ROW_COLORS) as RequestColor[];

/** Menu label for a colour; `null` clears the stripe. */
export function colorLabel(color: RequestColor | null): string {
  return color ? color.charAt(0) + color.slice(1).toLowerCase() : 'None';
}

export function rowColor(row: HistoryRow): RowColor | null {
  return row.color ? ROW_COLORS[row.color] : null;
}

/** Column id → `RequestSortField`; the server sorts, the page only names the column. */
const SORT_FIELDS: Record<string, RequestSortField> = {
  id: 'ID',
  host: 'HOST',
  method: 'METHOD',
  path: 'PATH',
  query: 'QUERY',
  status: 'STATUS',
  length: 'RESPONSE_LENGTH',
  ext: 'EXT',
  rtt: 'ROUNDTRIP',
  created: 'CREATED_AT',
  tls: 'TLS',
  port: 'PORT',
  source: 'SOURCE',
  color: 'COLOR',
  note: 'NOTE',
  edited: 'EDITED',
};

export function requestSort(sort: SortState | null): RequestSort | null {
  const field = sort && SORT_FIELDS[sort.columnId];
  return field ? { field, direction: sort?.direction === 'desc' ? 'DESC' : 'ASC' } : null;
}

/** Sort keys, mirroring the server's documented order (nulls/no response lowest). */
const SORT_KEYS: Record<RequestSortField, (row: HistoryRow) => string | number> = {
  ID: (row) => Number(row.id),
  HOST: (row) => row.host,
  METHOD: (row) => row.method,
  PATH: (row) => row.path,
  QUERY: (row) => row.query,
  STATUS: (row) => row.response?.code ?? -1,
  RESPONSE_LENGTH: (row) => Number(row.response?.length ?? -1),
  EXT: (row) => row.ext,
  ROUNDTRIP: (row) => row.response?.roundtripMs ?? -1,
  CREATED_AT: (row) => row.createdAt,
  TLS: (row) => Number(row.tls),
  PORT: (row) => row.port,
  SOURCE: (row) => row.source,
  COLOR: (row) => (row.color ? COLORS.indexOf(row.color) : -1),
  NOTE: (row) => row.note ?? '',
  EDITED: (row) => (row.editedFrom ? 1 : 0),
};

/** Places a live row where the current sort wants it; ties fall back to ID like the server. */
export function insertRow(
  rows: readonly HistoryRow[],
  row: HistoryRow,
  sort: SortState | null,
): HistoryRow[] {
  const field = sort && SORT_FIELDS[sort.columnId];
  if (!field) {
    // No sort state: the server's default order is ID ascending.
    const index = rows.findIndex((other) => Number(other.id) > Number(row.id));
    return index < 0 ? [...rows, row] : [...rows.slice(0, index), row, ...rows.slice(index)];
  }
  const key = SORT_KEYS[field];
  const direction = sort?.direction === 'desc' ? -1 : 1;
  const compare = (a: string | number, b: string | number) => (a < b ? -1 : a > b ? 1 : 0);
  const index = rows.findIndex(
    (other) =>
      compare(key(row), key(other)) * direction < 0 ||
      (compare(key(row), key(other)) === 0 && Number(row.id) < Number(other.id)),
  );
  return index < 0 ? [...rows, row] : [...rows.slice(0, index), row, ...rows.slice(index)];
}

const time = (createdAt: string) => new Date(createdAt).toLocaleTimeString();

export const historyColumns: readonly TrafficColumn<HistoryRow>[] = [
  { id: 'id', title: 'ID', width: 64, align: 'end', value: (row) => row.id },
  { id: 'host', title: 'Host', width: 160, value: (row) => row.host },
  { id: 'method', title: 'Method', width: 72, value: (row) => row.method },
  { id: 'path', title: 'Path', width: 200, value: (row) => row.path },
  { id: 'query', title: 'Query', width: 140, value: (row) => row.query },
  {
    id: 'status',
    title: 'Status',
    width: 64,
    align: 'end',
    value: (row) => row.response?.code ?? null,
  },
  {
    id: 'length',
    title: 'Length',
    width: 84,
    align: 'end',
    value: (row) => row.response?.length ?? null,
  },
  { id: 'ext', title: 'Ext', width: 56, value: (row) => row.ext },
  {
    id: 'rtt',
    title: 'RTT',
    width: 60,
    align: 'end',
    value: (row) => row.response?.roundtripMs ?? null,
  },
  { id: 'created', title: 'Created', width: 88, value: (row) => time(row.createdAt) },
  { id: 'tls', title: 'TLS', width: 48, value: (row) => (row.tls ? '✓' : '') },
  { id: 'port', title: 'Port', width: 56, align: 'end', value: (row) => row.port },
  { id: 'source', title: 'Source', width: 84, value: (row) => row.source.toLowerCase() },
  { id: 'color', title: 'Colour', width: 72, value: (row) => colorLabel(row.color) },
  { id: 'note', title: 'Note', width: 140, value: (row) => row.note ?? '' },
  { id: 'edited', title: 'Edited', width: 60, value: (row) => (row.editedFrom ? '✎' : '') },
];

/** `https://host[:port]/path?query`, with the port only when it is not the scheme default. */
export function requestUrl(row: HistoryRow): string {
  const scheme = row.tls ? 'https' : 'http';
  const port = (row.tls && row.port === 443) || (!row.tls && row.port === 80) ? '' : `:${row.port}`;
  const target = row.query ? `${row.path}?${row.query}` : row.path;
  return `${scheme}://${row.host}${port}${target}`;
}

/** POSIX single-quoting: captured bytes must never reach the shell unescaped. */
const shellQuote = (value: string) => `'${value.replaceAll("'", "'\\''")}'`;

/** Copy-ready cURL for a row. Headers and body need `head`/`body`, which rows do not load. */
export function requestCurl(row: HistoryRow): string {
  return `curl -X ${shellQuote(row.method)} ${shellQuote(requestUrl(row))}`;
}
