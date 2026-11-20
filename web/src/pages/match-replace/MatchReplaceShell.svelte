<script lang="ts">
  // Match & Replace page (F-MR-1, docs/02 §3.6): collections and rules | rule editor | tester.
  import type { Client } from '@urql/core';
  import { untrack } from 'svelte';
  import { getApi } from '$lib/api/client';
  import { registry } from '$lib/commands';
  import type { CommandContext } from '$lib/commands/registry';
  import SplitPane from '$lib/components/SplitPane/SplitPane.svelte';
  import { MatchReplaceState } from './matchreplace.svelte';
  import RuleEditor from './RuleEditor.svelte';
  import RuleList from './RuleList.svelte';
  import TesterPanel from './tester/TesterPanel.svelte';

  interface Props {
    /** GraphQL client override for tests; the page otherwise uses the shared client. */
    client?: Client;
  }

  let { client }: Props = $props();

  // Dev-only mock backend (T-013): `?mock=match-replace` or `localStorage['match-replace.mock'] = '1'`.
  function mockRequested(): boolean {
    try {
      return (
        new URLSearchParams(window.location.search).get('mock') === 'match-replace' ||
        localStorage.getItem('match-replace.mock') === '1'
      );
    } catch {
      return false;
    }
  }

  const mr = new MatchReplaceState(untrack(() => client) ?? getApi().client);

  $effect(() => {
    let cancelled = false;
    void (async () => {
      if (!client && import.meta.env.DEV && mockRequested()) {
        const { createMockClient } = await import('$lib/api/mocks/exchange');
        if (cancelled) return;
        mr.useClient(createMockClient());
      }
      if (!cancelled) await mr.start();
    })();
    return () => {
      cancelled = true;
    };
  });

  // shortcut: window.confirm/prompt for the unsaved-edit guard, rename and delete; upgrade to a dialog component.
  function guard(): boolean {
    return !mr.dirty || window.confirm('Discard unsaved changes to this rule?');
  }

  // Deletes the rule with this id (a context-menu row, else the open rule), after naming it.
  async function remove(id: string | null | undefined = mr.draft?.id) {
    const rule = id ? mr.collections.flatMap((c) => c.rules).find((r) => r.id === id) : undefined;
    if (!rule || !window.confirm(`Delete rule "${rule.name}"?`)) return;
    await mr.removeRule(rule.id);
  }

  async function exportRules() {
    const json = await mr.exportJson();
    if (json === null) return;
    const url = URL.createObjectURL(new Blob([json], { type: 'application/json' }));
    const link = document.createElement('a');
    link.href = url;
    link.download = 'match-replace.json';
    link.click();
    URL.revokeObjectURL(url);
  }

  const ruleOf = (ctx: CommandContext) => (ctx as { mrRule?: string }).mrRule;

  // The context menu of a rule row runs these through the registry (docs/06 §5).
  $effect(() =>
    registry.register(
      {
        id: 'mr.rule.new',
        title: 'New Match & Replace rule',
        group: 'Match & Replace',
        run: () => void (guard() && mr.newRule()),
      },
      {
        id: 'mr.rule.delete',
        title: 'Delete Match & Replace rule',
        group: 'Match & Replace',
        when: (ctx) => (ruleOf(ctx) ?? mr.draft?.id) != null,
        // Row menus carry their row id; the palette has none and acts on the open rule.
        run: (ctx) => remove(ruleOf(ctx)),
      },
    ),
  );
</script>

<div class="h-[calc(100vh-140px)] rounded-token border border-line">
  <SplitPane defaultSizes={[30, 70]} label="Resize rule list">
    {#snippet first()}
      <RuleList
        {mr}
        {guard}
        onexport={() => void exportRules()}
        onimport={async (file) => void (await mr.importJson(await file.text()))}
      />
    {/snippet}
    {#snippet second()}
      <SplitPane defaultSizes={[57, 43]} label="Resize tester">
        {#snippet first()}
          <RuleEditor {mr} onsave={() => void mr.save()} ondelete={() => void remove()} />
        {/snippet}
        {#snippet second()}
          <TesterPanel rule={mr.testerRule} />
        {/snippet}
      </SplitPane>
    {/snippet}
  </SplitPane>
</div>
