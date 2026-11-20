<script lang="ts">
  import { Dialog } from 'bits-ui';

  interface Props {
    open: boolean;
    /** Returns an error message, or null when the session was created. */
    oncreate: (form: {
      tls: boolean;
      host: string;
      port: string;
      path: string;
    }) => Promise<string | null>;
    onclose: () => void;
  }

  let { open, oncreate, onclose }: Props = $props();

  let tls = $state(true);
  let host = $state('');
  let port = $state('');
  let path = $state('/');
  let error = $state<string | null>(null);
  let pending = $state(false);

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    if (pending) return;
    pending = true;
    try {
      error = await oncreate({ tls, host, port, path });
    } finally {
      pending = false;
    }
    if (!error) {
      host = port = '';
      path = '/';
      onclose();
    }
  }
</script>

<Dialog.Root {open} onOpenChange={(o) => !o && onclose()}>
  <Dialog.Portal>
    <Dialog.Overlay class="fixed inset-0 z-40 bg-[var(--scrim)]" />
    <Dialog.Content
      class="fixed top-1/4 left-1/2 z-50 w-96 -translate-x-1/2 rounded-token border border-line bg-elevated p-4"
      style="box-shadow: var(--shadow-overlay)"
    >
      <form class="flex flex-col gap-3" onsubmit={submit}>
        <Dialog.Title class="font-semibold">New session</Dialog.Title>
        <Dialog.Description class="sr-only"
          >Connection and path of the first request.</Dialog.Description
        >
        <div class="flex gap-2">
          <select
            aria-label="Scheme"
            class="rounded-token border border-line bg-subtle px-2 py-1"
            bind:value={tls}
          >
            <option value={true}>https</option>
            <option value={false}>http</option>
          </select>
          <input
            aria-label="Host"
            class="min-w-0 flex-1 rounded-token border border-line bg-subtle px-2 py-1"
            placeholder="example.test"
            bind:value={host}
          />
          <input
            aria-label="Port"
            inputmode="numeric"
            class="w-20 rounded-token border border-line bg-subtle px-2 py-1"
            placeholder={tls ? '443' : '80'}
            bind:value={port}
          />
        </div>
        <input
          aria-label="Path"
          class="rounded-token border border-line bg-subtle px-2 py-1"
          bind:value={path}
        />
        {#if error}<p role="alert" class="text-xs text-danger">{error}</p>{/if}
        <div class="flex justify-end gap-2">
          <Dialog.Close
            type="button"
            class="rounded-token border border-line px-2 py-1 hover:bg-hover">Cancel</Dialog.Close
          >
          <button
            type="submit"
            disabled={pending}
            class="rounded-token border border-line px-2 py-1 hover:bg-hover disabled:opacity-50"
            >Create</button
          >
        </div>
      </form>
    </Dialog.Content>
  </Dialog.Portal>
</Dialog.Root>
