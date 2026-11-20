import { fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { CombinedError } from '@urql/core';
import { createMockClient } from '$lib/api/mocks/exchange';
import { ExportMatchReplaceDocument } from '$lib/api/generated/graphql';
import { registry } from '$lib/commands';
import MatchReplacePage from './MatchReplacePage.svelte';

beforeEach(() => localStorage.clear());

// jsdom has no layout; CodeMirror (the condition QueryBar) measures text through Range rects.
Range.prototype.getClientRects ??= () => [] as unknown as DOMRectList;
Range.prototype.getBoundingClientRect ??= () => new DOMRect();
afterEach(() => vi.restoreAllMocks());

const page = (client = createMockClient({ count: 5, liveIntervalMs: 0 })) => {
  render(MatchReplacePage, { client, projectId: 'p' });
  return client;
};
const rule = (name: string) => screen.findByRole('button', { name });
const editor = () => screen.getByRole('form', { name: 'Rule editor' });
const select = (label: string) => within(editor()).getByLabelText(label) as HTMLSelectElement;
const optionValues = (el: HTMLSelectElement) => [...el.options].map((o) => o.value);

describe('MatchReplacePage', () => {
  it('lists collections with their rules and a tester placeholder', async () => {
    page();

    expect(await rule('Add debug header')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Force 200' })).toBeInTheDocument();
    expect(screen.getByRole('checkbox', { name: 'Enable Force 200' })).not.toBeChecked();
    expect(screen.getByRole('region', { name: 'Tester' })).toBeInTheDocument();
  });

  it('narrows operations and matchers, and hides the matcher for Add', async () => {
    page();
    await userEvent.click(await rule('Mock user agent'));

    expect(optionValues(select('Operation'))).toEqual([
      'UPDATE_RAW',
      'UPDATE_VALUE',
      'ADD',
      'REMOVE',
    ]);
    expect(within(editor()).getByRole('radio', { name: 'Name' })).toBeChecked();

    await userEvent.selectOptions(select('Section'), 'REQUEST_METHOD');
    expect(optionValues(select('Operation'))).toEqual(['UPDATE']);
    expect(within(editor()).queryByRole('radio', { name: 'Name' })).not.toBeInTheDocument();

    await userEvent.selectOptions(select('Section'), 'REQUEST_HEADER');
    await userEvent.selectOptions(select('Operation'), 'ADD');
    expect(within(editor()).queryByRole('radio', { name: 'Full' })).not.toBeInTheDocument();
  });

  it('shows the RE2 hint for Regex', async () => {
    page();
    await userEvent.click(await rule('Force 200'));

    expect(within(editor()).getByText(/RE2 syntax/)).toBeInTheDocument();
  });

  it('creates a rule and shows BAD_INPUT on its field', async () => {
    page();
    await rule('Add debug header');
    await userEvent.click(screen.getByRole('button', { name: '+ New rule' }));

    await userEvent.click(within(editor()).getByRole('button', { name: 'Save' }));
    expect(await within(editor()).findByText(/name must be/)).toBeInTheDocument();

    await userEvent.type(within(editor()).getByLabelText('Name'), 'Strip CSP');
    await userEvent.type(within(editor()).getByLabelText('Term'), 'no colon');
    await userEvent.click(within(editor()).getByRole('button', { name: 'Save' }));
    expect(await within(editor()).findByText(/"Name: value"/)).toBeInTheDocument();

    await userEvent.clear(within(editor()).getByLabelText('Term'));
    await userEvent.type(within(editor()).getByLabelText('Term'), 'X-A: 1');
    await userEvent.click(within(editor()).getByRole('button', { name: 'Save' }));
    expect(await screen.findByRole('button', { name: 'Strip CSP' })).toBeInTheDocument();
  });

  it('puts a BAD_QUERY error on the condition', async () => {
    page();
    await userEvent.click(await rule('Add debug header'));
    const cond = within(editor()).getByRole('textbox', { name: 'Condition' });
    await userEvent.click(cond);
    await userEvent.keyboard('req.host.eq:"x');
    await userEvent.click(within(editor()).getByRole('button', { name: 'Save' }));

    expect(await within(editor()).findByRole('alert')).toHaveTextContent(/unterminated string/);
  });

  it('shows a BAD_INPUT error for the condition', async () => {
    const client = page();
    await userEvent.click(await rule('Add debug header'));
    const real = client.mutation.bind(client);
    vi.spyOn(client, 'mutation').mockImplementation(((doc: unknown, vars: unknown) =>
      JSON.stringify(doc).includes('updateMatchReplaceRule')
        ? {
            toPromise: async () => ({
              error: new CombinedError({
                graphQLErrors: [
                  { message: 'condition too long', extensions: { code: 'BAD_INPUT' } },
                ],
              }),
            }),
          }
        : real(doc as never, vars as never)) as never);
    await userEvent.click(within(editor()).getByRole('button', { name: 'Save' }));

    expect(await within(editor()).findByText('condition too long')).toBeInTheDocument();
  });

  it('creates once and keeps the rule when the refresh is slow and fails', async () => {
    const client = page();
    await rule('Add debug header');
    await userEvent.click(screen.getByRole('button', { name: '+ New rule' }));
    await userEvent.type(within(editor()).getByLabelText('Name'), 'Once');
    await userEvent.type(within(editor()).getByLabelText('Term'), 'X-A: 1');
    let release!: () => void;
    const gate = new Promise<void>((r) => (release = r));
    const realQuery = client.query.bind(client);
    vi.spyOn(client, 'query').mockImplementation(((doc: unknown, vars: unknown, o: unknown) =>
      JSON.stringify(doc).includes('matchReplaceCollections')
        ? { toPromise: async () => (await gate, { error: new Error('refresh failed') }) }
        : realQuery(doc as never, vars as never, o as never)) as never);
    const create = vi.spyOn(client, 'mutation');

    await userEvent.click(within(editor()).getByRole('button', { name: 'Save' }));
    await userEvent.click(within(editor()).getByRole('button', { name: 'Save' }));
    release();

    await screen.findByRole('alert');
    expect(
      create.mock.calls.filter(([d]) => JSON.stringify(d).includes('createMatchReplaceRule')),
    ).toHaveLength(1);
    expect(within(editor()).getByLabelText('Name')).toHaveValue('Once');
    expect(within(editor()).getByRole('button', { name: 'Delete' })).toBeEnabled();
  });

  it('keeps the new draft when a delete refresh finishes after a switch', async () => {
    const client = page();
    await userEvent.click(await rule('Add debug header'));
    let release!: () => void;
    const gate = new Promise<void>((r) => (release = r));
    const realQuery = client.query.bind(client);
    vi.spyOn(window, 'confirm').mockReturnValue(true);
    vi.spyOn(client, 'query').mockImplementation(((doc: unknown, vars: unknown, o: unknown) => {
      const r = realQuery(doc as never, vars as never, o as never);
      return JSON.stringify(doc).includes('matchReplaceCollections')
        ? { toPromise: async () => (await gate, r.toPromise()) }
        : r;
    }) as never);
    await userEvent.click(within(editor()).getByRole('button', { name: 'Delete' }));
    await userEvent.click(screen.getByRole('button', { name: 'Force 200' }));
    await userEvent.type(within(editor()).getByLabelText('Name'), '?');
    release();

    await waitFor(() =>
      expect(screen.queryByRole('button', { name: 'Add debug header' })).not.toBeInTheDocument(),
    );
    expect(within(editor()).getByLabelText('Name')).toHaveValue('Force 200?');
  });

  it('toggles enabled and deletes a rule', async () => {
    page();
    await rule('Force 200');
    await userEvent.click(screen.getByRole('checkbox', { name: 'Enable Force 200' }));
    await waitFor(() =>
      expect(screen.getByRole('checkbox', { name: 'Enable Force 200' })).toBeChecked(),
    );

    await userEvent.click(screen.getByRole('button', { name: 'Force 200' }));
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(true);
    await userEvent.click(within(editor()).getByRole('button', { name: 'Delete' }));
    expect(confirm).toHaveBeenCalledWith('Delete rule "Force 200"?');
    await waitFor(() =>
      expect(screen.queryByRole('button', { name: 'Force 200' })).not.toBeInTheDocument(),
    );
  });

  it('creates and deletes a collection', async () => {
    page();
    await rule('Add debug header');
    vi.spyOn(window, 'prompt').mockReturnValue('Auth');
    await userEvent.click(screen.getByRole('button', { name: '+ New collection' }));
    expect(await screen.findByRole('group', { name: 'Auth' })).toBeInTheDocument();

    vi.spyOn(window, 'confirm').mockReturnValue(true);
    await userEvent.click(screen.getByRole('button', { name: 'Delete Auth' }));
    await waitFor(() => expect(screen.queryByRole('group', { name: 'Auth' })).toBeNull());
  });

  it('drags a rule across collections and reorders', async () => {
    const client = page();
    await rule('Add debug header');
    const row = (name: string) => screen.getByRole('button', { name }).closest('li')!;

    const spy = vi.spyOn(client, 'mutation');
    await fireEvent.dragStart(row('Add debug header'));
    await fireEvent.drop(row('Force 200'));
    await waitFor(() => {
      const bodies = screen.getByRole('list', { name: 'Bodies rules' });
      expect(
        within(bodies)
          .getAllByRole('button', { name: /^(?!Move)/ })
          .map((b) => b.textContent?.trim()),
      ).toEqual(['Rewrite staging links', 'Add debug header', 'Force 200']);
    });
    const vars = spy.mock.calls.find(([d]) =>
      JSON.stringify(d).includes('moveMatchReplaceRule'),
    )![1] as {
      collectionId: string;
      index: number;
    };
    expect(vars.index).toBe(1);

    await userEvent.click(screen.getByRole('button', { name: 'Move Bodies up' }));
    const groups = screen.getAllByRole('group').map((g) => g.getAttribute('aria-label'));
    expect(groups).toEqual(['Bodies', 'Headers']);
  });

  it('reverts a failed move', async () => {
    const client = page();
    await rule('Add debug header');
    const spy = vi.spyOn(client, 'mutation');
    spy.mockImplementation(((doc: unknown, vars: unknown) => {
      const name = JSON.stringify(doc);
      if (name.includes('moveMatchReplaceRule'))
        return { toPromise: async () => ({ error: new Error('nope') }) };
      return (client.mutation as unknown as (...a: unknown[]) => unknown)(doc, vars);
    }) as never);
    const row = (name: string) => screen.getByRole('button', { name }).closest('li')!;
    await fireEvent.dragStart(row('Add debug header'));
    await fireEvent.drop(row('Force 200'));

    await waitFor(() => expect(screen.getByRole('alert')).toBeInTheDocument());
    const headers = screen.getByRole('list', { name: 'Headers rules' });
    expect(within(headers).getByRole('button', { name: 'Add debug header' })).toBeInTheDocument();
  });

  it('exports the rules and imports a file', async () => {
    const client = page();
    await rule('Add debug header');
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {});
    let blob: Blob | undefined;
    URL.createObjectURL = vi.fn((b: Blob) => ((blob = b), 'blob:x'));
    URL.revokeObjectURL = vi.fn();
    await userEvent.click(screen.getByRole('button', { name: 'Export' }));
    await waitFor(() => expect(click).toHaveBeenCalled());
    const exported = await blob!.text();
    const expected = (
      await client
        .query(ExportMatchReplaceDocument, {}, { requestPolicy: 'network-only' })
        .toPromise()
    ).data!.exportMatchReplace;
    expect(exported).toBe(expected);
    expect((click.mock.contexts[0] as HTMLAnchorElement).download).toBe('match-replace.json');

    const file = new File([exported], 'rules.json', { type: 'application/json' });
    await userEvent.upload(screen.getByLabelText('Import file'), file);
    expect(await screen.findByText('Imported 2 collections')).toBeInTheDocument();

    await userEvent.upload(
      screen.getByLabelText('Import file'),
      new File(['{"version":2}'], 'bad.json'),
    );
    expect(await screen.findByRole('alert')).toBeInTheDocument();
  });

  it('asks before leaving unsaved edits', async () => {
    page();
    await userEvent.click(await rule('Add debug header'));
    await userEvent.type(within(editor()).getByLabelText('Name'), '!');
    const confirm = vi.spyOn(window, 'confirm').mockReturnValueOnce(false).mockReturnValue(true);

    await userEvent.click(screen.getByRole('button', { name: 'Force 200' }));
    expect(confirm).toHaveBeenCalledTimes(1);
    expect(within(editor()).getByLabelText('Name')).toHaveValue('Add debug header!');

    await userEvent.click(screen.getByRole('button', { name: 'Force 200' }));
    expect(within(editor()).getByLabelText('Name')).toHaveValue('Force 200');
  });

  it('does not delete the open rule when the switch is declined from the context menu', async () => {
    page();
    await userEvent.click(await rule('Add debug header'));
    await userEvent.type(within(editor()).getByLabelText('Name'), '!');
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false);

    await userEvent.pointer({
      keys: '[MouseRight]',
      target: screen.getByRole('button', { name: 'Force 200' }),
    });
    await userEvent.click(
      await screen.findByRole('menuitem', { name: 'Delete Match & Replace rule' }),
    );

    expect(confirm).toHaveBeenCalledWith('Delete rule "Force 200"?');
    expect(screen.getByRole('button', { name: 'Add debug header' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Force 200' })).toBeInTheDocument();
  });

  it('keeps the saved values of an updated rule when the refresh fails', async () => {
    const client = page();
    await userEvent.click(await rule('Add debug header'));
    await userEvent.type(within(editor()).getByLabelText('Name'), '!');
    const realQuery = client.query.bind(client);
    vi.spyOn(client, 'query').mockImplementation(((doc: unknown, vars: unknown, o: unknown) =>
      JSON.stringify(doc).includes('matchReplaceCollections')
        ? { toPromise: async () => ({ error: new Error('refresh failed') }) }
        : realQuery(doc as never, vars as never, o as never)) as never);

    await userEvent.click(within(editor()).getByRole('button', { name: 'Save' }));

    await screen.findByRole('button', { name: 'Add debug header!' });
    expect(within(editor()).getByLabelText('Name')).toHaveValue('Add debug header!');
  });

  it('palette delete targets the open rule after a row menu was dismissed', async () => {
    page();
    await rule('Add debug header');
    await userEvent.pointer({
      keys: '[MouseRight]',
      target: screen.getByRole('button', { name: 'Force 200' }),
    });
    await screen.findByRole('menuitem', { name: 'Delete Match & Replace rule' });
    await userEvent.keyboard('{Escape}');
    await userEvent.click(screen.getByRole('button', { name: 'Add debug header' }));
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false);

    await registry.run('mr.rule.delete');

    expect(confirm).toHaveBeenCalledWith('Delete rule "Add debug header"?');
  });

  it('keeps edits of the next rule when a slow save finishes', async () => {
    const client = page();
    await userEvent.click(await rule('Add debug header'));
    await userEvent.type(within(editor()).getByLabelText('Name'), '!');
    let release!: () => void;
    const gate = new Promise<void>((r) => (release = r));
    const real = client.mutation.bind(client);
    vi.spyOn(client, 'mutation').mockImplementation(((doc: unknown, vars: unknown) => {
      const result = real(doc as never, vars as never);
      if (!JSON.stringify(doc).includes('updateMatchReplaceRule')) return result;
      return { toPromise: async () => (await gate, result.toPromise()) };
    }) as never);

    await userEvent.click(within(editor()).getByRole('button', { name: 'Save' }));
    vi.spyOn(window, 'confirm').mockReturnValue(true);
    await userEvent.click(screen.getByRole('button', { name: 'Force 200' }));
    await userEvent.type(within(editor()).getByLabelText('Name'), '?');
    release();

    await screen.findByRole('button', { name: 'Add debug header!' });
    expect(within(editor()).getByLabelText('Name')).toHaveValue('Force 200?');
  });

  it('rolls back only the failed toggle', async () => {
    const client = page();
    await rule('Add debug header');
    let fail!: () => void;
    const gate = new Promise<void>((r) => (fail = r));
    const real = client.mutation.bind(client);
    let calls = 0;
    vi.spyOn(client, 'mutation').mockImplementation(((doc: unknown, vars: unknown) => {
      if (!JSON.stringify(doc).includes('setMatchReplaceRuleEnabled'))
        return real(doc as never, vars as never);
      // The first toggle fails late; the second one succeeds meanwhile.
      return calls++ === 0
        ? { toPromise: async () => (await gate, { error: new Error('nope') }) }
        : real(doc as never, vars as never);
    }) as never);

    const box = (name: string) => screen.getByRole('checkbox', { name: `Enable ${name}` });
    await userEvent.click(box('Force 200'));
    await userEvent.click(box('Add debug header'));
    await waitFor(() => expect(box('Add debug header')).not.toBeChecked());
    fail();

    await screen.findByRole('alert');
    expect(box('Force 200')).not.toBeChecked();
    expect(box('Add debug header')).not.toBeChecked();
  });

  it('starts fresh when the project changes', async () => {
    const client = createMockClient({ count: 5, liveIntervalMs: 0 });
    const view = render(MatchReplacePage, { client, projectId: 'a' });
    await userEvent.click(await rule('Add debug header'));
    expect(within(editor()).getByLabelText('Name')).toHaveValue('Add debug header');

    await view.rerender({ client, projectId: 'b' });
    await rule('Add debug header');
    expect(screen.queryByRole('form', { name: 'Rule editor' })).not.toBeInTheDocument();
  });
});
