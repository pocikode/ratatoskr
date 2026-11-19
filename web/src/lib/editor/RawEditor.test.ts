import { cleanup, render, screen, waitFor } from '@testing-library/svelte';
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

  it('Shift+Enter inserts CRLF', async () => {
    const onchange = vi.fn();
    render(RawEditor, { value: enc('ab'), onchange });
    await userEvent.click(box());
    await userEvent.keyboard('{Control>}{Home}{/Control}{ArrowRight}{Shift>}{Enter}{/Shift}');
    expect(dec(onchange.mock.lastCall![0])).toBe('a\r\nb');
  });

  // Each break is one caret step whatever its bytes; keys act on the second line start/first line end.
  describe.each([
    ['CRLF', '\r\n'],
    ['LF', '\n'],
    ['CR', '\r'],
  ])('%s break', (_, sep) => {
    const text = `a${sep}b`;
    const home = '{Control>}{Home}{/Control}';
    const second = `${home}{ArrowRight}{ArrowRight}`; // jsdom has no layout: avoid ArrowDown
    const run = async (keys: string, value = text) => {
      const onchange = vi.fn();
      render(RawEditor, { value: enc(value), onchange });
      await userEvent.click(box());
      await userEvent.keyboard(keys);
      return onchange.mock.lastCall ? dec(onchange.mock.lastCall[0]) : null;
    };

    it('End then typing and Shift+End then typing', async () => {
      expect(await run(`${home}{End}x`)).toBe(`ax${sep}b`);
      cleanup();
      expect(await run(`${home}{ArrowRight}{End}x`)).toBe(`ax${sep}b`);
      cleanup();
      expect(await run(`${home}{ArrowRight}{Shift>}{End}{/Shift}x`)).toBe(`ax${sep}b`);
    });

    it('Backspace, Shift+Backspace and Delete remove the break only', async () => {
      expect(await run(`${second}{Backspace}`)).toBe('ab');
      cleanup();
      expect(await run(`${second}{Shift>}{Backspace}{/Shift}`)).toBe('ab');
      cleanup();
      expect(await run(`${home}{End}{Delete}`)).toBe('ab');
    });

    it('Mod+Backspace keeps the other break bytes', async () => {
      expect(await run(`${second}{Control>}{Backspace}{/Control}`, `${text}${sep}c`)).toBe(
        `ab${sep}c`,
      );
    });

    it('Alt+Up/Down never emit a lone LF or reorder bytes with a bare separator', async () => {
      const out = await run(`${home}{Alt>}{ArrowDown}{/Alt}`);
      expect(out === null || /^b\r\na$/.test(out)).toBe(true);
    });

    it('undo after deleting the break restores the exact bytes', async () => {
      const onchange = vi.fn();
      render(RawEditor, { value: enc(text), onchange });
      await userEvent.click(box());
      await userEvent.keyboard(`${second}{Backspace}{Control>}z{/Control}`);
      expect(dec(onchange.mock.lastCall![0])).toBe(text);
    });
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

  it('undo after loading byte-identical bytes under a new identity emits nothing', async () => {
    const onchange = vi.fn();
    const { rerender } = render(RawEditor, { value: enc('a'), onchange, identity: 1 });
    await userEvent.click(box());
    await userEvent.keyboard('{Control>}{End}{/Control}x');
    await rerender({ value: enc('ax'), onchange, identity: 2 });
    await tick();
    onchange.mockClear();
    await userEvent.keyboard('{Control>}z{/Control}');
    expect(onchange).not.toHaveBeenCalled();
  });

  it('a parent echo with the same identity keeps undo', async () => {
    const onchange = vi.fn();
    const { rerender } = render(RawEditor, { value: enc('a'), onchange, identity: 1 });
    await userEvent.click(box());
    await userEvent.keyboard('{Control>}{End}{/Control}x');
    await rerender({ value: enc('ax'), onchange, identity: 1 });
    await tick();
    await userEvent.keyboard('{Control>}z{/Control}');
    expect(dec(onchange.mock.lastCall![0])).toBe('a');
  });

  it('Mod+Enter reaches window unprevented and emits nothing', async () => {
    const onchange = vi.fn();
    const seen: boolean[] = [];
    const listener = (e: KeyboardEvent) => seen.push(e.defaultPrevented);
    window.addEventListener('keydown', listener);
    render(RawEditor, { value: enc('a'), onchange });
    await userEvent.click(box());
    await userEvent.keyboard('{Control>}{Enter}{/Control}');
    window.removeEventListener('keydown', listener);
    expect(seen).toContain(false);
    expect(seen).not.toContain(true);
    expect(onchange).not.toHaveBeenCalled();
  });
});
