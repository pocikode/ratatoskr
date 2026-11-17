import type { IconName } from '../utils/icons';

export interface CommandContext {
  /** Extended by later tasks (selected rows, active page …). */
  page?: string;
}

export interface Command {
  /** Namespaced id, e.g. `nav.goto.replay`. */
  id: string;
  title: string;
  icon?: IconName;
  /** Palette-only grouping label. */
  group?: string;
  /** Hidden from palette/menus when it returns false. */
  when?: (ctx: CommandContext) => boolean;
  run: (ctx: CommandContext) => void | Promise<void>;
}

/** Single source of truth for palette entries, shortcuts and menus (docs/06 §5). */
export class CommandRegistry {
  #commands = new Map<string, Command>();

  /** Registers commands; returns an unregister function (plugins use it later). */
  register(...cmds: Command[]): () => void {
    for (const c of cmds) this.#commands.set(c.id, c);
    return () => cmds.forEach((c) => this.#commands.delete(c.id));
  }

  get(id: string): Command | undefined {
    return this.#commands.get(id);
  }

  available(ctx: CommandContext = {}): Command[] {
    return [...this.#commands.values()].filter((c) => c.when?.(ctx) ?? true);
  }

  async run(id: string, ctx: CommandContext = {}): Promise<boolean> {
    const cmd = this.#commands.get(id);
    if (!cmd || !(cmd.when?.(ctx) ?? true)) return false;
    await cmd.run(ctx);
    return true;
  }
}

/** Case-insensitive, all-words-must-match filter; prefix matches rank first. */
export function filterCommands(cmds: Command[], query: string): Command[] {
  const words = query.toLowerCase().split(/\s+/).filter(Boolean);
  if (words.length === 0) return cmds;
  const scored: [number, Command][] = [];
  for (const c of cmds) {
    const title = c.title.toLowerCase();
    if (!words.every((w) => title.includes(w))) continue;
    const rank = words.some((w) => title.split(/\s+/).some((t) => t.startsWith(w))) ? 0 : 1;
    scored.push([rank, c]);
  }
  // Stable sort: word-prefix matches first, then shorter titles ("repl" → Replay before Match & Replace).
  return scored
    .sort((a, b) => a[0] - b[0] || a[1].title.length - b[1].title.length)
    .map(([, c]) => c);
}
