import type { Component } from 'svelte';
import type { IconName } from '../utils/icons';

export interface PageRoute {
  /** First hash segment: `#/<id>[/<param>]`. */
  id: string;
  title: string;
  /** Sidebar category; routes without one (settings) are reachable but not listed. */
  category?: string;
  /** Lucide icon file name under @lucide/svelte/icons. */
  icon: IconName;
  /** Single-letter `g <letter>` go-to shortcut, where the spec defines one. */
  goto?: string;
  load: () => Promise<{ default: Component }>;
}

export const DEFAULT_ROUTE = 'sitemap';

export const CATEGORIES = ['Overview', 'Proxy', 'Testing', 'Logging', 'Workspace'] as const;

export const routes: PageRoute[] = [
  {
    id: 'sitemap',
    title: 'Sitemap',
    category: 'Overview',
    icon: 'network',
    load: () => import('../../pages/sitemap/SitemapPage.svelte'),
  },
  {
    id: 'scopes',
    title: 'Scopes',
    category: 'Overview',
    icon: 'crosshair',
    load: () => import('../../pages/scopes/ScopesPage.svelte'),
  },
  {
    id: 'filters',
    title: 'Filters',
    category: 'Overview',
    icon: 'funnel',
    load: () => import('../../pages/filters/FiltersPage.svelte'),
  },
  {
    id: 'intercept',
    title: 'Intercept',
    category: 'Proxy',
    icon: 'circle-pause',
    goto: 'i',
    load: () => import('../../pages/intercept/InterceptPage.svelte'),
  },
  {
    id: 'http-history',
    title: 'HTTP History',
    category: 'Proxy',
    icon: 'clock',
    goto: 'h',
    load: () => import('../../pages/http-history/HttpHistoryPage.svelte'),
  },
  {
    id: 'ws-history',
    title: 'WS History',
    category: 'Proxy',
    icon: 'radio',
    load: () => import('../../pages/ws-history/WsHistoryPage.svelte'),
  },
  {
    id: 'match-replace',
    title: 'Match & Replace',
    category: 'Proxy',
    icon: 'replace',
    load: () => import('../../pages/match-replace/MatchReplacePage.svelte'),
  },
  {
    id: 'replay',
    title: 'Replay',
    category: 'Testing',
    icon: 'repeat',
    goto: 'r',
    load: () => import('../../pages/replay/ReplayPage.svelte'),
  },
  {
    id: 'automate',
    title: 'Automate',
    category: 'Testing',
    icon: 'zap',
    goto: 'a',
    load: () => import('../../pages/automate/AutomatePage.svelte'),
  },
  {
    id: 'workflows',
    title: 'Workflows',
    category: 'Testing',
    icon: 'workflow',
    goto: 'w',
    load: () => import('../../pages/workflows/WorkflowsPage.svelte'),
  },
  {
    id: 'assistant',
    title: 'Assistant',
    category: 'Testing',
    icon: 'sparkles',
    load: () => import('../../pages/assistant/AssistantPage.svelte'),
  },
  {
    id: 'search',
    title: 'Search',
    category: 'Logging',
    icon: 'search',
    goto: 's',
    load: () => import('../../pages/search/SearchPage.svelte'),
  },
  {
    id: 'findings',
    title: 'Findings',
    category: 'Logging',
    icon: 'flag',
    goto: 'f',
    load: () => import('../../pages/findings/FindingsPage.svelte'),
  },
  {
    id: 'exports',
    title: 'Exports',
    category: 'Logging',
    icon: 'download',
    load: () => import('../../pages/exports/ExportsPage.svelte'),
  },
  {
    id: 'environment',
    title: 'Environment',
    category: 'Workspace',
    icon: 'variable',
    load: () => import('../../pages/environment/EnvironmentPage.svelte'),
  },
  {
    id: 'files',
    title: 'Files',
    category: 'Workspace',
    icon: 'folder',
    load: () => import('../../pages/files/FilesPage.svelte'),
  },
  {
    id: 'plugins',
    title: 'Plugins',
    category: 'Workspace',
    icon: 'puzzle',
    load: () => import('../../pages/plugins/PluginsPage.svelte'),
  },
  {
    id: 'workspace',
    title: 'Workspace',
    category: 'Workspace',
    icon: 'layout-dashboard',
    load: () => import('../../pages/workspace/WorkspacePage.svelte'),
  },
  {
    id: 'settings',
    title: 'Settings',
    icon: 'settings',
    load: () => import('../../pages/settings/SettingsPage.svelte'),
  },
];

export function findRoute(id: string): PageRoute | undefined {
  return routes.find((r) => r.id === id);
}
