import { render, screen, waitFor, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { router } from '$lib/router/router.svelte';
import { createMockClient } from '$lib/api/mocks/exchange';
import ReplayPage from './ReplayPage.svelte';
import { moveSession, type Collection } from './replay.svelte';

let stop = () => {};
beforeEach(() => {
  localStorage.clear();
  window.location.hash = '#/replay';
  stop = router.start();
});
afterEach(() => stop());

const page = (projectId = 'p1') =>
  render(ReplayPage, { client: createMockClient({ count: 5, liveIntervalMs: 0 }), projectId });
const tree = () => screen.findByRole('tree', { name: 'Sessions' });

describe('ReplayPage', () => {
  it('lists collections and sessions and filters by name, host or path', async () => {
    page();
    const t = await tree();
    expect(await within(t).findByText('GET /api/v1/users')).toBeInTheDocument();
    expect(within(t).getByText('Auth flows')).toBeInTheDocument();

    await userEvent.type(screen.getByLabelText('Search sessions'), 'auth.example');
    expect(within(t).getByText('POST /login')).toBeInTheDocument();
    expect(within(t).queryByText('GET /api/v1/users')).toBeNull();
  });

  it('opens a tab on click, renames on double-click and closes it', async () => {
    page();
    await userEvent.click(await screen.findByText('POST /login'));
    const tab = await screen.findByRole('tab', { name: 'POST /login' });
    expect(tab).toHaveAttribute('aria-selected', 'true');
    expect(window.location.hash).toMatch(/^#\/replay\/session\//);

    await userEvent.dblClick(tab);
    await userEvent.keyboard('{Control>}a{/Control}login 2{Enter}');
    expect(await screen.findByRole('tab', { name: 'login 2' })).toBeInTheDocument();

    await userEvent.click(screen.getByRole('button', { name: 'Close login 2' }));
    expect(screen.queryByRole('tab')).toBeNull();
  });

  it('opens the session of a deep link', async () => {
    window.location.hash = '#/replay/session/rs3';
    page();
    expect(await screen.findByRole('tab', { name: 'GET /api/v1/users' })).toHaveAttribute(
      'aria-selected',
      'true',
    );
  });

  it('restores open tabs for the same project only', async () => {
    const first = page('p1');
    await userEvent.click(await screen.findByText('POST /login'));
    await screen.findByRole('tab', { name: 'POST /login' });
    first.unmount();

    page('p2');
    await tree();
    expect(screen.queryByRole('tab')).toBeNull();
    document.body.innerHTML = '';

    page('p1');
    expect(await screen.findByRole('tab', { name: 'POST /login' })).toBeInTheDocument();
  });

  it('builds the request from the URL dialog', async () => {
    page();
    await tree();
    await userEvent.click(screen.getByRole('button', { name: '+ Session' }));
    await userEvent.type(screen.getByLabelText('Host'), 'shop.test');
    await userEvent.clear(screen.getByLabelText('Path'));
    await userEvent.type(screen.getByLabelText('Path'), '/cart');
    await userEvent.click(screen.getByRole('button', { name: 'Create' }));
    expect(await screen.findByRole('tab', { name: 'GET /cart' })).toBeInTheDocument();
  });

  it('duplicates and deletes (with confirm) a session', async () => {
    page();
    const t = await tree();
    await userEvent.click(await within(t).findByText('POST /login'));
    await userEvent.click(await screen.findByRole('tab', { name: 'POST /login' }));
    await userEvent.keyboard('{Control>}k{/Control}');
    // Registry commands also reach the palette.
    await userEvent.keyboard('{Escape}');

    await userEvent.pointer({ keys: '[MouseRight]', target: within(t).getByText('POST /login') });
    await userEvent.click(await screen.findByRole('menuitem', { name: 'Duplicate session' }));
    expect(await within(t).findByText('POST /login (copy)')).toBeInTheDocument();

    await userEvent.pointer({
      keys: '[MouseRight]',
      target: within(t).getByText('POST /login (copy)'),
    });
    await userEvent.click(await screen.findByRole('menuitem', { name: 'Delete session' }));
    await userEvent.click(await screen.findByRole('button', { name: 'Delete' }));
    await waitFor(() => expect(within(t).queryByText('POST /login (copy)')).toBeNull());
  });

  it('hides delete for the Default collection', async () => {
    page();
    const t = await tree();
    await userEvent.pointer({
      keys: '[MouseRight]',
      target: await within(t).findByText('Default'),
    });
    expect(await screen.findByRole('menuitem', { name: 'Rename collection' })).toBeInTheDocument();
    expect(screen.queryByRole('menuitem', { name: 'Delete collection' })).toBeNull();
  });
});

describe('moveSession', () => {
  const s = (id: string) => ({ id }) as Collection['sessions'][number];
  const cols = [
    { id: 'a', name: 'A', isDefault: true, sessions: [s('1'), s('2')] },
    { id: 'b', name: 'B', isDefault: false, sessions: [s('3')] },
  ];

  it('moves across collections and reorders within one', () => {
    const ids = (c: Collection[]) => c.map((x) => x.sessions.map((y) => y.id).join(''));
    expect(ids(moveSession(cols, '1', 'b', 0))).toEqual(['2', '13']);
    expect(ids(moveSession(cols, '1', 'a', 1))).toEqual(['21', '3']);
    expect(ids(moveSession(cols, '3', 'a', 99))).toEqual(['123', '']);
  });
});
