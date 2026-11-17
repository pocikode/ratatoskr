import { describe, expect, it, vi } from 'vitest';
import { CommandRegistry, filterCommands } from './registry';

describe('CommandRegistry', () => {
  it('runs registered commands and honours when()', async () => {
    const reg = new CommandRegistry();
    const run = vi.fn();
    reg.register({ id: 'a', title: 'A', run }, { id: 'b', title: 'B', when: () => false, run });
    expect(await reg.run('a')).toBe(true);
    expect(await reg.run('b')).toBe(false);
    expect(await reg.run('missing')).toBe(false);
    expect(run).toHaveBeenCalledTimes(1);
    expect(reg.available().map((c) => c.id)).toEqual(['a']);
  });

  it('unregisters', () => {
    const reg = new CommandRegistry();
    const off = reg.register({ id: 'a', title: 'A', run: () => {} });
    off();
    expect(reg.get('a')).toBeUndefined();
  });

  it('filters by all words, ranking word-prefix matches first', () => {
    const cmds = ['Go to Replay', 'Go to Workflows', 'Toggle sidebar'].map((title) => ({
      id: title,
      title,
      run: () => {},
    }));
    expect(filterCommands(cmds, 'repl').map((c) => c.title)).toEqual(['Go to Replay']);
    expect(filterCommands(cmds, 'go flow').map((c) => c.title)).toEqual(['Go to Workflows']);
    expect(filterCommands(cmds, 'zzz')).toEqual([]);
    expect(filterCommands(cmds, '')).toHaveLength(3);
  });
});
