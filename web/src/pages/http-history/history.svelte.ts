// HTTP History page state (F-HIST-1..3): server-paged rows, live updates with the
// "↓ N new" buffer (docs/02 §6), the query, the sort and the selected message detail.
import type { Client, CombinedError } from '@urql/core';
import { SvelteSet } from 'svelte/reactivity';
import {
  HistoryDeleteRequestsDocument,
  HistoryRequestAddedDocument,
  HistoryRequestDocument,
  HistoryRequestsDeletedDocument,
  HistoryRequestsDocument,
  HistoryRequestUpdatedDocument,
  HistorySetColorDocument,
  HistorySetNoteDocument,
  type HistoryRowFragment,
  type RequestColor,
} from '$lib/api/generated/graphql';
import { Selection } from '$lib/components/TrafficTable/selection.svelte';
import type { ColumnConfig, SortState } from '$lib/components/TrafficTable/types';
import { byteToDoc, syntaxError, type QueryError } from '$lib/query/httpql';
import { insertRow, requestSort } from './columns';
import { buildDetail, type HistoryDetail } from './detail';
import { loadColumnConfig, saveColumnConfig } from './ui-state';

/** Schema maximum for `first`; paging beyond it needs a TrafficTable scroll hook (Handoff). */
const PAGE = 1000;
const MAX_HISTORY = 20;

export class HistoryState {
  client: Client;
  readonly selection = new Selection();

  rows = $state.raw<HistoryRowFragment[]>([]);
  /** Rows held behind the "↓ N new" pill while the user is working in the table. */
  pending = $state.raw<HistoryRowFragment[]>([]);
  total = $state(0);
  loading = $state(false);
  message = $state<string | null>(null);

  /** Text in the query bar; `applied` is what the server filtered on. */
  query = $state('');
  applied = $state('');
  history = $state.raw<string[]>([]);
  /** Query error: local syntax check or the server's `BAD_QUERY` span. */
  error = $state<QueryError | null>(null);

  sort = $state<SortState | null>({ columnId: 'id', direction: 'desc' });
  /** Viewers below the table (default) or to its right (docs/02 §3.1). */
  layout = $state<'below' | 'right'>('below');
  config = $state<ColumnConfig | null>(loadColumnConfig());

  detail = $state.raw<HistoryDetail | null>(null);
  detailLoading = $state(false);

  private stops: Array<() => void> = [];
  private reloadToken = 0;

  constructor(client: Client) {
    this.client = client;
  }

  /** Swaps the backend before the first load (dev mock, T-013). */
  useClient(client: Client): void {
    this.client = client;
  }

  /** Loads the first page and follows live changes; returns the cleanup. */
  start(): () => void {
    void this.reload();
    return () => this.stop();
  }

  stop() {
    for (const stop of this.stops) stop();
    this.stops = [];
  }

  async reload(query = this.applied): Promise<void> {
    const token = ++this.reloadToken;
    this.loading = true;
    const result = await this.client
      .query(
        HistoryRequestsDocument,
        { query: query || null, first: PAGE, sort: requestSort(this.sort) },
        { requestPolicy: 'network-only' },
      )
      .toPromise();
    if (token !== this.reloadToken) return; // a newer query replaced this one
    this.loading = false;

    // The bar text may carry leading whitespace that was trimmed before sending.
    const lead =
      this.query.trim() === query ? this.query.length - this.query.trimStart().length : 0;
    const badQuery = badQuerySpan(result.error);
    if (badQuery) {
      this.error = {
        message: result.error?.graphQLErrors[0]?.message ?? 'Invalid query',
        // The span is into the query that was sent, which may differ from the bar text.
        from: lead + byteToDoc(query, badQuery.start),
        to: lead + byteToDoc(query, badQuery.end),
      };
      return; // the table keeps its rows (T-017 AC)
    }
    if (result.error) {
      this.message = result.error.message;
      return;
    }
    const connection = result.data?.requests;
    if (!connection) return;

    if (query !== this.applied) {
      this.applied = query;
      if (query) this.history = [...this.history.slice(-(MAX_HISTORY - 1)), query];
    }
    this.rows = connection.edges.map((edge) => edge.node);
    this.total = connection.totalCount;
    this.pending = [];
    this.error = null;
    this.message = null;
    this.subscribe();
  }

  /** Enter in the query bar: validates, stores history and reloads. */
  apply(text = this.query): void {
    this.query = text;
    const problem = syntaxError(text);
    this.error = problem;
    if (problem) return; // keep the current rows and filter
    const next = text.trim();
    if (next === this.applied) return;
    this.selection.clear();
    this.detail = null;
    this.pending = [];
    void this.reload(next);
  }

  setSort(sort: SortState | null): void {
    this.sort = sort;
    void this.reload();
  }

  toggleLayout(): void {
    this.layout = this.layout === 'below' ? 'right' : 'below';
  }

  setConfig(config: ColumnConfig): void {
    this.config = config;
    saveColumnConfig(config);
  }

  applyPending(): void {
    let rows = this.rows;
    for (const row of [...this.pending].reverse()) rows = insertRow(rows, row, this.sort);
    this.rows = rows;
    this.pending = [];
  }

  /** Fetches the selected row's head and body; a newer selection drops this result. */
  async showDetail(id: string | null): Promise<void> {
    if (!id) {
      this.detail = null;
      this.detailLoading = false;
      return;
    }
    this.detailLoading = true;
    const result = await this.client
      .query(HistoryRequestDocument, { id }, { requestPolicy: 'network-only' })
      .toPromise();
    if (this.selection.anchor !== id) return;
    this.detailLoading = false;
    const message = result.data?.request;
    if (result.error || !message) {
      this.detail = null;
      this.message = result.error?.message ?? `Request ${id} not found`;
      return;
    }
    this.detail = buildDetail(message);
  }

  async setColor(ids: readonly string[], color: RequestColor | null): Promise<void> {
    const result = await this.client
      .mutation(HistorySetColorDocument, { ids: [...ids], color })
      .toPromise();
    if (result.error) {
      this.message = result.error.message;
      return;
    }
    const changed = result.data?.setRequestColor ?? [];
    this.rows = this.rows.map((row) => {
      const updated = changed.find((other) => other.id === row.id);
      return updated ? { ...row, color: updated.color } : row;
    });
  }

  async setNote(id: string, note: string | null): Promise<void> {
    const result = await this.client
      .mutation(HistorySetNoteDocument, { id, note: note?.trim() || null })
      .toPromise();
    if (result.error) {
      this.message = result.error.message;
      return;
    }
    const changed = result.data?.setRequestNote;
    if (changed)
      this.rows = this.rows.map((row) =>
        row.id === changed.id ? { ...row, note: changed.note } : row,
      );
  }

  async remove(ids: readonly string[]): Promise<void> {
    if (ids.length === 0) return;
    const result = await this.client
      .mutation(HistoryDeleteRequestsDocument, { ids: [...ids] })
      .toPromise();
    if (result.error) {
      this.message = result.error.message;
      return;
    }
    this.dropRows(ids);
  }

  /** Rows behind the pill: selected rows stay put, an idle table keeps growing live. */
  private addRow(row: HistoryRowFragment): void {
    if (this.rows.some((other) => other.id === row.id)) return;
    if (this.pending.some((other) => other.id === row.id)) return;
    this.total += 1;
    if (this.selection.size > 0) {
      this.pending = [row, ...this.pending];
      return;
    }
    this.rows = insertRow(this.rows, row, this.sort);
  }

  private replaceRow(row: HistoryRowFragment): void {
    if (this.rows.some((other) => other.id === row.id)) {
      this.rows = this.rows.map((other) => (other.id === row.id ? row : other));
    } else if (this.pending.some((other) => other.id === row.id)) {
      this.pending = this.pending.map((other) => (other.id === row.id ? row : other));
    } else {
      this.addRow(row);
    }
  }

  private dropRows(ids: readonly string[]): void {
    const gone = new SvelteSet(ids);
    const removed =
      this.rows.filter((row) => gone.has(row.id)).length +
      this.pending.filter((row) => gone.has(row.id)).length;
    this.rows = this.rows.filter((row) => !gone.has(row.id));
    this.pending = this.pending.filter((row) => !gone.has(row.id));
    this.total = Math.max(0, this.total - removed);
    if ([...this.selection.keys].some((id) => gone.has(id))) this.selection.clear();
    if (this.detail && gone.has(this.detail.id)) this.detail = null;
  }

  /** (Re)starts the three live subscriptions for the applied query. */
  private subscribe(): void {
    this.stop();
    const query = this.applied || null;
    const added = this.client
      .subscription(HistoryRequestAddedDocument, { query })
      .subscribe((result) => {
        if (result.data) this.addRow(result.data.requestAdded);
      });
    const updated = this.client
      .subscription(HistoryRequestUpdatedDocument, { query })
      .subscribe((result) => {
        const change = result.data?.requestUpdated;
        if (!change) return;
        if (change.matches) this.replaceRow(change.request);
        else this.dropRows([change.request.id]);
      });
    const deleted = this.client
      .subscription(HistoryRequestsDeletedDocument, {})
      .subscribe((result) => {
        if (result.data) this.dropRows(result.data.requestsDeleted.ids);
      });
    this.stops = [
      () => added.unsubscribe(),
      () => updated.unsubscribe(),
      () => deleted.unsubscribe(),
    ];
  }
}

/** `extensions.span = {start, end}` of a `BAD_QUERY` error, in UTF-8 bytes (docs/03 §6). */
export function badQuerySpan(error?: CombinedError): { start: number; end: number } | null {
  for (const graphQLError of error?.graphQLErrors ?? []) {
    const extensions = graphQLError.extensions as
      { code?: unknown; span?: { start?: unknown; end?: unknown } } | undefined;
    const span = extensions?.span;
    if (extensions?.code !== 'BAD_QUERY' || !span) continue;
    const start = Number(span.start);
    const end = Number(span.end);
    if (Number.isFinite(start) && Number.isFinite(end)) return { start, end };
  }
  return null;
}
