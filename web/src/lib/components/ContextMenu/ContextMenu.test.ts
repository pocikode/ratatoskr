import { fireEvent, render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { createRawSnippet } from 'svelte';
import { describe, expect, it, vi } from 'vitest';
import { CommandRegistry } from '$lib/commands/registry';
import ContextMenu from './ContextMenu.svelte';

const children = createRawSnippet(() => ({ render: () => '<span>row 42</span>' }));

function setup() {
  const reg = new CommandRegistry();
  const run = vi.fn();
  reg.register(
    { id: 'row.sendToReplay', title: 'Send to Replay', group: 'Selection', run },
    { id: 'row.copyUrl', title: 'Copy URL', group: 'Selection', run },
    { id: 'row.delete', title: 'Delete', run, when: () => false },
  );
  render(ContextMenu, {
    props: {
      ids: ['row.sendToReplay', 'row.copyUrl', 'row.delete'],
      context: { page: 'http-history' },
      registry: reg,
      children,
    },
  });
  return { reg, run };
}

describe('ContextMenu', () => {
  it('builds its items from the command registry and runs the picked one with the context', async () => {
    const { run } = setup();
    await userEvent.pointer({ keys: '[MouseRight]', target: screen.getByText('row 42') });
    const item = await screen.findByRole('menuitem', { name: 'Copy URL' });
    expect(screen.getAllByRole('menuitem')).toHaveLength(2);
    await fireEvent.click(item);
    expect(run).toHaveBeenCalledOnce();
  });

  it('hides commands whose when() rejects the trigger context', async () => {
    setup();
    await userEvent.pointer({ keys: '[MouseRight]', target: screen.getByText('row 42') });
    await screen.findByRole('menuitem', { name: 'Send to Replay' });
    expect(screen.queryByText('Delete')).toBeNull();
  });
});
