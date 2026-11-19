// HTTP History page state (F-HIST-1..3): server-paged rows, live updates with the
// "↓ N new" buffer (docs/02 §6), the query, the sort and the selected message detail.
import type { Client, CombinedError } from '@urql/core';
import { SvelteSet } from 'svelte/reactivity';
import { filter, makeSubject, pipe, take, takeUntil, toPromise, type Subject } from 'wonka';
import {
  FilterPresetsDocument,
  HistoryDeleteRequestsDocument,
  HistoryRequestAddedDocument,
  HistoryRequestDocument,
  HistoryRequestsDeletedDocument,
  HistoryRequestsDocument,
  HistoryRequestUpdatedDocument,
  HistorySetColorDocument,
  HistorySetNoteDocument,
  ScopePresetsDocument,
  UpdateScopePresetDocument,
  type FilterPresetFieldsFragment,
  type HistoryRequestsQuery,
  type HistoryRequestsQueryVariables,
  type HistoryRowFragment,
  type RequestColor,
  type ScopePresetFieldsFragment,
} from '$lib/api/generated/graphql';
import { Selection } from '$lib/components/TrafficTable/selection.svelte';
import type { ColumnConfig, SortState } from '$lib/components/TrafficTable/types';
import { byteToDoc, syntaxError, type QueryError } from '$lib/query/httpql';
import { compareRows, insertRow, requestSort } from './columns';
import { buildDetail, type HistoryDetail } from './detail';
import { loadColumnConfig, saveColumnConfig } from './ui-state';

/** Schema maximum for each page. */
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
  private notice = $state<{ text: string; paging?: true } | null>(null);
  paginationKey = $state(0);

  get message(): string | null {
    return this.notice?.text ?? null;
  }

  set message(text: string | null) {
    this.notice = text === null ? null : { text };
  }

  /** Text in the query bar; `applied` is what the server filtered on. */
  query = $state('');
  applied = $state('');
  history = $state.raw<string[]>([]);
  /** Query error: local syntax check or the server's `BAD_QUERY` span. */
  error = $state<QueryError | null>(null);

  sort = $state<SortState | null>({ columnId: 'id', direction: 'desc' });
  /** Active scope preset for the table and its subscriptions; null = no scope (F-SCOPE-1). */
  scopeId = $state<string | null>(null);
  scopes = $state.raw<ScopePresetFieldsFragment[]>([]);
  presets = $state.raw<FilterPresetFieldsFragment[]>([]);
  /** Viewers below the table (default) or to its right (docs/02 §3.1). */
  layout = $state<'below' | 'right'>('below');
  config = $state<ColumnConfig | null>(loadColumnConfig());

  detail = $state.raw<HistoryDetail | null>(null);
  detailLoading = $state(false);

  private stops: Array<() => void> = [];
  private projectToken = 0;
  private reloadToken = 0;
  private page: {
    variables: HistoryRequestsQueryVariables;
    info: HistoryRequestsQuery['requests']['pageInfo'];
  } | null = null;
  private request: { cancel: Subject<void>; dropped?: SvelteSet<string> } | null = null;

  constructor(client: Client) {
    this.client = client;
  }

  /** Swaps the backend before the first load (dev mock, T-013). */
  useClient(client: Client): void {
    this.client = client;
  }

  /** Loads the first page and follows live changes; returns the cleanup. */
  start(): () => void {
    void this.loadPresets();
    void this.reload();
    return () => this.stop();
  }

  /** Scope presets and filter presets for the query bar; the table works without them. */
  private async loadPresets(): Promise<void> {
    const token = this.projectToken;
    const [scopes, presets] = await Promise.all([
      this.client.query(ScopePresetsDocument, {}, { requestPolicy: 'network-only' }).toPromise(),
      this.client.query(FilterPresetsDocument, {}, { requestPolicy: 'network-only' }).toPromise(),
    ]);
    if (token !== this.projectToken) return;
    this.scopes = scopes.data?.scopePresets ?? [];
    this.presets = presets.data?.filterPresets ?? [];
    // A preset deleted elsewhere falls back to no scope (scope.graphqls).
    if (this.scopeId && !this.scopes.some((scope) => scope.id === this.scopeId))
      this.scopeId = null;
  }

  /** Applies a scope preset to the table and its live subscriptions; null clears it. */
  setScope(id: string | null): void {
    if (id === this.scopeId) return;
    this.scopeId = id;
    this.selection.clear();
    this.detail = null;
    void this.reload();
  }

  /** Quick-apply a filter preset: the bar holds `preset:"Name"` and the query runs. */
  applyPreset(name: string): void {
    this.apply(`preset:"${literal(name)}"`);
  }

  /** Name of the preset the applied query references, for the chips' pressed state. */
  get activePreset(): string | null {
    for (const preset of this.presets) {
      if (this.applied.includes(`preset:"${literal(preset.name)}"`)) return preset.name;
      if (preset.alias && this.applied.includes(`preset:${preset.alias}`)) return preset.name;
    }
    return null;
  }

  /** Adds a host to the active scope's allow or deny list, then refilters the table. */
  async addHostToScope(host: string, kind: 'allow' | 'deny'): Promise<void> {
    const scope = this.scopes.find((other) => other.id === this.scopeId);
    if (!scope) return;
    if ((kind === 'allow' ? scope.allow : scope.deny).includes(host)) return;
    const result = await this.client
      .mutation(UpdateScopePresetDocument, {
        id: scope.id,
        name: scope.name,
        allow: kind === 'allow' ? [...scope.allow, host] : scope.allow,
        deny: kind === 'deny' ? [...scope.deny, host] : scope.deny,
      })
      .toPromise();
    const saved = result.data?.updateScopePreset;
    if (result.error || !saved) {
      this.message = result.error?.message ?? 'Updating the scope failed';
      return;
    }
    this.message = null;
    this.scopes = this.scopes.map((other) => (other.id === saved.id ? saved : other));
    void this.reload();
  }

  stop() {
    this.projectToken++;
    this.reloadToken++;
    this.request?.cancel.next();
    this.request = null;
    this.unsubscribe();
  }

  reset(): void {
    this.stop();
    this.page = null;
    this.rows = this.pending = [];
    this.query = this.applied = this.sent = '';
    this.history = this.scopes = this.presets = [];
    this.scopeId = this.error = this.message = this.detail = null;
    this.total = 0;
    this.selection.clear();
    this.start();
  }

  private unsubscribe(): void {
    for (const stop of this.stops) stop();
    this.stops = [];
  }

  private fetchPage(variables: HistoryRequestsQueryVariables, cancel: Subject<void>) {
    return pipe(
      this.client.query(HistoryRequestsDocument, variables, { requestPolicy: 'network-only' }),
      filter((result) => !result.stale && !result.hasNext),
      take(1),
      takeUntil(cancel.source),
      toPromise,
    );
  }

  /** Query last sent to the server; accepted once a load succeeds (then it equals `applied`). */
  private sent = '';

  async reload(query = this.sent): Promise<void> {
    const token = ++this.reloadToken;
    this.request?.cancel.next();
    const cancel = makeSubject<void>();
    this.request = { cancel };
    this.loading = true;
    const variables = {
      query: query || null,
      scopeId: this.scopeId,
      first: PAGE,
      sort: requestSort(this.sort),
    };
    const result = await this.fetchPage(variables, cancel);
    if (token !== this.reloadToken) return; // a newer query replaced this one
    this.request = null;
    this.loading = false;
    if (!result) return;

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
      this.sent = this.applied;
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
    this.page = { variables, info: connection.pageInfo };
    this.paginationKey++;
    this.total = connection.totalCount;
    this.pending = [];
    this.error = null;
    this.message = null;
    this.subscribe();
  }

  async loadNextPage(): Promise<void> {
    const page = this.page;
    if (this.request || !page?.info.hasNextPage) return;

    const token = this.reloadToken;
    const request = { cancel: makeSubject<void>(), dropped: new SvelteSet<string>() };
    this.request = request;
    try {
      while (page.info.hasNextPage && page.info.endCursor != null) {
        const after = page.info.endCursor;
        const result = await this.fetchPage({ ...page.variables, after }, request.cancel);
        if (token !== this.reloadToken) return;
        const connection = result?.data?.requests;
        if (result?.error || !connection) {
          this.notice = {
            text: result?.error?.message ?? 'Loading more requests failed',
            paging: true,
          };
          return;
        }

        page.info = connection.pageInfo;
        if (this.notice?.paging) this.message = null;
        this.total = connection.totalCount;
        const seen = new SvelteSet(this.rows.map((row) => row.id));
        for (const row of this.pending) seen.add(row.id);
        const appended: HistoryRowFragment[] = [];
        for (const { node } of connection.edges) {
          if (seen.has(node.id) || request.dropped.has(node.id)) continue;
          seen.add(node.id);
          appended.push(node);
        }
        if (appended.length) {
          // Live rows may already sit beyond the last fetched page in the current sort order.
          const compare = compareRows(page.variables.sort);
          let live = this.rows.findLastIndex((row) => compare(row, appended[0]!) < 0) + 1;
          const merged = this.rows.slice(0, live);
          for (const row of appended) {
            while (live < this.rows.length && compare(this.rows[live]!, row) < 0) {
              merged.push(this.rows[live++]!);
            }
            merged.push(row);
          }
          this.rows = merged.concat(this.rows.slice(live));
          return;
        }
        // A page of only live duplicates leaves the table's row count unchanged.
        if (page.info.endCursor === after) return;
      }
    } finally {
      if (this.request === request) this.request = null;
    }
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
    this.sent = next;
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
    const token = this.projectToken;
    const result = await this.client
      .query(HistoryRequestDocument, { id }, { requestPolicy: 'network-only' })
      .toPromise();
    if (token !== this.projectToken || this.selection.anchor !== id) return;
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
    const dropped = this.request?.dropped;
    if (dropped) for (const id of gone) dropped.add(id);
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
    this.unsubscribe();
    const query = this.applied || null;
    const scopeId = this.scopeId;
    const added = this.client
      .subscription(HistoryRequestAddedDocument, { query, scopeId })
      .subscribe((result) => {
        if (result.data) this.addRow(result.data.requestAdded);
      });
    const updated = this.client
      .subscription(HistoryRequestUpdatedDocument, { query, scopeId })
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

/** Escapes a value for an HTTPQL string literal (docs/04 §1). */
export function literal(value: string): string {
  return value.replace(/[\\"]/g, '\\$&');
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
