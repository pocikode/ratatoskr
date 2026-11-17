import { describe, expect, it, vi } from 'vitest';
import { CommandRegistry } from './registry';
import { ShortcutManager } from './shortcuts';

function setup() {
  const reg = new CommandRegistry();
  const run = vi.fn();
  reg.register({ id: 'palette.open', title: 'p', run }, { id: 'nav.goto.replay', title: 'r', run });
  return { run, sm: new ShortcutManager(reg) };
}

const key = (k: string, init: KeyboardEventInit = {}, target?: EventTarget) => {
  const e = new KeyboardEvent('keydown', { key: k, cancelable: true, bubbles: true, ...init });
  if (target) Object.defineProperty(e, 'target', { value: target });
  return e;
};

describe('ShortcutManager', () => {
  it('opens the palette on Ctrl+K and ⌘K', () => {
    const { run, sm } = setup();
    sm.handle(key('k', { ctrlKey: true }));
    sm.handle(key('k', { metaKey: true }));
    expect(run).toHaveBeenCalledTimes(2);
  });

  it('supports the g then letter chord', () => {
    const { run, sm } = setup();
    sm.handle(key('g'));
    sm.handle(key('r'));
    expect(run).toHaveBeenCalledTimes(1);
  });

  it('ignores the chord while typing in an input', () => {
    const { run, sm } = setup();
    const input = document.createElement('input');
    sm.handle(key('g', {}, input));
    sm.handle(key('r', {}, input));
    expect(run).not.toHaveBeenCalled();
  });
});
