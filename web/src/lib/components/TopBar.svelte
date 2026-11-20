<script lang="ts">
  import { DropdownMenu } from 'bits-ui';
  import { registry } from '$lib/commands';
  import { intercept, type InterceptStore } from '$lib/stores/intercept.svelte';
  import { theme } from '$lib/stores/theme.svelte';
  import ProjectSwitcher from './ProjectSwitcher.svelte';
  import { iconFor } from '$lib/utils/icons';
  import logo from '../../assets/logo.svg';

  let { store = intercept }: { store?: InterceptStore } = $props();
  $effect(() => store.start());
  const queuing = $derived(store.settings?.queuing ?? false);

  const isMac = /Mac|iPhone|iPad/.test(navigator.platform);
  const Search = iconFor('search')!;
  const User = iconFor('user')!;
  const menu = ['nav.goto.settings', 'theme.dark', 'theme.light'];
  const entries = $derived(
    menu.map((id) => registry.get(id)).filter((c): c is NonNullable<typeof c> => !!c),
  );
</script>

<header
  class="flex shrink-0 items-center gap-3 border-b border-line bg-elevated px-3"
  style:height="var(--topbar-h)"
>
  <a href="#/sitemap" class="flex items-center gap-2 font-semibold" aria-label="Ratatoskr home">
    <img src={logo} alt="" class="size-5" />
    Ratatoskr
  </a>

  <ProjectSwitcher />

  <button
    type="button"
    class="ml-4 flex w-72 items-center gap-2 rounded-token border border-line bg-subtle px-2 py-1 text-muted hover:bg-hover"
    onclick={() => registry.run('palette.open')}
  >
    <Search size={14} aria-hidden="true" />
    <span class="flex-1 text-left">Search commands…</span>
    <kbd class="font-mono text-xs">{isMac ? '⌘K' : 'Ctrl K'}</kbd>
  </button>

  <div class="flex-1"></div>

  <div class="flex items-center">
    <button
      type="button"
      class="rounded-l-token border border-line px-2 py-1 font-medium hover:bg-hover
        {queuing ? 'bg-warn/20 text-warn' : 'bg-subtle text-muted'}"
      aria-pressed={queuing}
      disabled={!store.settings}
      aria-label="Intercept: {queuing ? 'Queuing' : 'Forwarding'}"
      title="Toggle intercept ({isMac ? '⌘I' : 'Ctrl I'})"
      onclick={() => registry.run('intercept.toggle')}
    >
      {queuing ? 'Queuing ‖' : 'Forwarding ▶'}
    </button>
    <DropdownMenu.Root>
      <DropdownMenu.Trigger
        class="rounded-r-token border border-l-0 border-line px-1.5 py-1 hover:bg-hover"
        aria-label="Intercept options"
        disabled={!store.settings}
      >
        <span aria-hidden="true">▾</span>
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content
          align="end"
          sideOffset={4}
          class="z-50 min-w-40 rounded-token border border-line bg-elevated p-1 shadow-lg"
        >
          <DropdownMenu.CheckboxItem
            checked={store.settings?.requests ?? false}
            class="flex cursor-default items-center justify-between gap-3 rounded-token px-2 py-1 outline-none data-highlighted:bg-hover"
            onSelect={() => registry.run('intercept.toggleRequests')}
          >
            Requests
            {#if store.settings?.requests}<span aria-hidden="true">✓</span>{/if}
          </DropdownMenu.CheckboxItem>
          <DropdownMenu.CheckboxItem
            checked={store.settings?.responses ?? false}
            class="flex cursor-default items-center justify-between gap-3 rounded-token px-2 py-1 outline-none data-highlighted:bg-hover"
            onSelect={() => registry.run('intercept.toggleResponses')}
          >
            Responses
            {#if store.settings?.responses}<span aria-hidden="true">✓</span>{/if}
          </DropdownMenu.CheckboxItem>
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  </div>

  <DropdownMenu.Root>
    <DropdownMenu.Trigger
      class="flex items-center rounded-token p-1.5 hover:bg-hover"
      aria-label="User menu"
    >
      <User size={16} aria-hidden="true" />
    </DropdownMenu.Trigger>
    <DropdownMenu.Portal>
      <DropdownMenu.Content
        align="end"
        sideOffset={4}
        class="z-50 min-w-40 rounded-token border border-line bg-elevated p-1 shadow-lg"
      >
        {#each entries as cmd (cmd.id)}
          <DropdownMenu.Item
            class="flex cursor-default items-center justify-between rounded-token px-2 py-1 outline-none data-highlighted:bg-hover"
            onSelect={() => registry.run(cmd.id)}
          >
            {cmd.title.replace(/^Go to /, '')}
            {#if cmd.id === `theme.${theme.current}`}<span aria-label="current">✓</span>{/if}
          </DropdownMenu.Item>
        {/each}
      </DropdownMenu.Content>
    </DropdownMenu.Portal>
  </DropdownMenu.Root>
</header>
