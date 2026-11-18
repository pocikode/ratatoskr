import { render, screen } from '@testing-library/svelte';
import { beforeAll, expect, it } from 'vitest';
import { router } from '$lib/router/router.svelte';
import DevComponentsPage from './DevComponentsPage.svelte';

// paneforge measures panes with ResizeObserver, which jsdom does not implement.
beforeAll(() => {
  globalThis.ResizeObserver ??= class {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver;
});

it('renders the component gallery behind #/dev/components', () => {
  router.location = { page: 'dev', params: ['components'] };
  render(DevComponentsPage);

  expect(screen.getByRole('heading', { name: 'Component gallery' })).toBeVisible();
  expect(screen.getByRole('grid', { name: 'Demo traffic' })).toBeVisible();
  expect(screen.getByRole('region', { name: 'Request' })).toBeVisible();
  expect(screen.getByRole('region', { name: 'Response' })).toBeVisible();
  expect(screen.getByText(/100000 rows/)).toBeVisible();
});
