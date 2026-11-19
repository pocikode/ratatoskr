import { render, screen, waitFor, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import {
  CreateReplayCollectionDocument,
  CreateReplaySessionDocument,
} from '$lib/api/generated/graphql';
import { router } from '$lib/router/router.svelte';
import { projects, type Project } from '$lib/stores/project.svelte';
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

  it('ignores only the outgoing project route after a project switch', async () => {
    const r = page('p1');
    await userEvent.click(await screen.findByText('POST /login'));
    await screen.findByRole('tab', { name: 'POST /login' });
    const loginHash = window.location.hash;

    // p2 has no tabs: the outgoing session must not open and its hash must not linger.
    await r.rerender({ projectId: 'p2' });
    await within(await tree()).findByText('POST /login');
    await waitFor(() => expect(window.location.hash).toBe('#/replay'));
    await waitFor(() => expect(router.location.params).toEqual([]));
    expect(screen.queryByRole('tab')).toBeNull();

    // A later link to the same id in p2 is a real deep link.
    window.location.hash = loginHash;
    expect(await screen.findByRole('tab', { name: 'POST /login' })).toHaveAttribute(
      'aria-selected',
      'true',
    );
  });

  it('keeps a startup deep link until the project list has loaded', async () => {
    const prev = { list: projects.projects, loading: projects.loading };
    projects.loading = true;
    window.location.hash = '#/replay/session/rs3';
    render(ReplayPage, { client: createMockClient({ count: 5, liveIntervalMs: 0 }) });
    await new Promise((r) => setTimeout(r, 20));
    expect(screen.queryByRole('tree')).toBeNull();
    projects.projects = [{ id: 'p9', isSelected: true } as Project];
    projects.loading = false;
    try {
      expect(await screen.findByRole('tab', { name: 'GET /api/v1/users' })).toHaveAttribute(
        'aria-selected',
        'true',
      );
    } finally {
      projects.projects = prev.list;
      projects.loading = prev.loading;
    }
  });

  it('Escape cancels a tab rename without saving', async () => {
    page();
    await userEvent.click(await screen.findByText('POST /login'));
    await userEvent.dblClick(await screen.findByRole('tab', { name: 'POST /login' }));
    await userEvent.keyboard('{Control>}a{/Control}nope{Escape}');
    expect(await screen.findByRole('tab', { name: 'POST /login' })).toBeInTheDocument();
    expect(screen.queryByRole('tab', { name: 'nope' })).toBeNull();
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

  it('creates one session and one collection on repeated submits', async () => {
    const client = createMockClient({ count: 5, liveIntervalMs: 0 });
    const real = client.mutation.bind(client);
    let release = () => {};
    const gate = new Promise<void>((r) => (release = r));
    const created: string[] = [];
    client.mutation = ((query, vars, ctx) => {
      if (query === CreateReplaySessionDocument) created.push('CreateReplaySession');
      if (query === CreateReplayCollectionDocument) created.push('CreateReplayCollection');
      return {
        toPromise: async () => {
          await gate;
          return real(query, vars, ctx).toPromise();
        },
      };
    }) as typeof client.mutation;
    render(ReplayPage, { client, projectId: 'p1' });
    await tree();

    await userEvent.click(screen.getByRole('button', { name: '+ Session' }));
    await userEvent.type(screen.getByLabelText('Host'), 'shop.test');
    const create = screen.getByRole('button', { name: 'Create' });
    create.closest('form')!.requestSubmit();
    create.closest('form')!.requestSubmit();
    await waitFor(() => expect(create).toBeDisabled());
    expect(created).toEqual(['CreateReplaySession']);
    release();
    await screen.findByRole('tab', { name: 'GET /' });

    await userEvent.click(screen.getByRole('button', { name: '+ Collection' }));
    await userEvent.type(await screen.findByLabelText('Name'), 'Once');
    const form = screen.getByRole('button', { name: 'Create' }).closest('form')!;
    form.requestSubmit();
    form.requestSubmit();
    await within(await tree()).findByText('Once');
    expect(created).toEqual(['CreateReplaySession', 'CreateReplayCollection']);
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

describe('ReplayPage actions', () => {
  it('renames a closed session through its sidebar menu', async () => {
    page();
    const t = await tree();
    await userEvent.pointer({
      keys: '[MouseRight]',
      target: await within(t).findByText('POST /login'),
    });
    await userEvent.click(await screen.findByRole('menuitem', { name: 'Rename session' }));
    await userEvent.keyboard('{Control>}a{/Control}renamed{Enter}');
    expect(await within(t).findByText('renamed')).toBeInTheDocument();
    expect(screen.queryByRole('tab')).toBeNull();
  });

  it('creates, renames and deletes a collection', async () => {
    page();
    const t = await tree();
    await userEvent.click(screen.getByRole('button', { name: '+ Collection' }));
    await userEvent.type(await screen.findByLabelText('Name'), 'Scratch');
    await userEvent.click(screen.getByRole('button', { name: 'Create' }));
    expect(await within(t).findByText('Scratch')).toBeInTheDocument();

    await userEvent.pointer({ keys: '[MouseRight]', target: within(t).getByText('Scratch') });
    await userEvent.click(await screen.findByRole('menuitem', { name: 'Rename collection' }));
    await userEvent.keyboard('{Control>}a{/Control}Scratch2{Enter}');
    expect(await within(t).findByText('Scratch2')).toBeInTheDocument();

    await userEvent.pointer({ keys: '[MouseRight]', target: within(t).getByText('Scratch2') });
    await userEvent.click(await screen.findByRole('menuitem', { name: 'Delete collection' }));
    await userEvent.click(await screen.findByRole('button', { name: 'Delete' }));
    await waitFor(() => expect(within(t).queryByText('Scratch2')).toBeNull());
  });

  it('tags a session with a colour from the tab menu', async () => {
    page();
    await userEvent.click(await screen.findByText('POST /login'));
    const tab = await screen.findByRole('tab', { name: 'POST /login' });
    await userEvent.pointer({ keys: '[MouseRight]', target: tab });
    await userEvent.click(await screen.findByRole('menuitem', { name: /Colour: Red/ }));
    await waitFor(() =>
      expect(
        screen
          .getByRole('tab', { name: 'POST /login' })
          .parentElement!.querySelector('[style*="--row-red"]'),
      ).not.toBeNull(),
    );
  });

  it('moves a session between collections from the keyboard', async () => {
    page();
    const t = await tree();
    const row = (await within(t).findByText('POST /login')).closest<HTMLElement>(
      '[role=treeitem]',
    )!;
    const group = (name: string) =>
      within(t).getByText(name).closest('[role=none]')!.querySelector('[role=group]')!;
    const before = within(group('Auth flows') as HTMLElement).queryByText('POST /login') !== null;
    row.focus();
    await userEvent.keyboard(before ? '{Alt>}{PageUp}{/Alt}' : '{Alt>}{PageDown}{/Alt}');
    await waitFor(() =>
      expect(
        within(group(before ? 'Default' : 'Auth flows') as HTMLElement).queryByText('POST /login'),
      ).not.toBeNull(),
    );
  });

  it('keeps focus on the row moved into a collapsed collection', async () => {
    page();
    const t = await tree();
    const row = (await within(t).findByText('POST /login')).closest<HTMLElement>(
      '[role=treeitem]',
    )!;
    const header = (name: string) =>
      within(t).getByText(name).closest<HTMLElement>('[role=treeitem]')!;
    const inAuth =
      header('Auth flows').closest('[role=none]')!.querySelector('[role=group]')?.contains(row) ??
      false;
    const dest = header(inAuth ? 'Default' : 'Auth flows');
    await userEvent.click(dest);
    expect(dest).toHaveAttribute('aria-expanded', 'false');
    row.focus();
    await userEvent.keyboard(inAuth ? '{Alt>}{PageUp}{/Alt}' : '{Alt>}{PageDown}{/Alt}');
    await waitFor(() => {
      expect(dest).toHaveAttribute('aria-expanded', 'true');
      const focused = document.activeElement as HTMLElement;
      expect(focused).toHaveAttribute('role', 'treeitem');
      expect(focused).toHaveTextContent('POST /login');
      expect(dest.closest('[role=none]')!.querySelector('[role=group]')!.contains(focused)).toBe(
        true,
      );
    });
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
