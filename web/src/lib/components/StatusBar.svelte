<script lang="ts">
  import { getApi } from '$lib/api/client';
  import { connection, type ConnectionState } from '$lib/api/connection.svelte';

  // Listener/project/env slots arrive with their tasks.
  let {
    conn = connection,
    start = () => connection.start(getApi()),
  }: { conn?: ConnectionState; start?: () => () => void } = $props();

  $effect(() => start());

  const LABELS = {
    connecting: 'Connecting…',
    connected: 'Connected',
    disconnected: 'Disconnected, retrying',
    unauthorized: 'Not authorized',
  } as const;

  const DOTS = {
    connecting: 'bg-warn',
    connected: 'bg-success',
    disconnected: 'bg-danger',
    unauthorized: 'bg-danger',
  } as const;
</script>

<footer
  class="flex shrink-0 items-center gap-4 border-t border-line bg-elevated px-3 text-xs text-muted"
  style:height="var(--statusbar-h)"
>
  <span class="flex items-center gap-1" role="status" aria-label="Server connection">
    <span class="inline-block size-2 rounded-full {DOTS[conn.status]}" aria-hidden="true"></span>
    {LABELS[conn.status]}
  </span>
  {#if conn.server}
    <span
      class="ml-auto"
      title="Commit {conn.server.commit}, built with {conn.server.goVersion}"
      data-testid="server-version">Ratatoskr {conn.server.version}</span
    >
  {/if}
</footer>
