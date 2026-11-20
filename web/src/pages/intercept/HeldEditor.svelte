<script lang="ts">
  // Editor pane of the Intercept page: the item's action buttons over a raw editor. Every button
  // runs a registered command, so palette and shortcuts do the same thing.
  import { registry } from '$lib/commands';
  import RawEditor from '$lib/editor/RawEditor.svelte';
  import type { InterceptItem } from '$lib/stores/intercept.svelte';

  interface Props {
    item: InterceptItem;
    bytes: Uint8Array;
    dirty: boolean;
    /** Over the 16 MiB edit cap: shown read-only (F-INT-1). */
    tooBig: boolean;
    busy: boolean;
    message: string | null;
    onedit: (bytes: Uint8Array) => void;
  }

  let { item, bytes, dirty, tooBig, busy, message, onedit }: Props = $props();

  const btn =
    'rounded-token border border-line bg-subtle px-2 py-1 hover:bg-hover disabled:opacity-50';
</script>

<div class="flex flex-wrap items-center gap-2 border-b border-line px-2 py-1">
  <button
    type="button"
    class={btn}
    disabled={busy}
    title="Forward (⌘/Ctrl+Enter)"
    onclick={() => registry.run('intercept.forward')}>Forward</button
  >
  <button
    type="button"
    class={btn}
    disabled={busy}
    title="Drop (⌘/Ctrl+Backspace)"
    onclick={() => registry.run('intercept.drop')}>Drop</button
  >
  <button type="button" class={btn} disabled={!dirty} onclick={() => registry.run('intercept.undo')}
    >Undo</button
  >
  <button type="button" class={btn} onclick={() => registry.run('intercept.replay')}
    >Send to Replay</button
  >
  <button type="button" class={btn} onclick={() => registry.run('intercept.copy.curl')}
    >Copy as cURL</button
  >
  {#if message}<span role="alert" class="text-xs text-danger">{message}</span>{/if}
</div>
{#if tooBig}
  <p class="border-b border-line px-2 py-1 text-xs text-muted" role="note">
    This message is over 16 MiB, so it is read-only: it can only be forwarded unchanged or dropped.
  </p>
{/if}
<div class="min-h-0 flex-1">
  <RawEditor
    value={bytes}
    identity={item.id}
    readonly={tooBig}
    label="Held {item.kind === 'REQUEST' ? 'request' : 'response'}"
    onchange={onedit}
  />
</div>
