import { routes } from '../router/routes';
import type { CommandRegistry } from './registry';

export interface Binding {
  /** `mod` = Ctrl on Win/Linux, ⌘ on macOS. */
  key: string;
  mod?: boolean;
  command: string;
}

export const defaultBindings: Binding[] = [
  { key: 'k', mod: true, command: 'palette.open' },
  { key: 'b', mod: true, command: 'sidebar.toggle' },
];

const CHORD_TIMEOUT_MS = 1000;

function isEditable(t: EventTarget | null): boolean {
  if (!(t instanceof HTMLElement)) return false;
  return t.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(t.tagName);
}

/** Maps key events to command ids: modifier bindings plus the `g` then letter go-to chord. */
export class ShortcutManager {
  #awaitingGoto = false;
  #timer: ReturnType<typeof setTimeout> | undefined;

  constructor(
    private registry: CommandRegistry,
    private bindings: Binding[] = defaultBindings,
  ) {}

  handle = (e: KeyboardEvent): void => {
    if (e.defaultPrevented) return;
    const mod = e.ctrlKey || e.metaKey;
    const key = e.key.toLowerCase();

    const hit = this.bindings.find((b) => b.key === key && !!b.mod === mod);
    if (hit && (mod || !isEditable(e.target))) {
      e.preventDefault();
      void this.registry.run(hit.command);
      return;
    }
    if (mod || e.altKey || isEditable(e.target)) return;

    if (this.#awaitingGoto) {
      this.#reset();
      const route = routes.find((r) => r.goto === key);
      if (route) {
        e.preventDefault();
        void this.registry.run(`nav.goto.${route.id}`);
      }
    } else if (key === 'g') {
      this.#awaitingGoto = true;
      this.#timer = setTimeout(() => this.#reset(), CHORD_TIMEOUT_MS);
    }
  };

  #reset() {
    this.#awaitingGoto = false;
    clearTimeout(this.#timer);
  }
}
