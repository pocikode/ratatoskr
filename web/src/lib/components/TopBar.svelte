<script lang="ts">
  import { DropdownMenu } from 'bits-ui';
  import { registry } from '$lib/commands';
  import { theme } from '$lib/stores/theme.svelte';
  import { iconFor } from '$lib/utils/icons';
  import logo from '../../assets/logo.svg';

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
