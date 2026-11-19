<script lang="ts" module>
  const last: { id: string | null } = { id: null };
</script>

<script lang="ts">
  import type { Client } from '@urql/core';
  import { untrack } from 'svelte';
  import { getApi } from '$lib/api/client';
  import { registry } from '$lib/commands';
  import SessionSidebar from '$lib/components/SessionSidebar/SessionSidebar.svelte';
  import TabStrip from '$lib/components/TabStrip/TabStrip.svelte';
  import { router } from '$lib/router/router.svelte';
  import { COLLECTION_MENU, replayCommands, SESSION_MENU, TAB_MENU, withTarget } from './commands';
  import NewSessionDialog from './NewSessionDialog.svelte';
  import ReplayDialogs from './ReplayDialogs.svelte';
  import { ReplayState, type Session } from './replay.svelte';
  import SessionEditor from './session/SessionEditor.svelte';

  interface Props {
    projectId: string;
    /** GraphQL client override for tests; the shell otherwise uses the shared client. */
    client?: Client;
  }

  let { projectId, client }: Props = $props();

  // A project switch remounts the shell while the old project's `#/replay/session/<id>` is still in
  // the URL; that id belongs to the other project, so it must not be applied as a deep link.
  const switched = last.id !== null && last.id !== untrack(() => projectId);
  last.id = untrack(() => projectId);
  const staleHash = switched ? router.location.params[1] : undefined;

  // Dev-only mock backend (T-013): `?mock=replay` or `localStorage['replay.mock'] = '1'`.
  function mockRequested(): boolean {
    try {
      return (
        new URLSearchParams(window.location.search).get('mock') === 'replay' ||
        localStorage.getItem('replay.mock') === '1'
      );
    } catch {
      return false;
    }
  }

  const replay = new ReplayState(
    untrack(() => client) ?? getApi().client,
    untrack(() => projectId),
  );
  let synced = $state(false);

  // `--row-*` token of a colour tag (docs/02 §10 names `CYAN` "teal").
  const token = (s: Session) => (s.color === 'CYAN' ? 'teal' : (s.color?.toLowerCase() ?? null));

  $effect(() => {
    let cancelled = false;
    void (async () => {
      if (!client && import.meta.env.DEV && mockRequested()) {
        const { createMockClient } = await import('$lib/api/mocks/exchange');
        if (cancelled) return;
        replay.useClient(createMockClient());
      }
      if (!cancelled) await replay.load();
    })();
    return () => {
      cancelled = true;
    };
  });

  $effect(() => registry.register(...replayCommands(replay)));

  // Route → tabs: the deep link `#/replay/session/<id>` wins over the stored active tab.
  const linked = $derived(
    router.location.page === 'replay' && router.location.params[0] === 'session'
      ? router.location.params[1] === staleHash
        ? undefined
        : router.location.params[1]
      : undefined,
  );
  $effect(() => {
    if (!replay.loaded) return;
    const id = linked;
    untrack(() => {
      if (id) void replay.open(id);
      synced = true;
    });
  });
  // Tabs → route, once the deep link has been applied.
  $effect(() => {
    if (!synced || router.location.page !== 'replay') return;
    const id = replay.activeId;
    if ((id ?? undefined) !== untrack(() => linked)) {
      if (id) router.navigate('replay', 'session', id);
      else router.navigate('replay');
    }
  });

  const groups = $derived(
    replay.filtered.map((c) => ({
      id: c.id,
      label: c.name,
      items: c.sessions.map((s) => ({
        id: s.id,
        label: s.name,
        detail: s.connection.host,
        color: token(s),
      })),
    })),
  );
  const tabs = $derived(
    replay.openSessions.map((s) => ({ id: s.id, label: s.name, color: token(s) })),
  );
  const renamingTab = $derived(replay.renaming?.from === 'tab' ? replay.renaming.id : null);
  const run = (id: string) => void registry.run(id);
</script>

<div class="flex h-full min-h-0">
  <h1 class="sr-only">Replay</h1>
  <aside class="w-60 shrink-0 border-r border-line">
    <SessionSidebar
      {groups}
      search={replay.search}
      activeId={replay.activeId}
      renaming={replay.renaming && replay.renaming.from !== 'tab'
        ? { kind: replay.renaming.kind === 'session' ? 'item' : 'group', id: replay.renaming.id }
        : null}
      onsearch={(v) => (replay.search = v)}
      onselect={(id) => void replay.open(id)}
      onrename={(t, name) =>
        void replay.rename({ kind: t.kind === 'item' ? 'session' : 'collection', id: t.id }, name)}
      oncancelrename={() => (replay.renaming = null)}
      onstartrename={(t) =>
        (replay.renaming = { kind: t.kind === 'item' ? 'session' : 'collection', id: t.id })}
      onmove={(id, to, index) => void replay.move(id, to, index)}
      menu={(t) => {
        const target = { kind: t.kind === 'item' ? 'session' : 'collection', id: t.id } as const;
        return {
          ids: target.kind === 'session' ? SESSION_MENU : COLLECTION_MENU,
          context: withTarget(target),
        };
      }}
    >
      {#snippet footer()}
        <button
          type="button"
          class="rounded-token border border-line px-2 py-1 hover:bg-hover"
          onclick={() => run('replay.session.new')}>+ Session</button
        >
        <button
          type="button"
          class="rounded-token border border-line px-2 py-1 hover:bg-hover"
          onclick={() => run('replay.collection.new')}>+ Collection</button
        >
      {/snippet}
    </SessionSidebar>
  </aside>
  <section class="flex min-w-0 flex-1 flex-col">
    <TabStrip
      {tabs}
      activeId={replay.activeId}
      renamingId={renamingTab}
      onselect={(id) => void replay.open(id)}
      onclose={(id) => replay.close(id)}
      onrename={(id, name) => void replay.rename({ kind: 'session', id }, name)}
      onstartrename={(id) => (replay.renaming = { kind: 'session', id, from: 'tab' })}
      oncancelrename={() => (replay.renaming = null)}
      onnew={() => run('replay.session.new')}
      menu={(id) => ({ ids: TAB_MENU, context: withTarget({ kind: 'session', id, from: 'tab' }) })}
    />
    {#if replay.error}<p role="alert" class="px-3 py-1 text-xs text-danger">{replay.error}</p>{/if}
    <div class="min-h-0 flex-1">
      {#if replay.activeId}
        {#key replay.activeId}<SessionEditor sessionId={replay.activeId} />{/key}
      {:else if replay.loaded}
        <p class="p-4 text-muted">Open a session from the list or create one.</p>
      {/if}
    </div>
  </section>
</div>

<NewSessionDialog
  open={replay.newSessionOpen}
  oncreate={(form) => replay.createFromUrl(form)}
  onclose={() => (replay.newSessionOpen = false)}
/>
<ReplayDialogs {replay} />
