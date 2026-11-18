import { render, screen, waitFor, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import type { Component } from 'svelte';
import App from './App.svelte';
import { CATEGORIES, routes } from '$lib/router/routes';
import { ui } from '$lib/stores/ui.svelte';

beforeEach(() => {
  window.location.hash = '';
});
afterEach(() => {
  ui.paletteOpen = false;
  ui.sidebarCollapsed = false;
});

const sidebarRoutes = routes.filter((r) => r.category);

describe('app shell', () => {
  it('lists every category and page in the sidebar', () => {
    render(App);
    const nav = screen.getByRole('navigation', { name: 'Main' });
    for (const c of CATEGORIES) expect(within(nav).getByRole('heading', { name: c })).toBeVisible();
    for (const r of sidebarRoutes)
      expect(within(nav).getByRole('link', { name: r.title })).toBeVisible();
  });

  it.each(sidebarRoutes.map((r) => [r.title, r.id] as const))(
    'navigates to %s placeholder',
    async (title, id) => {
      const user = userEvent.setup();
      render(App);
      await user.click(screen.getByRole('link', { name: title }));
      expect(window.location.hash).toBe(`#/${id}`);
      expect(await screen.findByRole('heading', { level: 1, name: title })).toBeVisible();
      expect(screen.getByRole('link', { name: title })).toHaveAttribute('aria-current', 'page');
    },
  );

  it('does not keep the previous page mounted while the next chunk loads', async () => {
    const target = routes.find((r) => r.id === 'findings')!;
    const original = target.load;
    let release!: () => void;
    const gate = new Promise<void>((r) => (release = r));
    target.load = async () => {
      await gate;
      return original();
    };
    try {
      window.location.hash = '#/replay';
      render(App);
      expect(await screen.findByRole('heading', { level: 1, name: 'Replay' })).toBeVisible();
      window.location.hash = '#/findings';
      await waitFor(() =>
        expect(screen.getByRole('link', { name: 'Findings' })).toHaveAttribute(
          'aria-current',
          'page',
        ),
      );
      expect(screen.queryByRole('heading', { level: 1, name: 'Replay' })).toBeNull();
      release();
      expect(await screen.findByRole('heading', { level: 1, name: 'Findings' })).toBeVisible();
    } finally {
      target.load = original as () => Promise<{ default: Component }>;
    }
  });

  it('restores the route from the hash on load', async () => {
    window.location.hash = '#/findings';
    render(App);
    expect(await screen.findByRole('heading', { level: 1, name: 'Findings' })).toBeVisible();
  });

  it('shows not-found for unknown routes', async () => {
    window.location.hash = '#/nope';
    render(App);
    expect(await screen.findByRole('heading', { name: 'Page not found' })).toBeVisible();
  });

  it('opens the palette with Ctrl+K and navigates by typing "repl" + Enter', async () => {
    const user = userEvent.setup();
    render(App);
    await user.keyboard('{Control>}k{/Control}');
    const box = await screen.findByRole('combobox', { name: 'Search commands' });
    expect(box).toHaveFocus();
    await user.type(box, 'repl');
    expect(screen.getByRole('option', { name: /Go to Replay/ })).toBeVisible();
    await user.keyboard('{Enter}');
    expect(window.location.hash).toBe('#/replay');
    expect(await screen.findByRole('heading', { level: 1, name: 'Replay' })).toBeVisible();
    await waitFor(() =>
      expect(screen.queryByRole('combobox', { name: 'Search commands' })).not.toBeInTheDocument(),
    );
  });

  it('switches theme from the user menu', async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    render(App);
    await user.click(screen.getByRole('button', { name: 'User menu' }));
    // bits-ui positions the menu with layout APIs jsdom lacks, so role queries see it as inaccessible.
    await user.click(await screen.findByText('Theme: Light'));
    expect(document.documentElement.dataset.theme).toBe('light');
    await user.click(screen.getByRole('button', { name: 'User menu' }));
    await user.click(await screen.findByText('Theme: Dark'));
    expect(document.documentElement.dataset.theme).toBe('dark');
  });

  it('collapses the sidebar to an icon rail', async () => {
    const user = userEvent.setup();
    render(App);
    await user.click(screen.getByRole('button', { name: 'Collapse sidebar' }));
    expect(screen.queryByRole('heading', { name: 'Proxy' })).not.toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Replay' })).toBeVisible();
  });
});
