<script lang="ts">
  let { shown, total, onmore }: { shown: number; total: number; onmore: () => void } = $props();
  const kb = (bytes: number) => `${Math.round(bytes / 1024)} KB`;
</script>

<div class="flex items-center gap-2 border-b border-line px-2 py-1 text-xs text-muted">
  <span>Showing {kb(shown)} of {kb(total)}</span>
  <button type="button" class="text-accent hover:underline" onclick={onmore}>Show more</button>
</div>
