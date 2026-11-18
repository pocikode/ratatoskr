<script lang="ts">
  // Quick-apply chips for filter presets (F-FILTER-1, docs/02 §2): clicking one applies
  // `preset:"Name"` to the query bar. Presentational — the page owns the list (T-019).
  interface Props {
    presets: readonly { id: string; name: string; alias: string | null }[];
    /** Preset the applied query references, for the pressed state. */
    active?: string | null;
    onapply: (preset: { id: string; name: string; alias: string | null }) => void;
  }

  let { presets, active = null, onapply }: Props = $props();
</script>

{#if presets.length > 0}
  <div
    role="group"
    aria-label="Filter presets"
    class="flex min-w-0 shrink-0 items-center gap-1 overflow-x-auto"
  >
    {#each presets as preset (preset.id)}
      <button
        type="button"
        aria-pressed={active === preset.name}
        title={preset.alias ? `preset:${preset.alias}` : `preset:"${preset.name}"`}
        class="shrink-0 rounded-full border px-2 py-0.5 text-xs whitespace-nowrap hover:bg-hover
          {active === preset.name ? 'border-accent text-accent' : 'border-line text-muted'}"
        onclick={() => onapply(preset)}
      >
        {preset.name}
      </button>
    {/each}
  </div>
{/if}
