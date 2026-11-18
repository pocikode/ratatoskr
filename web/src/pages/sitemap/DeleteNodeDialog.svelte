<script lang="ts">
  import { Dialog } from 'bits-ui';
  import type { SitemapState } from './sitemap.svelte';
  let { state }: { state: SitemapState } = $props();
</script>

<Dialog.Root
  open={state.confirmation !== null}
  onOpenChange={(open) => {
    if (!open && !state.deleting) state.confirmation = null;
  }}
>
  <Dialog.Portal>
    <Dialog.Overlay class="fixed inset-0 z-40 bg-[var(--scrim)]" />
    <Dialog.Content
      class="fixed top-1/4 left-1/2 z-50 w-96 -translate-x-1/2 rounded-token border border-line bg-elevated p-4"
      style="box-shadow: var(--shadow-overlay)"
    >
      <Dialog.Title class="font-semibold">Delete sitemap node?</Dialog.Title>
      <Dialog.Description class="mt-2 text-xs text-muted">
        Delete {state.confirmation?.node.requestCount} matching requests under {state.confirmation
          ?.node.label}? This removes them from history.
      </Dialog.Description>
      {#if state.message}<p role="alert" class="mt-2 text-xs text-danger">{state.message}</p>{/if}
      <div class="mt-4 flex justify-end gap-2">
        <Dialog.Close
          disabled={state.deleting}
          class="rounded-token border border-line px-2 py-1 hover:bg-hover">Cancel</Dialog.Close
        >
        <button
          type="button"
          disabled={state.deleting}
          class="rounded-token border border-line px-2 py-1 text-danger hover:bg-hover"
          onclick={() => state.remove()}>{state.deleting ? 'Deleting…' : 'Delete requests'}</button
        >
      </div>
    </Dialog.Content>
  </Dialog.Portal>
</Dialog.Root>
