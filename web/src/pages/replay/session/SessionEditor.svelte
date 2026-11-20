<script lang="ts">
  // Replay session body (F-REP-2, docs/02 §3.7): connection bar, request editor, send/cancel,
  // entry history and the response pane. The shell keys this by session id, so one instance
  // edits one session and unmounting saves its pending draft.
  import { untrack } from 'svelte';
  import type { Client } from '@urql/core';
  import { getApi } from '$lib/api/client';
  import { registry } from '$lib/commands';
  import MessageViewer from '$lib/components/MessageViewer/MessageViewer.svelte';
  import SplitPane from '$lib/components/SplitPane/SplitPane.svelte';
  import RawEditor from '$lib/editor/RawEditor.svelte';
  import type { EditorVariable } from '$lib/editor/http';
  import { environment as defaultEnv, type EnvironmentStore } from '$lib/stores/environment.svelte';
  import ConnectionBar from './ConnectionBar.svelte';
  import EntryNav from './EntryNav.svelte';
  import { messageText, SessionEditorState } from './session.svelte';

  interface Props {
    sessionId: string;
    /** GraphQL client override for tests; the editor otherwise uses the shared client. */
    client?: Client;
    /** Environment store override for tests; the status bar starts the shared one. */
    env?: EnvironmentStore;
  }

  let { sessionId, client, env = defaultEnv }: Props = $props();

  const state = new SessionEditorState(
    untrack(() => client) ?? getApi().client,
    untrack(() => sessionId),
  );

  $effect(() => {
    void state.load();
    return () => state.dispose();
  });
  $effect(() =>
    registry.register({
      id: 'replay.send',
      title: 'Send Replay request',
      group: 'Replay',
      run: () => void state.send(),
    }),
  );

  // `{{var}}` hover values (F-ENV-1): the selected environment over Global, secrets masked.
  const variables = $derived.by(() => {
    const out: Record<string, EditorVariable> = {};
    for (const e of env.environments)
      if (e.isGlobal || e.selected)
        for (const v of e.variables) out[v.name] = { value: v.value, secret: v.secret };
    return out;
  });

  const response = $derived(state.viewed?.request.response ?? null);
  const responseText = $derived(response ? messageText(response) : '');
  const size = (bytes: string) => {
    const n = Number(bytes);
    return n < 1024 ? `${n} B` : `${(n / 1024).toFixed(1)} KB`;
  };
</script>

<div class="flex h-full min-h-0 flex-col">
  {#if state.session}
    <ConnectionBar
      connection={state.connection}
      options={state.options}
      environment={env}
      onconnection={(c) => void state.saveConnection(c)}
      onoptions={(o) => void state.saveOptions(o)}
    />
  {/if}

  <div class="flex min-h-9 flex-wrap items-center gap-2 border-b border-line px-2 py-1">
    <button
      type="button"
      class="rounded-token bg-accent px-2 py-0.5 text-xs text-on-accent hover:bg-accent/90
        disabled:opacity-50"
      aria-busy={state.sending}
      disabled={state.sending || state.entryTruncated || !state.session}
      onclick={() => void state.send()}>{state.sending ? 'Sending…' : 'Send'}</button
    >
    <button
      type="button"
      class="rounded-token border border-line px-2 py-0.5 text-xs hover:bg-hover
        disabled:opacity-50"
      disabled={!state.sending}
      onclick={() => void state.cancel()}>Cancel</button
    >
    <span class="text-xs text-muted" aria-hidden="true">⌘⏎</span>
    <EntryNav
      entries={state.entries}
      viewed={state.viewed}
      position={state.position}
      onselect={(id) => void state.selectEntry(id)}
    />
  </div>

  {#if state.error}
    <p role="alert" class="border-b border-line px-2 py-1 text-xs text-danger">{state.error}</p>
  {/if}
  {#if state.entryTruncated}
    <p role="status" class="border-b border-line px-2 py-1 text-xs text-warn">
      This entry's request is longer than the 4 MiB shown here, so it opens read-only.
    </p>
  {/if}

  <div class="min-h-0 flex-1">
    {#if state.loading}
      <p class="p-4 text-muted">Loading…</p>
    {:else if state.session}
      <SplitPane direction="horizontal" defaultSizes={[50, 50]} label="Resize request and response">
        {#snippet first()}
          <RawEditor
            value={state.shown}
            identity={state.activeEntryId ?? 'draft'}
            readonly={state.entryTruncated}
            {variables}
            label="Request"
            onchange={(bytes) => state.edit(bytes)}
          />
        {/snippet}

        {#snippet second()}
          <div class="flex h-full min-h-0 flex-col">
            {#if state.viewed?.error}
              <p
                role="alert"
                class="m-2 rounded-token border border-danger px-2 py-1 text-xs text-danger"
              >
                Send failed: {state.viewed.error}
              </p>
            {:else if state.viewed && response}
              <p class="border-b border-line px-2 py-1 text-xs text-muted" aria-live="polite">
                {response.code} · {size(response.length)} · {response.roundtripMs} ms
              </p>
              <div class="min-h-0 flex-1">
                <MessageViewer
                  raw={responseText}
                  title="Response"
                  previewUrl={`/api/rest/preview/${state.viewed.request.id}`}
                />
              </div>
            {:else}
              <p class="flex h-full items-center justify-center text-muted">
                No response yet. Send the request to see one.
              </p>
            {/if}
          </div>
        {/snippet}
      </SplitPane>
    {/if}
  </div>
</div>
