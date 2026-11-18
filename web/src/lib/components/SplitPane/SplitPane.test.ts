import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeAll, describe, expect, it, vi } from 'vitest';
import { createRawSnippet } from 'svelte';
import SplitPane from './SplitPane.svelte';

// paneforge measures panes with ResizeObserver, which jsdom does not implement.
beforeAll(() => {
  globalThis.ResizeObserver ??= class {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver;
});

const pane = (text: string) => createRawSnippet(() => ({ render: () => `<div>${text}</div>` }));

describe('SplitPane', () => {
  it('renders both panes with an accessible divider', () => {
    render(SplitPane, { props: { first: pane('request'), second: pane('response') } });
    expect(screen.getByText('request')).toBeVisible();
    expect(screen.getByText('response')).toBeVisible();
    expect(screen.getByRole('separator')).toBeVisible();
  });

  it('reports layout changes', async () => {
    const user = userEvent.setup();
    const onsizeschange = vi.fn();
    render(SplitPane, {
      props: { first: pane('a'), second: pane('b'), onsizeschange },
    });

    // paneforge reports the mount layout too, so only the keyboard step should count.
    onsizeschange.mockClear();
    const divider = screen.getByRole('separator');
    divider.focus();
    await user.keyboard('{ArrowRight}');

    const [first] = onsizeschange.mock.lastCall?.[0] ?? [];
    expect(first).toBeGreaterThan(55);
  });
});
