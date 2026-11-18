import { SvelteSet } from 'svelte/reactivity';

export interface SelectModifier {
  /** Ctrl/Cmd click: add to or remove from the selection. */
  toggle?: boolean;
  /** Shift click: select the range from the anchor to the clicked key. */
  range?: boolean;
}

/**
 * Row selection shared by the table and the page's bulk commands. Keys are row keys; range
 * selection follows the order the table currently displays (filtered and sorted).
 */
export class Selection {
  keys = $state<ReadonlySet<string>>(new SvelteSet());
  anchor = $state<string | null>(null);

  get size(): number {
    return this.keys.size;
  }

  has(key: string): boolean {
    return this.keys.has(key);
  }

  click(
    key: string,
    order: readonly string[],
    { toggle = false, range = false }: SelectModifier = {},
  ) {
    if (range && this.anchor !== null) {
      const from = order.indexOf(this.anchor);
      const to = order.indexOf(key);
      if (from >= 0 && to >= 0) {
        this.keys = new SvelteSet(order.slice(Math.min(from, to), Math.max(from, to) + 1));
        return;
      }
    }
    if (toggle) {
      const next = new SvelteSet(this.keys);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      this.keys = next;
    } else {
      this.keys = new SvelteSet([key]);
    }
    this.anchor = key;
  }

  selectAll(order: readonly string[]) {
    this.keys = new SvelteSet(order);
    this.anchor = order[0] ?? null;
  }

  clear() {
    this.keys = new SvelteSet();
    this.anchor = null;
  }
}
