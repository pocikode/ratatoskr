import { routes } from '../router/routes';
import { router } from '../router/router.svelte';
import type { CommandRegistry } from './registry';

export interface Binding {
  /** Lowercase `KeyboardEvent.key` (`'k'`, `'enter'`, `'['`). */
  key: string;
  /** `mod` = Ctrl on Win/Linux, ⌘ on macOS. */
  mod?: boolean;
  /** Only active on this page id; one key can then serve several commands (docs/02 §4). */
  page?: string;
  /** Fires while typing in an input/editor too. Default: suppressed there. */
  global?: boolean;
  command: string;
}

/** docs/02 §4. */
export const defaultBindings: Binding[] = [
  { key: 'k', mod: true, global: true, command: 'palette.open' },
  { key: 'b', mod: true, command: 'sidebar.toggle' },
  { key: 'r', mod: true, command: 'history.sendToReplay' },
  { key: 'm', mod: true, command: 'history.sendToAutomate' },
  { key: 'enter', mod: true, page: 'replay', global: true, command: 'replay.send' },
  { key: 'enter', mod: true, page: 'intercept', global: true, command: 'intercept.forward' },
  { key: 'backspace', mod: true, page: 'intercept', global: true, command: 'intercept.drop' },
  { key: 'i', mod: true, command: 'intercept.toggle' },
  { key: ']', mod: true, global: true, command: 'tab.next' },
  { key: '[', mod: true, global: true, command: 'tab.prev' },
  { key: 'w', mod: true, global: true, command: 'tab.close' },
  { key: '/', command: 'querybar.focus' },
  { key: 'f', mod: true, global: true, command: 'editor.find' },
  { key: 'delete', command: 'selection.delete' },
  { key: 'backspace', command: 'selection.delete' },
];

const CHORD_TIMEOUT_MS = 1000;

function isEditable(t: EventTarget | null): boolean {
  if (!(t instanceof HTMLElement)) return false;
  return t.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(t.tagName);
}

export interface ShortcutOptions {
  bindings?: Binding[];
  /** Called after every rebind/reset so settings can persist. */
  persist?: (bindings: Binding[]) => void;
  /** Current page id for page-scoped bindings; defaults to the router. */
  page?: () => string | undefined;
}

/** Maps key events to command ids: modifier bindings plus the `g` then letter go-to chord. */
export class ShortcutManager {
  bindings: Binding[];
  #awaitingGoto = false;
  #timer: ReturnType<typeof setTimeout> | undefined;
  #persist: ShortcutOptions['persist'];
  #page: () => string | undefined;

  constructor(
    private registry: CommandRegistry,
    { bindings = defaultBindings, persist, page }: ShortcutOptions = {},
  ) {
    this.bindings = bindings.map((b) => ({ ...b }));
    this.#persist = persist;
    this.#page = page ?? (() => router.location.page);
  }

  handle = (e: KeyboardEvent): void => {
    if (e.defaultPrevented) return;
    const mod = e.ctrlKey || e.metaKey;
    const key = e.key.toLowerCase();
    const page = this.#page();

    // Page-specific binding wins over an unscoped one on the same key.
    const hits = this.bindings.filter(
      (b) => b.key === key && !!b.mod === mod && (!b.page || b.page === page),
    );
    const hit = hits.find((b) => b.page) ?? hits[0];
    if (hit && (!isEditable(e.target) || hit.global)) {
      e.preventDefault();
      void this.registry.run(hit.command, { page });
      return;
    }
    if (mod || e.altKey || isEditable(e.target)) return;

    if (this.#awaitingGoto) {
      this.#reset();
      const route = routes.find((r) => r.goto === key);
      if (route) {
        e.preventDefault();
        void this.registry.run(`nav.goto.${route.id}`, { page });
      }
    } else if (key === 'g') {
      this.#awaitingGoto = true;
      this.#timer = setTimeout(() => this.#reset(), CHORD_TIMEOUT_MS);
    }
  };

  /** Bindings that collide with `key`/`mod` in an overlapping scope. */
  conflictFor(key: string, mod = false, page?: string): Binding[] {
    const k = key.toLowerCase();
    return this.bindings.filter(
      (b) => b.key === k && !!b.mod === mod && (!b.page || !page || b.page === page),
    );
  }

  /**
   * Rebinds a command, keeping its page scope. Returns the id of a conflicting command
   * (the rebind is then refused) or null.
   */
  rebind(command: string, key: string, { mod = false }: { mod?: boolean } = {}): string | null {
    const current = this.bindings.find((b) => b.command === command);
    const [conflict] = this.conflictFor(key, mod, current?.page).filter(
      (b) => b.command !== command,
    );
    if (conflict) return conflict.command;
    const binding: Binding = {
      page: current?.page,
      global: current?.global,
      key: key.toLowerCase(),
      command,
    };
    if (mod) binding.mod = true;
    this.bindings = [...this.bindings.filter((b) => b.command !== command), binding];
    this.#changed();
    return null;
  }

  reset(): void {
    this.bindings = defaultBindings.map((b) => ({ ...b }));
    this.#changed();
  }

  #changed() {
    this.#persist?.(this.bindings.map((b) => ({ ...b })));
  }

  #reset() {
    this.#awaitingGoto = false;
    clearTimeout(this.#timer);
  }
}
