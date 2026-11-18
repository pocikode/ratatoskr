<script lang="ts">
  // Scope selector for a query bar (docs/02 §2): one preset or none. Presentational —
  // the page owns the list and reacts to `onchange` (T-019, F-SCOPE-1).
  interface Props {
    presets: readonly { id: string; name: string }[];
    /** Selected preset id; null = no scope. */
    value?: string | null;
    onchange?: (id: string | null) => void;
    label?: string;
  }

  let { presets, value = $bindable(null), onchange, label = 'Scope' }: Props = $props();

  const selected = $derived(value !== null && presets.some((p) => p.id === value) ? value : '');
</script>

<select
  aria-label={label}
  class="max-w-44 shrink-0 cursor-pointer rounded-token border border-line bg-elevated px-1.5
    py-0.5 text-xs text-fg"
  value={selected}
  onchange={(event) => {
    const id = event.currentTarget.value || null;
    value = id;
    onchange?.(id);
  }}
>
  <option value="">No scope</option>
  {#each presets as preset (preset.id)}
    <option value={preset.id}>{preset.name}</option>
  {/each}
</select>
