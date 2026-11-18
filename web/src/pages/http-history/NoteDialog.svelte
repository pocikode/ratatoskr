<script lang="ts">
  import { Dialog } from 'bits-ui';
  import type { HistoryRow } from './columns';

  interface Props {
    /** The row being edited; null keeps the dialog closed. */
    row: HistoryRow | null;
    onsave: (note: string) => void;
    onclose: () => void;
  }

  let { row, onsave, onclose }: Props = $props();

  // Writable derived: starts from the row's note whenever another row opens.
  let text = $derived(row?.note ?? '');
</script>

<Dialog.Root open={row !== null} onOpenChange={(open) => !open && onclose()}>
  <Dialog.Portal>
    <Dialog.Overlay class="fixed inset-0 z-40 bg-black/40" />
    <Dialog.Content
      class="fixed top-1/4 left-1/2 z-50 w-96 -translate-x-1/2 rounded-token border border-line
        bg-elevated p-4 shadow-lg"
    >
      <form
        class="flex flex-col gap-3"
        onsubmit={(event) => {
          event.preventDefault();
          onsave(text);
        }}
      >
        <Dialog.Title class="font-semibold">Request note</Dialog.Title>
        <Dialog.Description class="text-xs text-muted">
          Row {row?.id ?? ''} · {row ? `${row.method} ${row.host}${row.path}` : ''}
        </Dialog.Description>
        <input
          aria-label="Note"
          class="rounded-token border border-line bg-subtle px-2 py-1"
          placeholder="Empty clears the note"
          bind:value={text}
        />
        <div class="flex justify-end gap-2">
          <Dialog.Close
            type="button"
            class="rounded-token border border-line px-2 py-1 hover:bg-hover">Cancel</Dialog.Close
          >
          <button type="submit" class="rounded-token border border-line px-2 py-1 hover:bg-hover">
            Save
          </button>
        </div>
      </form>
    </Dialog.Content>
  </Dialog.Portal>
</Dialog.Root>
