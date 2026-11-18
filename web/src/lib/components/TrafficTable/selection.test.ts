import { describe, expect, it } from 'vitest';
import { Selection } from './selection.svelte';

const order = ['a', 'b', 'c', 'd', 'e'];

describe('Selection', () => {
  it('replaces the selection on a plain click and remembers the anchor', () => {
    const selection = new Selection();
    selection.click('a', order);
    selection.click('c', order);
    expect([...selection.keys]).toEqual(['c']);
    expect(selection.anchor).toBe('c');
  });

  it('toggles rows with ctrl/cmd', () => {
    const selection = new Selection();
    selection.click('a', order);
    selection.click('c', order, { toggle: true });
    expect([...selection.keys].sort()).toEqual(['a', 'c']);
    selection.click('a', order, { toggle: true });
    expect([...selection.keys]).toEqual(['c']);
  });

  it('selects the range from the anchor with shift, in either direction', () => {
    const selection = new Selection();
    selection.click('b', order);
    selection.click('d', order, { range: true });
    expect([...selection.keys]).toEqual(['b', 'c', 'd']);
    selection.click('a', order, { range: true });
    expect([...selection.keys]).toEqual(['a', 'b']);
  });

  it('falls back to a plain click when the anchor is not in the current order', () => {
    const selection = new Selection();
    selection.click('z', order);
    selection.click('c', order, { range: true });
    expect([...selection.keys]).toEqual(['c']);
  });

  it('selects all and clears', () => {
    const selection = new Selection();
    selection.selectAll(order);
    expect(selection.size).toBe(order.length);
    selection.clear();
    expect(selection.size).toBe(0);
    expect(selection.anchor).toBeNull();
  });
});
