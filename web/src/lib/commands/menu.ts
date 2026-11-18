import type { Command, CommandContext, CommandRegistry } from './registry';

/**
 * Commands for a context menu (docs/02 §5): the given ids in the given order,
 * minus unregistered ones and those whose `when` rejects the context.
 */
export function commandMenu(
  registry: CommandRegistry,
  ids: string[],
  ctx: CommandContext = {},
): Command[] {
  const available = new Set(registry.available(ctx).map((c) => c.id));
  return ids.flatMap((id) => {
    const cmd = registry.get(id);
    return cmd && available.has(id) ? [cmd] : [];
  });
}
