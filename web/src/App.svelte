<script lang="ts">
  import { registry } from '$lib/commands';
  import { ShortcutManager } from '$lib/commands/shortcuts';
  import { router } from '$lib/router/router.svelte';
  import CommandPalette from '$lib/components/CommandPalette.svelte';
  import RouterView from '$lib/components/RouterView.svelte';
  import Sidebar from '$lib/components/Sidebar.svelte';
  import StatusBar from '$lib/components/StatusBar.svelte';
  import TopBar from '$lib/components/TopBar.svelte';

  const shortcuts = new ShortcutManager(registry);

  $effect(() => router.start());
  $effect(() => {
    window.addEventListener('keydown', shortcuts.handle);
    return () => window.removeEventListener('keydown', shortcuts.handle);
  });
</script>

<a
  href="#main"
  class="sr-only focus:not-sr-only focus:absolute focus:z-50 focus:bg-elevated focus:p-2"
  onclick={(e) => {
    e.preventDefault();
    document.getElementById('main')?.focus();
  }}>Skip to content</a
>
<div class="flex h-full flex-col">
  <TopBar />
  <div class="flex min-h-0 flex-1">
    <Sidebar />
    <div class="min-w-0 flex-1"><RouterView /></div>
  </div>
  <StatusBar />
</div>
<CommandPalette />
