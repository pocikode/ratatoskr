<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { PaneGroupAPI } from 'paneforge';
  import { Pane, PaneGroup, PaneResizer } from 'paneforge';

  interface Props {
    direction?: 'horizontal' | 'vertical';
    /** Percentage split at mount and after double-clicking the divider. */
    defaultSizes?: [number, number];
    /** Fired on drag/keyboard resize; persist into the page's `ui_state`. */
    onsizeschange?: (sizes: [number, number]) => void;
    label?: string;
    first: Snippet;
    second: Snippet;
  }

  let {
    direction = 'horizontal',
    defaultSizes = [55, 45],
    onsizeschange,
    label = 'Resize panes',
    first,
    second,
  }: Props = $props();

  let group = $state<PaneGroupAPI | null>(null);

  function reset() {
    group?.setLayout([...defaultSizes]);
  }
</script>

<PaneGroup
  bind:this={group}
  {direction}
  class="h-full w-full"
  onLayoutChange={(layout) => {
    const [a = defaultSizes[0], b = defaultSizes[1]] = layout;
    onsizeschange?.([a, b]);
  }}
>
  <Pane defaultSize={defaultSizes[0]} minSize={10} class="flex min-h-0 min-w-0 flex-col">
    {@render first()}
  </Pane>
  <PaneResizer
    class="group relative flex shrink-0 items-center justify-center bg-line hover:bg-accent data-[active]:bg-accent
      {direction === 'horizontal' ? 'w-1 cursor-col-resize' : 'h-1 cursor-row-resize'}"
    aria-label={label}
    ondblclick={reset}
  ></PaneResizer>
  <Pane defaultSize={defaultSizes[1]} minSize={10} class="flex min-h-0 min-w-0 flex-col">
    {@render second()}
  </Pane>
</PaneGroup>
