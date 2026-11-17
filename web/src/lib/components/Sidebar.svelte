<script lang="ts">
  import { CATEGORIES, routes } from '$lib/router/routes';
  import { router } from '$lib/router/router.svelte';
  import { ui } from '$lib/stores/ui.svelte';
  import { iconFor } from '$lib/utils/icons';
  import { registry } from '$lib/commands';
</script>

<nav
  aria-label="Main"
  class="flex shrink-0 flex-col overflow-y-auto border-r border-line bg-elevated"
  style:width={ui.sidebarCollapsed ? 'var(--sidebar-w-rail)' : 'var(--sidebar-w)'}
>
  <div class="flex-1 py-2">
    {#each CATEGORIES as category (category)}
      <div class="mb-2">
        {#if !ui.sidebarCollapsed}
          <h2 class="px-3 pt-2 pb-1 text-xs font-semibold tracking-wider text-muted uppercase">
            {category}
          </h2>
        {/if}
        <ul>
          {#each routes.filter((r) => r.category === category) as r (r.id)}
            {@const Icon = iconFor(r.icon)}
            {@const active = router.location.page === r.id}
            <li>
              <a
                href={`#/${r.id}`}
                aria-current={active ? 'page' : undefined}
                aria-label={ui.sidebarCollapsed ? r.title : undefined}
                title={ui.sidebarCollapsed ? r.title : undefined}
                class="mx-1 flex items-center gap-2 rounded-token px-2 py-1 hover:bg-hover
                  {active ? 'bg-accent-soft text-accent' : ''}"
              >
                {#if Icon}<Icon size={16} aria-hidden="true" />{/if}
                {#if !ui.sidebarCollapsed}<span class="truncate">{r.title}</span>{/if}
              </a>
            </li>
          {/each}
        </ul>
      </div>
    {/each}
  </div>
  <button
    type="button"
    class="flex items-center gap-2 border-t border-line px-3 py-2 text-muted hover:bg-hover"
    aria-label={ui.sidebarCollapsed ? 'Expand sidebar' : 'Collapse sidebar'}
    onclick={() => registry.run('sidebar.toggle')}
  >
    {#if ui.sidebarCollapsed}
      {@const Icon = iconFor('chevrons-right')}
      {#if Icon}<Icon size={16} aria-hidden="true" />{/if}
    {:else}
      {@const Icon = iconFor('chevrons-left')}
      {#if Icon}<Icon size={16} aria-hidden="true" />{/if}
      <span>Collapse</span>
    {/if}
  </button>
</nav>
