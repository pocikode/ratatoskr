<script lang="ts">
  import { ContextMenu as Menu } from 'bits-ui';
  import type { Snippet } from 'svelte';
  import { registry as defaultRegistry } from '$lib/commands';
  import { commandMenu } from '$lib/commands/menu';
  import type { CommandContext, CommandRegistry } from '$lib/commands/registry';
  import { iconFor } from '$lib/utils/icons';

  interface Props {
    /** Command ids, in menu order; hidden when unregistered or `when(ctx)` is false. */
    ids: string[];
    context?: CommandContext;
    registry?: CommandRegistry;
    /** Trigger area. */
    children: Snippet;
  }

  let { ids, context = {}, registry = defaultRegistry, children }: Props = $props();

  const items = $derived(commandMenu(registry, ids, context));
</script>

{#snippet trigger({ props }: { props: Record<string, unknown> })}
  <div {...props}>{@render children()}</div>
{/snippet}

<Menu.Root>
  <Menu.Trigger child={trigger} disabled={items.length === 0} />
  <Menu.Portal>
    <Menu.Content
      class="z-50 min-w-40 rounded-token border border-line bg-elevated p-1"
      style="box-shadow: var(--shadow-overlay)"
    >
      {#each items as cmd, i (cmd.id)}
        {@const Icon = iconFor(cmd.icon)}
        {#if i > 0 && items[i - 1]?.group !== cmd.group}
          <Menu.Separator class="my-1 h-px bg-line" />
        {/if}
        <Menu.Item
          class="flex cursor-default items-center gap-2 rounded-token px-2 py-1 outline-none data-highlighted:bg-hover"
          onSelect={() => registry.run(cmd.id, context)}
        >
          {#if Icon}<Icon size={14} aria-hidden="true" />{/if}
          <span class="flex-1 truncate">{cmd.title}</span>
        </Menu.Item>
      {/each}
    </Menu.Content>
  </Menu.Portal>
</Menu.Root>
