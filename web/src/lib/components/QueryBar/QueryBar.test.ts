import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import QueryBar from './QueryBar.svelte';

describe('QueryBar', () => {
  it('shows the query text in a labelled editor', () => {
    render(QueryBar, {
      value: 'req.host.eq:"api"',
      label: 'HTTPQL query',
      placeholder: 'type a query',
    });

    expect(screen.getByRole('textbox', { name: 'HTTPQL query' })).toHaveTextContent(
      'req.host.eq:"api"',
    );
  });

  it('submits the current text on Enter', async () => {
    const onsubmit = vi.fn();
    render(QueryBar, { value: 'req.host.eq:"api"', label: 'HTTPQL query', onsubmit });

    await userEvent.click(screen.getByRole('textbox', { name: 'HTTPQL query' }));
    await userEvent.keyboard('{Enter}');

    expect(onsubmit).toHaveBeenCalledWith('req.host.eq:"api"');
  });

  it('walks the applied queries with ↑/↓', async () => {
    const onsubmit = vi.fn();
    render(QueryBar, {
      value: '',
      label: 'HTTPQL query',
      placeholder: 'type a query',
      history: ['req.host.cont:"api"', 'resp.code.gte:400'],
      onsubmit,
    });
    const editor = screen.getByRole('textbox', { name: 'HTTPQL query' });
    await userEvent.click(editor);

    await userEvent.keyboard('{ArrowUp}');
    expect(editor).toHaveTextContent('resp.code.gte:400');
    await userEvent.keyboard('{ArrowUp}');
    expect(editor).toHaveTextContent('req.host.cont:"api"');
    await userEvent.keyboard('{ArrowDown}{ArrowDown}');
    expect(editor).not.toHaveTextContent('req.host.cont:"api"');

    await userEvent.keyboard('{Enter}');
    expect(onsubmit.mock.calls.at(-1)?.[0]).toBe('');
  });

  it('follows an external value change', async () => {
    const { rerender } = render(QueryBar, {
      value: 'req.a',
      label: 'HTTPQL query',
      placeholder: 'type a query',
    });
    const editor = screen.getByRole('textbox', { name: 'HTTPQL query' });
    expect(editor).toHaveTextContent('req.a');

    await rerender({
      value: 'resp.code.eq:500',
      label: 'HTTPQL query',
      placeholder: 'type a query',
    });

    expect(editor).toHaveTextContent('resp.code.eq:500');
  });

  it('renders query errors inline and marks the editor invalid', () => {
    render(QueryBar, {
      value: 'req.host.eq:"api',
      label: 'HTTPQL query',
      placeholder: 'type a query',
      error: { message: 'Unterminated string', from: 12, to: 16 },
    });

    expect(screen.getByRole('alert')).toHaveTextContent('Unterminated string');
    expect(screen.getByRole('textbox', { name: 'HTTPQL query' })).toHaveAttribute(
      'aria-invalid',
      'true',
    );
  });

  it('highlights namespaces, fields and operators', () => {
    render(QueryBar, {
      value: 'req.host.eq:"api"',
      label: 'HTTPQL query',
      placeholder: 'type a query',
    });

    expect(document.querySelector('.cm-httpql-namespace')).toHaveTextContent('req');
    expect(document.querySelector('.cm-httpql-field')).toHaveTextContent('host');
    expect(document.querySelector('.cm-httpql-operator')).toHaveTextContent('eq');
    expect(document.querySelector('.cm-httpql-string')).toHaveTextContent('"api"');
  });

  it('flattens pasted newlines into spaces', async () => {
    const user = userEvent.setup();
    const onsubmit = vi.fn();
    render(QueryBar, { value: '', label: 'HTTPQL query', onsubmit });
    await user.click(screen.getByRole('textbox'));
    await user.paste('a\nb');
    await user.keyboard('{Enter}');
    expect(onsubmit).toHaveBeenCalledWith('a b');
  });
});
