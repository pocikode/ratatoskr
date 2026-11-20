<script lang="ts" module>
  import type { CommandContext } from '$lib/commands/registry';

  export interface SidebarItem {
    id: string;
    label: string;
    /** Secondary text, e.g. host. */
    detail?: string;
    /** `--row-<name>` token for the colour tag. */
    color?: string | null;
  }
  export interface SidebarGroup {
    id: string;
    label: string;
    items: SidebarItem[];
  }
  export interface RowTarget {
    kind: 'item' | 'group';
    id: string;
  }
  export interface RowMenu {
    ids: string[];
    context?: CommandContext;
  }
</script>

<script lang="ts">
  import { tick, type Snippet } from 'svelte';
  import ChevronDownIcon from '@lucide/svelte/icons/chevron-down';
  import ContextMenu from '$lib/components/ContextMenu/ContextMenu.svelte';

  interface Props {
    groups: SidebarGroup[];
    search: string;
    activeId?: string | null;
    /** Row in inline rename, if any. */
    renaming?: RowTarget | null;
    label?: string;
    onsearch: (value: string) => void;
    onselect: (id: string) => void;
    onrename: (target: RowTarget, name: string) => void;
    oncancelrename: () => void;
    /** F2 on a row. */
    onstartrename?: (target: RowTarget) => void;
    /**
     * `index` counts the target group's items without the dragged one (0-based). Return the
     * pending move so a keyboard move can restore focus after a rollback.
     */
    onmove: (id: string, groupId: string, index: number) => void | Promise<unknown>;
    /** Context menu of a row, as command ids. */
    menu?: (target: RowTarget) => RowMenu;
    footer?: Snippet;
  }

  let {
    groups,
    search,
    activeId = null,
    renaming = null,
    label = 'Sessions',
    onsearch,
    onselect,
    onrename,
    oncancelrename,
    onstartrename,
    onmove,
    menu,
    footer,
  }: Props = $props();

  let collapsed = $state<Record<string, boolean>>({});
  let dragId = $state<string | null>(null);
  let tree = $state<HTMLElement | null>(null);

  const searching = $derived(search.trim() !== '');
  const open = (g: SidebarGroup) => searching || !collapsed[g.id];
  const key = (t: RowTarget) => `${t.kind}:${t.id}`;

  function focusRow(delta: number, from: HTMLElement) {
    const rows = [...(tree?.querySelectorAll<HTMLElement>('[data-row]') ?? [])];
    rows[rows.indexOf(from) + delta]?.focus();
  }

  // Alt+↑/↓ reorders within the group, Alt+PageUp/PageDown appends to the previous/next group.
  function keyMove(e: KeyboardEvent, id: string, g: SidebarGroup, gi: number): boolean {
    const i = g.items.findIndex((x) => x.id === id);
    const to =
      e.key === 'ArrowUp'
        ? [g, i - 1]
        : e.key === 'ArrowDown'
          ? [g, i + 1]
          : e.key === 'PageUp'
            ? [groups[gi - 1], Infinity]
            : e.key === 'PageDown'
              ? [groups[gi + 1], Infinity]
              : null;
    if (!to) return false;
    const [dest, index] = to as [SidebarGroup | undefined, number];
    if (dest && index >= 0 && (dest !== g || index < g.items.length)) {
      collapsed[dest.id] = false; // keep the destination mounted so focus can return to the row
      const done = onmove(
        id,
        dest.id,
        Math.min(index, dest.items.filter((x) => x.id !== id).length),
      );
      // The row may be re-mounted in another group: put focus back once the tree has updated,
      // and again after a rollback re-mounts it in the original group (focus then fell to body).
      const refocus = () =>
        tree?.querySelector<HTMLElement>(`[data-row="${key({ kind: 'item', id })}"]`)?.focus();
      void tick().then(refocus);
      void Promise.resolve(done)
        .then(tick)
        .then(() => {
          if (document.activeElement === document.body) refocus();
        });
    }
    return true;
  }

  function onkeydown(e: KeyboardEvent, t: RowTarget, g: SidebarGroup, gi: number) {
    const el = e.currentTarget as HTMLElement;
    if (e.altKey && t.kind === 'item') {
      if (!keyMove(e, t.id, g, gi)) return;
    } else if (e.key === 'ArrowDown') focusRow(1, el);
    else if (e.key === 'ArrowUp') focusRow(-1, el);
    else if (e.key === 'ArrowRight' && t.kind === 'group') collapsed[g.id] = false;
    else if (e.key === 'ArrowLeft' && t.kind === 'group') collapsed[g.id] = true;
    else if (e.key === 'F2') onstartrename?.(t);
    else if (e.key === 'Enter' || e.key === ' ') {
      if (t.kind === 'item') onselect(t.id);
      else collapsed[g.id] = !collapsed[g.id];
    } else return;
    e.preventDefault();
  }

  function drop(e: DragEvent, g: SidebarGroup, before?: string) {
    e.preventDefault();
    const id = dragId;
    dragId = null;
    if (!id || id === before) return;
    const rest = g.items.filter((i) => i.id !== id);
    onmove(id, g.id, before ? rest.findIndex((i) => i.id === before) : rest.length);
  }

  // Deferred: a context menu closing right after the command returns focus to its trigger.
  function focusOnMount(node: HTMLInputElement) {
    const timer = setTimeout(() => {
      node.focus();
      node.select();
    });
    return { destroy: () => clearTimeout(timer) };
  }
</script>

{#snippet rename(t: RowTarget, current: string)}
  <input
    use:focusOnMount
    aria-label="Name"
    class="min-w-0 flex-1 rounded-token border border-accent bg-subtle px-1"
    value={current}
    onkeydown={(e) => {
      e.stopPropagation();
      if (e.key === 'Enter') onrename(t, e.currentTarget.value);
      else if (e.key === 'Escape') oncancelrename();
    }}
    onblur={(e) =>
      renaming && key(renaming) === key(t) ? onrename(t, e.currentTarget.value) : null}
  />
{/snippet}

{#snippet wrap(t: RowTarget, content: Snippet)}
  {#if menu}
    {@const m = menu(t)}
    <ContextMenu ids={m.ids} context={m.context}>{@render content()}</ContextMenu>
  {:else}
    {@render content()}
  {/if}
{/snippet}

<div class="flex h-full min-h-0 flex-col">
  <input
    type="search"
    aria-label="Search sessions"
    placeholder="Search name, host, path"
    class="m-2 rounded-token border border-line bg-subtle px-2 py-1"
    value={search}
    oninput={(e) => onsearch(e.currentTarget.value)}
  />
  <div bind:this={tree} role="tree" aria-label={label} class="min-h-0 flex-1 overflow-y-auto">
    {#each groups as g, gi (g.id)}
      {@const gt = { kind: 'group', id: g.id } as const}
      <div role="none">
        {#snippet header()}
          <div
            role="treeitem"
            aria-selected="false"
            aria-expanded={open(g)}
            aria-level={1}
            tabindex={gi === 0 ? 0 : -1}
            data-row={key(gt)}
            class="flex cursor-default items-center gap-1 px-2 py-1 font-medium hover:bg-hover focus-visible:outline-2 focus-visible:outline-accent"
            ondragover={(e) => dragId && e.preventDefault()}
            ondrop={(e) => drop(e, g)}
            onclick={() => (collapsed[g.id] = !collapsed[g.id])}
            onkeydown={(e) => onkeydown(e, gt, g, gi)}
          >
            <ChevronDownIcon size={14} aria-hidden="true" class={open(g) ? '' : '-rotate-90'} />
            {#if renaming && key(renaming) === key(gt)}
              {@render rename(gt, g.label)}
            {:else}
              <span class="flex-1 truncate">{g.label}</span>
            {/if}
          </div>
        {/snippet}
        {@render wrap(gt, header)}
        {#if open(g)}
          <div role="group">
            {#each g.items as item (item.id)}
              {@const it = { kind: 'item', id: item.id } as const}
              {#snippet row()}
                <div
                  role="treeitem"
                  aria-selected={activeId === item.id}
                  aria-level={2}
                  tabindex={-1}
                  data-row={key(it)}
                  draggable={renaming === null}
                  class="flex cursor-default items-center gap-2 py-1 pr-2 pl-6 hover:bg-hover focus-visible:outline-2 focus-visible:outline-accent
                    {activeId === item.id ? 'bg-hover' : ''} {dragId === item.id
                    ? 'opacity-50'
                    : ''}"
                  ondragstart={(e) => {
                    dragId = item.id;
                    e.dataTransfer?.setData('text/plain', item.id);
                  }}
                  ondragend={() => (dragId = null)}
                  ondragover={(e) => dragId && e.preventDefault()}
                  ondrop={(e) => drop(e, g, item.id)}
                  onclick={() => onselect(item.id)}
                  onkeydown={(e) => onkeydown(e, it, g, gi)}
                >
                  {#if item.color}
                    <span
                      aria-hidden="true"
                      class="size-2 shrink-0 rounded-full"
                      style="background: var(--row-{item.color})"
                    ></span>
                  {/if}
                  {#if renaming && key(renaming) === key(it)}
                    {@render rename(it, item.label)}
                  {:else}
                    <span class="flex-1 truncate">{item.label}</span>
                    {#if item.detail}<span class="truncate text-xs text-muted">{item.detail}</span
                      >{/if}
                  {/if}
                </div>
              {/snippet}
              {@render wrap(it, row)}
            {/each}
          </div>
        {/if}
      </div>
    {:else}
      <p class="px-3 py-2 text-muted">
        {searching ? 'No matching sessions.' : 'Nothing here yet.'}
      </p>
    {/each}
  </div>
  {#if footer}<div class="flex gap-2 border-t border-line p-2">{@render footer()}</div>{/if}
</div>
