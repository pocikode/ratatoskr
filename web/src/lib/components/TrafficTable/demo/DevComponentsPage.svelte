<script lang="ts">
  import { router } from '$lib/router/router.svelte';
  import MessageViewer from '../../MessageViewer/MessageViewer.svelte';
  import SplitPane from '../../SplitPane/SplitPane.svelte';
  import TrafficTable from '../TrafficTable.svelte';
  import { Selection } from '../selection.svelte';
  import type { ColumnConfig, RowColor, SortState, TrafficColumn } from '../types';

  interface DemoRow {
    id: number;
    method: string;
    host: string;
    path: string;
    status: number;
    length: number;
    rtt: number;
    created: number;
    color: RowColor | null;
  }

  const HOSTS = ['api.acme.test', 'app.acme.test', 'cdn.acme.test', 'auth.acme.test'];
  const METHODS = ['GET', 'POST', 'PUT', 'DELETE', 'PATCH'];
  const STATUS = [200, 201, 204, 301, 400, 401, 403, 404, 500, 502];
  const COLORS: RowColor[] = ['red', 'orange', 'yellow', 'green', 'teal', 'blue', 'purple', 'pink'];

  // Deterministic rows so the demo (and screenshots) look the same every run.
  function random(seed: number) {
    return () => {
      seed = (seed + 0x6d2b79f5) | 0;
      let t = Math.imul(seed ^ (seed >>> 15), 1 | seed);
      t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
      return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
    };
  }

  function generate(count: number): DemoRow[] {
    const rnd = random(42);
    return Array.from({ length: count }, (_, i) => ({
      id: count - i,
      method: METHODS[Math.floor(rnd() * METHODS.length)]!,
      host: HOSTS[Math.floor(rnd() * HOSTS.length)]!,
      path: `/v${1 + Math.floor(rnd() * 3)}/${['users', 'login', 'orders', 'files'][Math.floor(rnd() * 4)]}/${Math.floor(rnd() * 9999)}`,
      status: STATUS[Math.floor(rnd() * STATUS.length)]!,
      length: Math.floor(rnd() * 90000),
      rtt: Math.floor(rnd() * 1200),
      created: Date.now() - i * 1000,
      color: i % 97 === 0 ? COLORS[Math.floor(rnd() * COLORS.length)]! : null,
    }));
  }

  const rows = generate(100_000);
  const selection = new Selection();
  let sort = $state<SortState | null>(null);
  let saved = $state('default layout');
  let contextRow = $state('');

  const columns: TrafficColumn<DemoRow>[] = [
    { id: 'id', title: 'ID', width: 64, align: 'end', value: (r) => r.id },
    { id: 'method', title: 'Method', width: 72, value: (r) => r.method },
    { id: 'host', title: 'Host', width: 160, value: (r) => r.host },
    { id: 'path', title: 'Path', width: 240, value: (r) => r.path },
    { id: 'status', title: 'Status', width: 64, align: 'end', value: (r) => r.status },
    { id: 'length', title: 'Length', width: 76, align: 'end', value: (r) => r.length },
    { id: 'rtt', title: 'RTT', width: 60, align: 'end', value: (r) => r.rtt },
    {
      id: 'created',
      title: 'Created',
      width: 88,
      value: (r) => new Date(r.created).toLocaleTimeString(),
    },
    { id: 'color', title: 'Colour', width: 72, value: (r) => r.color ?? '', sortable: false },
  ];

  const sorted = $derived.by(() => {
    if (!sort) return rows;
    const column = columns.find((c) => c.id === sort!.columnId);
    if (!column) return rows;
    const sign = sort.direction === 'asc' ? 1 : -1;
    const text = (v: string | number | null) => (v === null ? '' : v);
    return [...rows].sort((a, b) => {
      const x = text(column.value(a));
      const y = text(column.value(b));
      const order =
        typeof x === 'number' && typeof y === 'number' ? x - y : String(x).localeCompare(String(y));
      return order * sign;
    });
  });

  const request =
    'GET /v2/login?next=%2Fhome HTTP/1.1\r\n' +
    'Host: api.acme.test\r\n' +
    'User-Agent: ratatoskr-demo\r\n' +
    'Accept: application/json\r\n\r\n';

  const response =
    'HTTP/1.1 401 Unauthorized\r\n' +
    'Content-Type: application/json\r\n' +
    'Content-Length: 76\r\n\r\n' +
    '{"error":"invalid_credentials","request_id":"8f3c2a1b","retry_after":30}';

  let big = $state(false);
  const responseBody = $derived(
    big
      ? `HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 5000000\r\n\r\n${'A web security proxy body. '.repeat(200_000)}`
      : response,
  );

  const componentsRoute = $derived(router.location.params[0] === 'components');

  function persist(config: ColumnConfig) {
    saved = `${config.order.length} columns · ${config.hidden.length} hidden · host ${config.widths.host}px`;
  }
</script>

{#if !componentsRoute}
  <section class="flex h-full flex-col items-center justify-center gap-2 text-center">
    <h1 class="text-lg font-semibold">Dev components</h1>
    <a class="text-accent underline" href="#/dev/components">Open the component gallery</a>
  </section>
{:else}
  <div class="flex h-[calc(100vh-140px)] flex-col gap-2">
    <div class="flex flex-wrap items-center gap-3 text-xs text-muted">
      <h1 class="text-base font-semibold text-fg">Component gallery</h1>
      <span aria-live="polite">
        {selection.size} selected · sort: {sort ? `${sort.columnId} ${sort.direction}` : 'none'} · context:
        {contextRow || 'none'}
      </span>
      <span>layout: {saved}</span>
      <span class="flex-1"></span>
      <button
        type="button"
        class="rounded-token border border-line px-2 py-0.5 hover:bg-hover"
        onclick={() => (big = !big)}
      >
        {big ? 'Use small response' : 'Use 5 MB response'}
      </button>
    </div>

    <div class="min-h-0 flex-1">
      <SplitPane
        direction="vertical"
        defaultSizes={[55, 45]}
        label="Resize table and viewers"
        onsizeschange={(sizes) => (saved = `panes ${Math.round(sizes[0])}/${Math.round(sizes[1])}`)}
      >
        {#snippet first()}
          <TrafficTable
            rows={sorted}
            {columns}
            rowKey={(r) => String(r.id)}
            {selection}
            color={(r) => r.color}
            bind:sort
            label="Demo traffic"
            onconfigchange={persist}
            onrowopen={(r) => (contextRow = `open ${r.method} ${r.path}`)}
            onrowcontextmenu={(r) => (contextRow = `${r.method} ${r.host}${r.path}`)}
          />
        {/snippet}
        {#snippet second()}
          <SplitPane direction="horizontal" label="Resize request and response">
            {#snippet first()}
              <MessageViewer
                raw={request}
                title="Request"
                modes={['raw', 'pretty', 'hex', 'table']}
              />
            {/snippet}
            {#snippet second()}
              <MessageViewer
                raw={responseBody}
                title="Response"
                previewUrl="about:blank"
                modes={['raw', 'pretty', 'hex', 'table', 'preview']}
              />
            {/snippet}
          </SplitPane>
        {/snippet}
      </SplitPane>
    </div>
  </div>
{/if}
