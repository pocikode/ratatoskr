<script lang="ts">
  import { DropdownMenu } from 'bits-ui';
  import { getApi } from '$lib/api/client';
  import { connection, type ConnectionState } from '$lib/api/connection.svelte';
  import { environment as defaultEnv, type EnvironmentStore } from '$lib/stores/environment.svelte';

  let {
    conn = connection,
    start = () => conn.start(getApi()),
    env = defaultEnv,
  }: {
    conn?: ConnectionState;
    start?: () => () => void;
    env?: EnvironmentStore;
  } = $props();

  $effect(() => start());
  $effect(() => env.start());

  /** Why the last switch failed; the old selection stays until a switch succeeds. */
  let error = $state<string | null>(null);

  async function select(id: string | null) {
    error = (await env.select(id)) ? null : env.error;
  }

  const projectEnvs = $derived(env.environments.filter((e) => !e.isGlobal));

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

  <DropdownMenu.Root>
    <DropdownMenu.Trigger
      class="rounded-token px-1 hover:bg-hover"
      aria-label="Environment"
      title="Selected environment (F-ENV-1)"
    >
      env: {env.selected?.name ?? 'No environment'}
    </DropdownMenu.Trigger>
    <DropdownMenu.Portal>
      <DropdownMenu.Content
        align="start"
        sideOffset={4}
        class="z-50 min-w-40 rounded-token border border-line bg-elevated p-1 shadow-lg"
      >
        <DropdownMenu.Item
          class="flex cursor-default items-center justify-between rounded-token px-2 py-1 outline-none data-highlighted:bg-hover"
          onSelect={() => void select(null)}
        >
          No environment
          {#if !env.selected}<span aria-label="current">✓</span>{/if}
        </DropdownMenu.Item>
        {#each projectEnvs as e (e.id)}
          <DropdownMenu.Item
            class="flex cursor-default items-center justify-between rounded-token px-2 py-1 outline-none data-highlighted:bg-hover"
            onSelect={() => void select(e.id)}
          >
            {e.name}
            {#if e.selected}<span aria-label="current">✓</span>{/if}
          </DropdownMenu.Item>
        {/each}
      </DropdownMenu.Content>
    </DropdownMenu.Portal>
  </DropdownMenu.Root>

  {#if error}
    <span role="alert" class="min-w-0 truncate text-danger" title={error}>{error}</span>
  {/if}

  {#if conn.server}
    <span
      class="ml-auto"
      title="Commit {conn.server.commit}, built with {conn.server.goVersion}"
      data-testid="server-version">Ratatoskr {conn.server.version}</span
    >
  {/if}
</footer>
