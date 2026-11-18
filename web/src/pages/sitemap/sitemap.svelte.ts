import type { Client } from '@urql/core';
import { SvelteSet } from 'svelte/reactivity';
import { onEnd, pipe, subscribe, tap, type Source } from 'wonka';
import {
  HistoryCompletionsDocument,
  HistoryRequestAddedDocument,
  HistoryRequestDocument,
  HistoryRequestsDeletedDocument,
  HistoryRequestUpdatedDocument,
  ScopePresetsDocument,
  SitemapDeleteDocument,
  SitemapNodesDocument,
  SitemapRequestsDocument,
  UpdateScopePresetDocument,
  type HistoryRowFragment,
  type ScopePresetFieldsFragment,
  type SitemapNodesQuery,
} from '$lib/api/generated/graphql';
import { Selection } from '$lib/components/TrafficTable/selection.svelte';
import type { SortState } from '$lib/components/TrafficTable/types';
import { syntaxError, type QueryError } from '$lib/query/httpql';
import { requestSort } from '../http-history/columns';
import { buildDetail, type HistoryDetail } from '../http-history/detail';

export type SitemapNode = SitemapNodesQuery['sitemapNodes'][number];
export type TreeRow = { node: SitemapNode; depth: number; position: number; siblings: number };

function childGroups(parentId: string, nodes: SitemapNode[], recursive: boolean) {
  const children: Record<string, SitemapNode[]> = { [parentId]: [] };
  for (const child of nodes) {
    if (recursive && child.hasChildren) children[child.id] = [];
    (children[child.parentId ?? ''] ??= []).push(child);
  }
  return children;
}

export class SitemapState {
  query = $state('');
  applied = $state('');
  scopeId = $state<string | null>(null);
  scopes = $state.raw<ScopePresetFieldsFragment[]>([]);
  error = $state<QueryError | null>(null);
  message = $state<string | null>(null);
  loading = $state(false);
  children = $state.raw<Record<string, SitemapNode[]>>({});
  readonly expanded = new SvelteSet<string>();
  readonly busy = new SvelteSet<string>();
  selected = $state.raw<SitemapNode | null>(null);
  target = $state.raw<SitemapNode | null>(null);
  confirmation = $state.raw<{ node: SitemapNode; scopeId: string | null } | null>(null);
  deleting = $state(false);
  rows = $state.raw<HistoryRowFragment[]>([]);
  total = $state(0);
  tableLoading = $state(false);
  cursor = $state<string | null>(null);
  sort = $state<SortState | null>({ columnId: 'id', direction: 'desc' });
  readonly selection = new Selection();
  detail = $state.raw<HistoryDetail | null>(null);
  detailLoading = $state(false);

  visible = $derived.by(() => {
    const rows: TreeRow[] = [];
    const walk = (parent: string, depth: number) => {
      const children = this.children[parent] ?? [];
      children.forEach((node, index) => {
        rows.push({ node, depth, position: index + 1, siblings: children.length });
        if (this.expanded.has(node.id)) walk(node.id, depth + 1);
      });
    };
    walk('', 1);
    return rows;
  });

  private epoch = 0;
  private tableToken = 0;
  private detailToken = 0;
  private scopeToken = 0;
  private refreshToken = 0;
  private rowsDirty = false;
  private readonly recursiveRoots = new SvelteSet<string>();
  private stops: Array<() => void> = [];
  private timer: ReturnType<typeof setTimeout> | undefined;
  private lastRefresh = 0;
  private running = false;
  private listening = false;

  constructor(readonly client: Client) {}

  start(): () => void {
    this.running = true;
    void this.loadScopes();
    void this.apply();
    return () => this.stop();
  }

  reset(): void {
    this.stop();
    this.query = this.applied = '';
    this.scopeId = null;
    this.scopes = [];
    this.children = {};
    this.expanded.clear();
    this.recursiveRoots.clear();
    this.busy.clear();
    this.clearTable();
    this.target = null;
    this.confirmation = null;
    this.message = null;
    this.start();
  }

  stop(): void {
    this.running = false;
    this.epoch++;
    this.tableToken++;
    this.detailToken++;
    this.scopeToken++;
    this.unlisten();
  }

  private unlisten(): void {
    this.listening = false;
    this.stops.forEach((stop) => stop());
    this.stops = [];
    clearTimeout(this.timer);
    this.timer = undefined;
  }

  private async loadScopes(): Promise<void> {
    const token = ++this.scopeToken;
    const result = await this.client
      .query(ScopePresetsDocument, {}, { requestPolicy: 'network-only' })
      .toPromise();
    if (this.running && token === this.scopeToken) this.scopes = result.data?.scopePresets ?? [];
  }

  async apply(text = this.query, scopeId = this.scopeId): Promise<void> {
    this.query = text;
    this.error = syntaxError(text);
    if (this.error) return;
    const epoch = ++this.epoch;
    this.unlisten();
    this.busy.clear();
    this.loading = true;
    const result = await this.client
      .query(
        SitemapNodesDocument,
        { query: text, scopeId, recursive: false },
        { requestPolicy: 'network-only' },
      )
      .toPromise();
    if (epoch !== this.epoch) return;
    this.loading = false;
    if (result.error || !result.data) {
      this.message = result.error?.message ?? 'Loading the sitemap failed';
      this.listen();
      return;
    }
    this.lastRefresh = Date.now();
    this.applied = text;
    this.scopeId = scopeId;
    this.children = { '': result.data.sitemapNodes };
    this.expanded.clear();
    this.recursiveRoots.clear();
    this.busy.clear();
    this.clearTable();
    this.target = result.data.sitemapNodes[0] ?? null;
    this.confirmation = null;
    this.message = null;
    this.listen();
  }

  async toggle(node: SitemapNode, recursive = false): Promise<void> {
    if (!node.hasChildren || this.busy.has(node.id)) return;
    if (this.expanded.has(node.id) && !recursive) {
      this.expanded.delete(node.id);
      return;
    }
    const epoch = this.epoch;
    this.busy.add(node.id);
    const result = await this.client
      .query(
        SitemapNodesDocument,
        {
          parentId: node.id,
          query: this.applied,
          scopeId: this.scopeId,
          recursive,
        },
        { requestPolicy: 'network-only' },
      )
      .toPromise();
    if (epoch !== this.epoch) return;
    this.busy.delete(node.id);
    if (result.error || !result.data) {
      this.message =
        (result.error?.message ?? 'Loading children failed') +
        (recursive ? ' Expand level by level.' : '');
      return;
    }
    this.children = {
      ...this.children,
      ...childGroups(node.id, result.data.sitemapNodes, recursive),
    };
    this.expanded.add(node.id);
    if (recursive) {
      this.recursiveRoots.add(node.id);
      result.data.sitemapNodes.forEach((child) => {
        if (child.hasChildren) this.expanded.add(child.id);
      });
    }
    this.message = null;
  }

  select(node: SitemapNode): void {
    this.selected = node;
    this.target = node;
    this.selection.clear();
    this.detail = null;
    this.detailToken++;
    this.rows = [];
    this.total = node.requestCount;
    this.cursor = null;
    this.detailLoading = false;
    void this.loadRows();
  }

  private clearTable(): void {
    this.tableToken++;
    this.detailToken++;
    this.selected = null;
    this.rows = [];
    this.total = 0;
    this.cursor = null;
    this.tableLoading = false;
    this.selection.clear();
    this.detail = null;
    this.detailLoading = false;
    this.rowsDirty = false;
  }

  async loadRows(more = false, count = 1000): Promise<void> {
    if (!this.selected) return;
    const token = ++this.tableToken;
    this.tableLoading = true;
    const rows: HistoryRowFragment[] = [];
    let cursor = more ? this.cursor : null;
    let total: number;
    do {
      const result = await this.client
        .query(
          SitemapRequestsDocument,
          {
            query: this.selected.query,
            scopeId: this.scopeId,
            after: cursor,
            sort: requestSort(this.sort),
          },
          { requestPolicy: 'network-only' },
        )
        .toPromise();
      if (token !== this.tableToken) return;
      if (result.error || !result.data) {
        this.tableLoading = false;
        this.rowsDirty = true;
        this.message = result.error?.message ?? 'Loading requests failed';
        return;
      }
      const connection = result.data.requests;
      rows.push(...connection.edges.map((edge) => edge.node));
      total = connection.totalCount;
      cursor = connection.pageInfo.hasNextPage ? (connection.pageInfo.endCursor ?? null) : null;
    } while (!more && rows.length < count && cursor);
    this.tableLoading = false;
    this.rows = more ? [...this.rows, ...rows] : rows;
    this.total = total;
    this.cursor = cursor;
    const id = this.selection.anchor;
    if (id && !this.rows.some((row) => row.id === id)) {
      this.selection.clear();
      this.detail = null;
      this.detailToken++;
      this.detailLoading = false;
    } else if (id) void this.showDetail(id);
  }

  async showDetail(id: string | null): Promise<void> {
    const token = ++this.detailToken;
    this.detail = null;
    this.detailLoading = !!id;
    if (!id) return;
    const result = await this.client
      .query(HistoryRequestDocument, { id }, { requestPolicy: 'network-only' })
      .toPromise();
    if (token !== this.detailToken) return;
    this.detailLoading = false;
    if (result.error) this.message = result.error.message;
    if (result.data?.request) this.detail = buildDetail(result.data.request);
  }

  async refresh(): Promise<void> {
    const token = ++this.refreshToken;
    const epoch = this.epoch;
    this.lastRefresh = Date.now();
    // eslint-disable-next-line svelte/prefer-svelte-reactivity -- T-020: local worklist with no reactive consumers.
    const parents = new Set([
      '',
      ...this.visible.filter(({ node }) => this.expanded.has(node.id)).map(({ node }) => node.id),
      ...(this.selected?.parentId ? [this.selected.parentId] : []),
      ...(this.confirmation?.node.parentId ? [this.confirmation.node.parentId] : []),
    ]);
    const omitDescendants = (parentId: string) => {
      for (const child of this.children[parentId] ?? []) {
        parents.delete(child.id);
        omitDescendants(child.id);
      }
    };
    for (const parentId of parents) {
      if (this.recursiveRoots.has(parentId)) omitDescendants(parentId);
    }
    const branches = [...parents].map((parentId) => ({
      parentId,
      recursive: this.recursiveRoots.has(parentId),
    }));
    const results = await Promise.all(
      branches.map(({ parentId, recursive }) =>
        this.client
          .query(
            SitemapNodesDocument,
            {
              parentId: parentId || null,
              query: this.applied,
              scopeId: this.scopeId,
              recursive,
            },
            { requestPolicy: 'network-only' },
          )
          .toPromise(),
      ),
    );
    if (epoch !== this.epoch || token !== this.refreshToken) return;
    const failure = results.find((result) => result.error || !result.data);
    if (failure) {
      this.message = failure.error?.message ?? 'Refreshing the sitemap failed';
      return;
    }
    const children = { ...this.children };
    results.forEach((result, i) => {
      const { parentId, recursive } = branches[i]!;
      Object.assign(children, childGroups(parentId, result.data!.sitemapNodes, recursive));
    });
    this.children = children;
    const nodes = results.flatMap((result) => result.data!.sitemapNodes);
    if (this.target) this.target = nodes.find((node) => node.id === this.target?.id) ?? null;
    if (this.confirmation) {
      const node = nodes.find((node) => node.id === this.confirmation?.node.id);
      this.confirmation = node ? { ...this.confirmation, node } : null;
    }
    if (this.selected) {
      this.selected = nodes.find((node) => node.id === this.selected?.id) ?? null;
      if (!this.selected) this.clearTable();
      else if (this.selected.requestCount !== this.total || this.rowsDirty) {
        this.rowsDirty = false;
        await this.loadRows(false, this.rows.length);
      }
    }
  }

  private scheduleRefresh = (): void => {
    if (this.timer !== undefined || !this.running) return;
    this.timer = setTimeout(
      () => {
        this.timer = undefined;
        void this.refresh();
      },
      Math.max(0, 1000 - (Date.now() - this.lastRefresh)),
    );
  };

  private listen(): void {
    if (!this.running) return;
    this.listening = true;
    const ended = () => {
      if (!this.listening) return;
      this.unlisten();
      this.timer = setTimeout(() => {
        this.timer = undefined;
        this.listen();
        this.scheduleRefresh();
      }, 1000);
    };
    const variables = { query: this.applied, scopeId: this.scopeId };
    const streams: Source<unknown>[] = [
      this.client.subscription(HistoryRequestAddedDocument, variables),
      pipe(
        this.client.subscription(HistoryRequestUpdatedDocument, variables),
        tap(() => {
          this.rowsDirty = true;
        }),
      ),
      this.client.subscription(HistoryRequestsDeletedDocument, {}),
    ];
    this.stops = streams.map((stream) => {
      const subscription = pipe(stream, onEnd(ended), subscribe(this.scheduleRefresh));
      return () => subscription.unsubscribe();
    });
  }

  async remove(): Promise<void> {
    const confirmation = this.confirmation;
    if (!confirmation || this.deleting) return;
    const epoch = this.epoch;
    this.deleting = true;
    const result = await this.client
      .mutation(SitemapDeleteDocument, {
        query: confirmation.node.query,
        scopeId: confirmation.scopeId,
      })
      .toPromise();
    this.deleting = false;
    if (epoch !== this.epoch) return;
    if (result.error) {
      this.message = result.error.message;
      return;
    }
    this.confirmation = null;
    this.scheduleRefresh();
  }

  async addHostToScope(): Promise<void> {
    const scope = this.scopes.find((scope) => scope.id === this.scopeId);
    if (!scope || !this.target) return;
    const host = this.target.host.includes(':') ? `[${this.target.host}]` : this.target.host;
    if (scope.allow.includes(host)) return;
    const epoch = this.epoch;
    const result = await this.client
      .mutation(UpdateScopePresetDocument, {
        id: scope.id,
        name: scope.name,
        allow: [...scope.allow, host],
        deny: scope.deny,
      })
      .toPromise();
    if (epoch !== this.epoch) return;
    if (result.error || !result.data) {
      this.message = result.error?.message ?? 'Updating the scope failed';
      return;
    }
    this.scopes = this.scopes.map((other) =>
      other.id === scope.id ? result.data!.updateScopePreset : other,
    );
    await this.apply();
  }

  async copyUrl(): Promise<void> {
    if (!this.target) return;
    try {
      await navigator.clipboard.writeText(this.target.url);
    } catch {
      this.message = 'Clipboard unavailable';
    }
  }

  async complete(text: string, cursor: number) {
    const result = await this.client
      .query(HistoryCompletionsDocument, { text, cursor })
      .toPromise();
    return result.data?.queryCompletions ?? null;
  }
}
