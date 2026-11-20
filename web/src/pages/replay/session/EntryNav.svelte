<script lang="ts">
  // Entry history of a session (F-REP-2): `‹ n/N ›` plus a dropdown, newest first.
  import type { Entry } from './session.svelte';

  interface Props {
    entries: Entry[];
    viewed: Entry | null;
    position: number;
    onselect: (id: string) => void;
  }

  let { entries, viewed, position, onselect }: Props = $props();

  const label = (entry: Entry, index: number) => {
    const time = new Date(entry.createdAt).toLocaleTimeString();
    return `${entries.length - index} · ${time}${entry.error ? ' · failed' : ''}`;
  };
</script>

{#if entries.length > 0}
  <div class="flex items-center gap-1 text-xs">
    <button
      type="button"
      aria-label="Newer entry"
      class="rounded-token px-1.5 py-0.5 text-muted hover:bg-hover disabled:opacity-40"
      disabled={position <= 1}
      onclick={() => {
        const prev = entries[position - 2];
        if (prev) onselect(prev.id);
      }}>‹</button
    >
    <span class="text-muted tabular-nums" aria-live="polite">{position}/{entries.length}</span>
    <button
      type="button"
      aria-label="Older entry"
      class="rounded-token px-1.5 py-0.5 text-muted hover:bg-hover disabled:opacity-40"
      disabled={position >= entries.length}
      onclick={() => {
        const next = entries[position];
        if (next) onselect(next.id);
      }}>›</button
    >
    <select
      aria-label="Entry history"
      class="max-w-44 rounded-token border border-line bg-subtle px-1.5 py-0.5 text-xs"
      value={viewed?.id ?? ''}
      onchange={(e) => onselect(e.currentTarget.value)}
    >
      {#each entries as entry, index (entry.id)}
        <option value={entry.id}>{label(entry, index)}</option>
      {/each}
    </select>
  </div>
{/if}
