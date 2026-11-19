<script lang="ts">
  // New collection and delete confirmation; both are opened by registry commands.
  import { Dialog } from 'bits-ui';
  import type { ReplayState } from './replay.svelte';

  let { replay }: { replay: ReplayState } = $props();

  let name = $state('');
  const pending = $derived(replay.pendingDelete);
  const doomed = $derived(
    pending?.kind === 'session'
      ? replay.sessions.get(pending.id)?.name
      : replay.collections.find((c) => c.id === pending?.id)?.name,
  );
  const content =
    'fixed top-1/4 left-1/2 z-50 w-96 -translate-x-1/2 rounded-token border border-line bg-elevated p-4';
  const button = 'rounded-token border border-line px-2 py-1 hover:bg-hover';
</script>

<Dialog.Root
  open={replay.newCollectionOpen}
  onOpenChange={(o) => {
    if (!o) replay.newCollectionOpen = false;
  }}
>
  <Dialog.Portal>
    <Dialog.Overlay class="fixed inset-0 z-40 bg-[var(--scrim)]" />
    <Dialog.Content class={content} style="box-shadow: var(--shadow-overlay)">
      <form
        class="flex flex-col gap-3"
        onsubmit={async (e) => {
          e.preventDefault();
          if (await replay.createCollection(name)) {
            name = '';
            replay.newCollectionOpen = false;
          }
        }}
      >
        <Dialog.Title class="font-semibold">New collection</Dialog.Title>
        <Dialog.Description class="sr-only">Name of the new collection.</Dialog.Description>
        <input
          aria-label="Name"
          class="rounded-token border border-line bg-subtle px-2 py-1"
          bind:value={name}
        />
        {#if replay.error}<p role="alert" class="text-xs text-danger">{replay.error}</p>{/if}
        <div class="flex justify-end gap-2">
          <Dialog.Close type="button" class={button}>Cancel</Dialog.Close>
          <button type="submit" class={button}>Create</button>
        </div>
      </form>
    </Dialog.Content>
  </Dialog.Portal>
</Dialog.Root>

<Dialog.Root
  open={pending !== null}
  onOpenChange={(o) => {
    if (!o) replay.pendingDelete = null;
  }}
>
  <Dialog.Portal>
    <Dialog.Overlay class="fixed inset-0 z-40 bg-[var(--scrim)]" />
    <Dialog.Content class={content} style="box-shadow: var(--shadow-overlay)">
      <Dialog.Title class="font-semibold">Delete {pending?.kind} “{doomed}”?</Dialog.Title>
      <Dialog.Description class="mt-2 text-xs text-muted">
        {pending?.kind === 'collection'
          ? 'Its sessions and their entries are deleted too.'
          : 'Its entries are deleted too.'} Requests stay in HTTP History.
      </Dialog.Description>
      <div class="mt-3 flex justify-end gap-2">
        <Dialog.Close type="button" class={button}>Cancel</Dialog.Close>
        <button type="button" class={button} onclick={() => pending && void replay.remove(pending)}>
          Delete
        </button>
      </div>
    </Dialog.Content>
  </Dialog.Portal>
</Dialog.Root>
