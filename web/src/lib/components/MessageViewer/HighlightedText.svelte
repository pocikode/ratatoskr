<script lang="ts">
  import type { Segment } from './format';

  let { parts, wrap }: { parts: Segment[]; wrap: boolean } = $props();
</script>

<!-- One line on purpose: whitespace inside <pre> is content. -->
<pre
  class="p-2 font-mono text-xs {wrap
    ? 'break-all whitespace-pre-wrap'
    : 'whitespace-pre'}">{#each parts as part, i (i)}{#if part.match}<mark
        class="bg-warn/40 text-fg"
        data-current={part.current ? '' : undefined}>{part.text}</mark
      >{:else}{part.text}{/if}{/each}</pre>
