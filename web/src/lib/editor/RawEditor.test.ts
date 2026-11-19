import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import RawEditor from './RawEditor.svelte';
import { variableTitle } from './http';
import { tick } from 'svelte';

// jsdom has no layout; CodeMirror measures text with Range#getClientRects.
Range.prototype.getClientRects = () => [] as unknown as DOMRectList;

const enc = (s: string) => new TextEncoder().encode(s);
const dec = (b: Uint8Array) => new TextDecoder().decode(b);
const box = () => screen.getByRole('textbox', { name: 'Raw HTTP message' });

describe('RawEditor', () => {
  it('Enter emits CRLF and keeps existing CRLF untouched', async () => {
    const onchange = vi.fn();
    render(RawEditor, { value: enc('GET / HTTP/1.1\r\n'), onchange });
    await userEvent.click(box());
    await userEvent.keyboard('{Control>}{End}{/Control}{Enter}');
    expect(dec(onchange.mock.lastCall![0])).toBe('GET / HTTP/1.1\r\n\r\n');
  });

  it('typing emits the new bytes', async () => {
    const onchange = vi.fn();
    render(RawEditor, { value: enc('ab'), onchange });
    await userEvent.click(box());
    await userEvent.keyboard('{Control>}{End}{/Control}c');
    await waitFor(() => expect(onchange).toHaveBeenCalled());
    expect(dec(onchange.mock.lastCall![0])).toBe('abc');
  });

  it('readonly blocks edits', async () => {
    const onchange = vi.fn();
    render(RawEditor, { value: enc('ab'), onchange, readonly: true });
    await userEvent.click(box());
    await userEvent.keyboard('c{Enter}');
    expect(onchange).not.toHaveBeenCalled();
  });

  it('invalid UTF-8 is read-only with a notice and emits nothing', async () => {
    const onchange = vi.fn();
    render(RawEditor, { value: new Uint8Array([0x47, 0xff]), onchange });
    expect(screen.getByRole('status')).toHaveTextContent('Not valid UTF-8');
    await userEvent.click(box());
    await userEvent.keyboard('c{Enter}');
    expect(onchange).not.toHaveBeenCalled();
  });

  it('decorates {{name}} and {{ name }}, known and unknown differently', () => {
    const { container } = render(RawEditor, {
      value: enc('GET /{{a}}/{{ b }}/{{c}} HTTP/1.1'),
      variables: { a: { value: '1', secret: false }, b: { value: 'x', secret: true } },
    });
    expect(container.querySelectorAll('.cm-var-known')).toHaveLength(2);
    const unknown = container.querySelectorAll('.cm-var-unknown');
    expect(unknown).toHaveLength(1);
    expect(unknown[0]).toHaveAttribute('title', 'c: not defined');
  });

  it('hover text shows the value or the mask', () => {
    const vars = { a: { value: '1', secret: false }, b: { value: 'x', secret: true } };
    expect(variableTitle('a', vars)).toBe('a = 1');
    expect(variableTitle('b', vars)).toBe('b = ••••');
  });

  it('Backspace and Shift+Enter treat CRLF as one unit', async () => {
    const onchange = vi.fn();
    render(RawEditor, { value: enc('a\r\nb'), onchange });
    await userEvent.click(box());
    await userEvent.keyboard('{Control>}{Home}{/Control}{ArrowDown}{Home}{Backspace}');
    expect(dec(onchange.mock.lastCall![0])).toBe('ab');
    await userEvent.keyboard('{Shift>}{Enter}{/Shift}');
    expect(dec(onchange.mock.lastCall![0])).toBe('a\r\nb');
  });

  it('editing one character keeps lone CR, LF and CRLF elsewhere', async () => {
    const onchange = vi.fn();
    render(RawEditor, { value: enc('a\rb\nc\r\nd'), onchange });
    await userEvent.click(box());
    await userEvent.keyboard('{Control>}{End}{/Control}e');
    expect(dec(onchange.mock.lastCall![0])).toBe('a\rb\nc\r\nde');
  });

  it('undo after the value prop changes cannot bring back the old message', async () => {
    const onchange = vi.fn();
    const { rerender } = render(RawEditor, { value: enc('one'), onchange });
    await userEvent.click(box());
    await userEvent.keyboard('{Control>}{End}{/Control}x');
    onchange.mockClear();
    await rerender({ value: enc('two'), onchange });
    await tick();
    await userEvent.keyboard('{Control>}z{/Control}');
    expect(onchange).not.toHaveBeenCalled();
  });
});
