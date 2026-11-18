<script lang="ts">
  import { Dialog } from 'bits-ui';
  import type { ProjectStore } from '$lib/stores/project.svelte';

  let { store }: { store: ProjectStore } = $props();

  const d = $derived(store.dialog);
  // Writable derived: resets to the dialog's initial text whenever a new dialog opens.
  let text = $derived(d?.kind === 'rename' ? d.project.name : '');
  // Delete needs the exact project name typed back.
  const valid = $derived(d?.kind === 'delete' ? text === d.project.name : text.trim().length > 0);

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    if (!d || !valid) return;
    const ok =
      d.kind === 'create'
        ? await store.create(text)
        : d.kind === 'rename'
          ? await store.rename(d.project.id, text)
          : await store.remove(d.project.id);
    if (ok) store.dialog = null;
  }

  const titles = { create: 'New project', rename: 'Rename project', delete: 'Delete project' };
</script>

<Dialog.Root open={d !== null} onOpenChange={(o) => !o && (store.dialog = null)}>
  <Dialog.Portal>
    <Dialog.Overlay class="fixed inset-0 z-40 bg-black/40" />
    <Dialog.Content
      class="fixed top-1/4 left-1/2 z-50 w-96 -translate-x-1/2 rounded-token border border-line bg-elevated p-4 shadow-lg"
    >
      {#if d}
        <form onsubmit={submit} class="flex flex-col gap-3">
          <Dialog.Title class="font-semibold">{titles[d.kind]}</Dialog.Title>
          {#if d.kind === 'delete'}
            <Dialog.Description class="text-muted">
              This permanently deletes the project and its data. Type
              <strong>{d.project.name}</strong> to confirm.
            </Dialog.Description>
          {:else}
            <Dialog.Description class="sr-only">Enter a project name.</Dialog.Description>
          {/if}
          <input
            aria-label={d.kind === 'delete' ? 'Confirm project name' : 'Project name'}
            class="rounded-token border border-line bg-subtle px-2 py-1"
            bind:value={text}
          />
          {#if store.error}<p role="alert" class="text-danger">{store.error}</p>{/if}
          <div class="flex justify-end gap-2">
            <Dialog.Close
              type="button"
              class="rounded-token border border-line px-2 py-1 hover:bg-hover">Cancel</Dialog.Close
            >
            <button
              type="submit"
              disabled={!valid}
              class="rounded-token border border-line px-2 py-1 hover:bg-hover disabled:opacity-50 {d.kind ===
              'delete'
                ? 'text-danger'
                : ''}"
            >
              {d.kind === 'create' ? 'Create' : d.kind === 'rename' ? 'Rename' : 'Delete'}
            </button>
          </div>
        </form>
      {/if}
    </Dialog.Content>
  </Dialog.Portal>
</Dialog.Root>
