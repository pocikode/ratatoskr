import { routes } from '../router/routes';
import type { Router } from '../router/router.svelte';
import type { ThemeState } from '../stores/theme.svelte';
import type { UiState } from '../stores/ui.svelte';
import { CommandRegistry } from './registry';

interface Deps {
  router: Pick<Router, 'navigate'>;
  ui: Pick<UiState, 'toggleSidebar' | 'openPalette'>;
  theme: Pick<ThemeState, 'set' | 'toggle'>;
}

/** Registers the shell's own commands: go-to-page, palette, sidebar and theme. */
export function registerBuiltins(registry: CommandRegistry, { router, ui, theme }: Deps) {
  registry.register(
    ...routes.map((r) => ({
      id: `nav.goto.${r.id}`,
      title: `Go to ${r.title}`,
      icon: r.icon,
      group: 'Navigation',
      run: () => router.navigate(r.id),
    })),
    {
      id: 'palette.open',
      title: 'Open command palette',
      icon: 'command',
      run: () => ui.openPalette(),
    },
    {
      id: 'sidebar.toggle',
      title: 'Toggle sidebar',
      icon: 'panel-left',
      group: 'View',
      run: () => ui.toggleSidebar(),
    },
    {
      id: 'theme.dark',
      title: 'Theme: Dark',
      icon: 'moon',
      group: 'View',
      run: () => theme.set('dark'),
    },
    {
      id: 'theme.light',
      title: 'Theme: Light',
      icon: 'sun',
      group: 'View',
      run: () => theme.set('light'),
    },
    {
      id: 'theme.toggle',
      title: 'Toggle theme',
      icon: 'palette',
      group: 'View',
      run: () => theme.toggle(),
    },
  );
}
