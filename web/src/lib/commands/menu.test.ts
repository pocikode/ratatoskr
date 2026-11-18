import { describe, expect, it, vi } from 'vitest';
import { commandMenu } from './menu';
import { CommandRegistry, type CommandContext } from './registry';

function registry() {
  const reg = new CommandRegistry();
  reg.register(
    { id: 'row.sendToReplay', title: 'Send to Replay', group: 'Selection', run: vi.fn() },
    { id: 'row.copyUrl', title: 'Copy URL', group: 'Selection', run: vi.fn() },
    {
      id: 'row.delete',
      title: 'Delete',
      run: vi.fn(),
      when: (ctx: CommandContext) => !!ctx.project,
    },
  );
  return reg;
}

describe('commandMenu', () => {
  it('keeps the requested order and drops unknown ids', () => {
    const menu = commandMenu(registry(), ['row.copyUrl', 'nope', 'row.sendToReplay']);
    expect(menu.map((c) => c.id)).toEqual(['row.copyUrl', 'row.sendToReplay']);
  });

  it('applies the command when() predicate to the menu context', () => {
    const reg = registry();
    expect(commandMenu(reg, ['row.delete'])).toEqual([]);
    expect(
      commandMenu(reg, ['row.delete'], { project: {} as CommandContext['project'] }).map(
        (c) => c.id,
      ),
    ).toEqual(['row.delete']);
  });
});
