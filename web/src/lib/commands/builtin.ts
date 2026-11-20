import { routes } from '../router/routes';
import type { Router } from '../router/router.svelte';
import { intercept as defaultIntercept, type InterceptStore } from '../stores/intercept.svelte';
import type { ThemeState } from '../stores/theme.svelte';
import type { UiState } from '../stores/ui.svelte';
import { CommandRegistry } from './registry';

interface Deps {
  router: Pick<Router, 'navigate'>;
  ui: Pick<UiState, 'toggleSidebar' | 'openPalette'>;
  theme: Pick<ThemeState, 'set' | 'toggle'>;
  intercept?: Pick<InterceptStore, 'toggleQueuing' | 'flip'>;
}

/** Registers the shell's own commands: go-to-page, palette, sidebar, theme and the intercept toggles. */
export function registerBuiltins(
  registry: CommandRegistry,
  { router, ui, theme, intercept = defaultIntercept }: Deps,
) {
  registry.register(
    {
      id: 'intercept.toggle',
      title: 'Toggle intercept (Forwarding / Queuing)',
      icon: 'circle-pause',
      group: 'Intercept',
      run: () => void intercept.toggleQueuing(),
    },
    {
      id: 'intercept.toggleRequests',
      title: 'Intercept: hold requests',
      group: 'Intercept',
      run: () => void intercept.flip('requests'),
    },
    {
      id: 'intercept.toggleResponses',
      title: 'Intercept: hold responses',
      group: 'Intercept',
      run: () => void intercept.flip('responses'),
    },
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
