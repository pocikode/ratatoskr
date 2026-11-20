<script lang="ts">
  // Match & Replace page (F-MR-1, docs/02 §3.6). Collections are per project: a project switch
  // starts a fresh shell, like Replay.
  import type { Client } from '@urql/core';
  import { projects } from '$lib/stores/project.svelte';
  import MatchReplaceShell from './MatchReplaceShell.svelte';

  let { client, projectId }: { client?: Client; projectId?: string } = $props();
</script>

{#if projectId || !projects.loading}
  {#key projectId ?? projects.current?.id ?? 'default'}
    <MatchReplaceShell {client} />
  {/key}
{/if}
