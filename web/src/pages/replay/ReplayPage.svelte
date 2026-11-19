<script lang="ts">
  // Replay shell (F-REP-1, docs/02 §3.7): collections and tabs; the session body is T-107's.
  import type { Client } from '@urql/core';
  import { projects } from '$lib/stores/project.svelte';
  import ReplayShell from './ReplayShell.svelte';

  let { client, projectId }: { client?: Client; projectId?: string } = $props();
</script>

<!-- Open tabs are per project: a project switch starts a fresh shell. -->
{#key projectId ?? projects.current?.id ?? 'default'}
  <ReplayShell projectId={projectId ?? projects.current?.id ?? 'default'} {client} />
{/key}
