import { describe, expect, it, vi } from 'vitest';
import { CommandRegistry } from './registry';
import { defaultBindings, ShortcutManager, type ShortcutOptions } from './shortcuts';

const ids = [
  'palette.open',
  'sidebar.toggle',
  'nav.goto.http-history',
  'nav.goto.replay',
  'replay.send',
  'intercept.forward',
  'querybar.focus',
];

function setup(opts: ShortcutOptions = {}) {
  const reg = new CommandRegistry();
  const calls: { id: string; page?: string }[] = [];
  reg.register(
    ...ids.map((id) => ({
      id,
      title: id,
      run: (ctx: { page?: string }) => void calls.push({ id, page: ctx.page }),
    })),
  );
  return { calls, reg, sm: new ShortcutManager(reg, opts) };
}

const key = (k: string, init: KeyboardEventInit = {}, target?: EventTarget) => {
  const e = new KeyboardEvent('keydown', { key: k, cancelable: true, bubbles: true, ...init });
  if (target) Object.defineProperty(e, 'target', { value: target });
  return e;
};

const input = () => document.createElement('input');

describe('ShortcutManager', () => {
  it('opens the palette on Ctrl+K and ⌘K', () => {
    const { calls, sm } = setup();
    sm.handle(key('k', { ctrlKey: true }));
    sm.handle(key('k', { metaKey: true }));
    expect(calls.map((c) => c.id)).toEqual(['palette.open', 'palette.open']);
  });

  it('runs a global binding from a text input', () => {
    const { calls, sm } = setup();
    sm.handle(key('k', { metaKey: true }, input()));
    expect(calls).toHaveLength(1);
  });

  it('ignores non-global bindings while typing in a text input', () => {
    const { calls, sm } = setup();
    sm.handle(key('b', { metaKey: true }, input()));
    sm.handle(key('/', {}, input()));
    expect(calls).toEqual([]);
  });

  it('supports the g then letter chord', () => {
    const { calls, sm } = setup();
    sm.handle(key('g'));
    sm.handle(key('h'));
    expect(calls.map((c) => c.id)).toEqual(['nav.goto.http-history']);
  });

  it('ignores the chord while typing in an input', () => {
    const { calls, sm } = setup();
    sm.handle(key('g', {}, input()));
    sm.handle(key('r', {}, input()));
    expect(calls).toEqual([]);
  });

  it('prefers the page-scoped binding for a shared key', () => {
    const replay = setup({ page: () => 'replay' });
    replay.sm.handle(key('Enter', { metaKey: true }));
    expect(replay.calls).toEqual([{ id: 'replay.send', page: 'replay' }]);

    const intercept = setup({ page: () => 'intercept' });
    intercept.sm.handle(key('Enter', { metaKey: true }));
    expect(intercept.calls).toEqual([{ id: 'intercept.forward', page: 'intercept' }]);
  });

  it('does nothing for a page-scoped key on another page', () => {
    const { calls, sm } = setup({ page: () => 'sitemap' });
    sm.handle(key('Enter', { metaKey: true }));
    expect(calls).toEqual([]);
  });

  it('covers the docs/02 §4 default table', () => {
    const first = (command: string) => defaultBindings.find((b) => b.command === command);
    expect(first('palette.open')).toMatchObject({ key: 'k', mod: true, global: true });
    expect(first('sidebar.toggle')).toMatchObject({ key: 'b', mod: true });
    expect(first('history.sendToReplay')).toMatchObject({ key: 'r', mod: true });
    expect(first('history.sendToAutomate')).toMatchObject({ key: 'm', mod: true });
    expect(first('intercept.toggle')).toMatchObject({ key: 'i', mod: true });
    expect(first('intercept.drop')).toMatchObject({ key: 'backspace', mod: true });
    expect(first('tab.next')).toMatchObject({ key: ']', mod: true });
    expect(first('tab.prev')).toMatchObject({ key: '[', mod: true });
    expect(first('tab.close')).toMatchObject({ key: 'w', mod: true });
    expect(first('querybar.focus')).toMatchObject({ key: '/' });
    expect(first('editor.find')).toMatchObject({ key: 'f', mod: true });
    expect(first('selection.delete')).toMatchObject({ key: 'delete' });
  });

  it('rebinds a command and persists a copy', () => {
    const persist = vi.fn();
    const { calls, sm } = setup({ persist });
    expect(sm.rebind('palette.open', 'p', { mod: true })).toBeNull();
    expect(persist).toHaveBeenCalledOnce();
    expect(persist.mock.calls[0]?.[0]).not.toBe(sm.bindings);
    sm.handle(key('p', { metaKey: true }));
    expect(calls.map((c) => c.id)).toEqual(['palette.open']);
    sm.handle(key('k', { metaKey: true }));
    expect(calls).toHaveLength(1);
  });

  it('refuses a rebind that would conflict and names the other command', () => {
    const persist = vi.fn();
    const { sm } = setup({ persist });
    expect(sm.rebind('sidebar.toggle', 'k', { mod: true })).toBe('palette.open');
    expect(sm.bindings.find((b) => b.command === 'sidebar.toggle')).toMatchObject({
      key: 'b',
      mod: true,
    });
    expect(persist).not.toHaveBeenCalled();
  });

  it('keeps the page scope when rebinding and allows a shared key across pages', () => {
    const { sm } = setup();
    expect(sm.rebind('replay.send', 'enter', { mod: true })).toBeNull();
    expect(sm.bindings.find((b) => b.command === 'intercept.forward')).toMatchObject({
      key: 'enter',
      mod: true,
      page: 'intercept',
    });
  });

  it('resets to the defaults', () => {
    const { sm } = setup();
    sm.rebind('palette.open', 'p', { mod: true });
    sm.reset();
    expect(sm.bindings).toEqual(defaultBindings);
  });
});
