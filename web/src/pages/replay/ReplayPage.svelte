<script lang="ts">
  // Replay shell (F-REP-1, docs/02 §3.7): collections and tabs; the session body is T-107's.
  import type { Client } from '@urql/core';
  import { projects } from '$lib/stores/project.svelte';
  import ReplayShell from './ReplayShell.svelte';

  let { client, projectId }: { client?: Client; projectId?: string } = $props();
  // Per page visit, so only a switch while Replay is shown counts as one.
  const last = { id: null as string | null };
</script>

<!-- Open tabs are per project: a project switch starts a fresh shell. Waiting for the project
     list keeps a startup deep link from being filed under the fallback key. -->
{#if projectId || !projects.loading}
  {#key projectId ?? projects.current?.id ?? 'default'}
    <ReplayShell projectId={projectId ?? projects.current?.id ?? 'default'} {client} {last} />
  {/key}
{/if}
