<script lang="ts">
  import { File, Folder, Globe, Search, ChevronDown, ChevronRight } from '@lucide/svelte';
  import ContextMenu from '$lib/components/ContextMenu/ContextMenu.svelte';
  import type { SitemapState, TreeRow } from './sitemap.svelte';

  let { state: sitemap }: { state: SitemapState } = $props();
  const uid = $props.id();
  const ROW_HEIGHT = 28;
  let viewport = $state<HTMLDivElement | null>(null);
  let scrollTop = $state(0);
  let height = $state(600);
  let active = $state(0);
  const start = $derived(Math.max(0, Math.floor(scrollTop / ROW_HEIGHT) - 8));
  const end = $derived(
    Math.min(sitemap.visible.length, Math.ceil((scrollTop + height) / ROW_HEIGHT) + 8),
  );

  $effect(() => {
    if (!viewport || typeof ResizeObserver === 'undefined') return;
    const el = viewport;
    const observer = new ResizeObserver(() => (height = el.clientHeight || 600));
    observer.observe(el);
    return () => observer.disconnect();
  });

  function focus(index: number) {
    active = Math.max(0, Math.min(sitemap.visible.length - 1, index));
    if (!viewport) return;
    const top = active * ROW_HEIGHT;
    if (top < viewport.scrollTop) viewport.scrollTop = top;
    else if (top + ROW_HEIGHT > viewport.scrollTop + height)
      viewport.scrollTop = top + ROW_HEIGHT - height;
    scrollTop = viewport.scrollTop;
    const row = sitemap.visible[active];
    if (row) sitemap.target = row.node;
  }

  function click(row: TreeRow, index: number, event: MouseEvent) {
    focus(index);
    viewport?.focus();
    sitemap.select(row.node);
    void sitemap.toggle(row.node, event.shiftKey);
  }

  function keydown(event: KeyboardEvent) {
    const row = sitemap.visible[active];
    if (!row) return;
    const { node } = row;
    if (event.key === 'ArrowDown') focus(active + 1);
    else if (event.key === 'ArrowUp') focus(active - 1);
    else if (event.key === 'Home') focus(0);
    else if (event.key === 'End') focus(sitemap.visible.length - 1);
    else if (event.key === 'ArrowRight' && node.hasChildren) {
      if (sitemap.expanded.has(node.id)) focus(active + 1);
      else void sitemap.toggle(node, event.shiftKey);
    } else if (event.key === 'ArrowLeft') {
      if (sitemap.expanded.has(node.id)) sitemap.expanded.delete(node.id);
      else {
        const index = sitemap.visible.findIndex((row) => row.node.id === node.parentId);
        if (index >= 0) focus(index);
      }
    } else if (event.key === 'Enter' || event.key === ' ') {
      sitemap.select(node);
      if (event.shiftKey) void sitemap.toggle(node, true);
    } else return;
    event.preventDefault();
  }
</script>

<ContextMenu ids={['sitemap.copy', 'sitemap.scope', 'sitemap.delete']}>
  <div
    bind:this={viewport}
    role="tree"
    aria-label="Sitemap"
    aria-busy={sitemap.loading}
    aria-activedescendant={active >= start && active < end ? `${uid}-${active}` : undefined}
    tabindex="0"
    class="h-full overflow-auto text-xs outline-none focus:ring-1 focus:ring-accent"
    onscroll={(event) => (scrollTop = event.currentTarget.scrollTop)}
    onkeydown={keydown}
    onfocus={() => {
      const row = sitemap.visible[active];
      if (row) sitemap.target = row.node;
    }}
  >
    {#if sitemap.visible.length === 0}
      <p class="p-3 text-muted">{sitemap.loading ? 'Loading…' : 'No requests match.'}</p>
    {:else}
      <div class="relative" style:height="{sitemap.visible.length * ROW_HEIGHT}px">
        <div style:transform="translateY({start * ROW_HEIGHT}px)">
          {#each sitemap.visible.slice(start, end) as row, i (row.node.id)}
            {@const node = row.node}
            {@const Icon =
              node.kind === 'HOST'
                ? Globe
                : node.kind === 'DIRECTORY'
                  ? Folder
                  : node.kind === 'QUERY'
                    ? Search
                    : File}
            <div
              id="{uid}-{start + i}"
              role="treeitem"
              tabindex="-1"
              aria-label="{node.label} · {node.requestCount} requests"
              aria-level={row.depth}
              aria-posinset={row.position}
              aria-setsize={row.siblings}
              aria-selected={sitemap.selected?.id === node.id}
              aria-expanded={node.hasChildren ? sitemap.expanded.has(node.id) : undefined}
              aria-busy={sitemap.busy.has(node.id)}
              class="flex cursor-default items-center gap-1 pr-2 hover:bg-hover"
              class:bg-accent-soft={sitemap.selected?.id === node.id}
              class:text-accent={sitemap.selected?.id === node.id}
              style:height="{ROW_HEIGHT}px"
              style:padding-left="{(row.depth - 1) * 16 + 4}px"
              onclick={(event) => click(row, start + i, event)}
              onkeydown={keydown}
              oncontextmenu={() => {
                focus(start + i);
                sitemap.target = node;
              }}
            >
              <span class="w-3 shrink-0" aria-hidden="true">
                {#if node.hasChildren}
                  {#if sitemap.expanded.has(node.id)}<ChevronDown size={12} />{:else}<ChevronRight
                      size={12}
                    />{/if}
                {/if}
              </span>
              <Icon size={14} aria-hidden="true" class="shrink-0 text-muted" />
              <span class="min-w-0 flex-1 truncate" title={node.url}>{node.label}</span>
              <span class="text-muted">{node.requestCount}</span>
            </div>
          {/each}
        </div>
      </div>
    {/if}
  </div>
</ContextMenu>

<style>
  :global([data-context-menu-trigger]:has(> [role='tree'])) {
    height: 100%;
    min-height: 0;
  }
</style>
