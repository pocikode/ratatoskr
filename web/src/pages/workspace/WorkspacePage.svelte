<script lang="ts">
  import { Tabs } from 'bits-ui';
  import {
    projectRowCommands,
    projects as defaultStore,
    type Project,
    type ProjectStore,
  } from '$lib/stores/project.svelte';
  import { registry } from '$lib/commands';
  import ProjectDialog from './ProjectDialog.svelte';

  let { store = defaultStore }: { store?: ProjectStore } = $props();

  $effect(() => registry.register(...projectRowCommands(store)));

  const tabs = [
    { id: 'projects', label: 'Projects' },
    { id: 'backups', label: 'Backups' },
  ];
  const run = (id: string, project: Project) => registry.run(`project.${id}`, { project });
  const fmtSize = (s: string) => {
    const n = Number(s);
    return n < 1024
      ? `${n} B`
      : n < 1024 ** 2
        ? `${(n / 1024).toFixed(1)} KB`
        : n < 1024 ** 3
          ? `${(n / 1024 ** 2).toFixed(1)} MB`
          : `${(n / 1024 ** 3).toFixed(1)} GB`;
  };
  const fmtDate = (t: string | null | undefined) => (t ? new Date(t).toLocaleString() : '—');
  const btn = 'rounded-token border border-line px-2 py-1 hover:bg-hover';
</script>

<Tabs.Root value="projects">
  <Tabs.List class="mb-3 flex gap-1 border-b border-line" aria-label="Workspace sections">
    {#each tabs as { id, label } (id)}
      <Tabs.Trigger
        value={id}
        class="px-3 py-1.5 data-[state=active]:border-b-2 data-[state=active]:border-accent"
      >
        {label}
      </Tabs.Trigger>
    {/each}
  </Tabs.List>

  <Tabs.Content value="projects">
    <div class="mb-3 flex items-center justify-between">
      <h1 class="text-lg font-semibold">Workspace</h1>
      <button type="button" class={btn} onclick={() => registry.run('project.create')}>
        New project
      </button>
    </div>
    {#if store.error}<p role="alert" class="mb-2 text-danger">{store.error}</p>{/if}
    {#if store.loading}
      <p class="text-muted">Loading…</p>
    {:else}
      <table class="w-full text-left">
        <thead class="text-muted">
          <tr><th>Name</th><th>Created</th><th>Last opened</th><th>Size</th><th>Actions</th></tr>
        </thead>
        <tbody>
          {#each store.projects as p (p.id)}
            <tr class="border-t border-line" aria-current={p.isSelected || undefined}>
              <td class="py-1.5">
                {p.name}
                {#if p.isSelected}<span class="text-accent">(current)</span>{/if}
                {#if p.isTemporary}<span class="text-warn">(temporary)</span>{/if}
                {#if p.isDefault}<span class="text-muted">(default)</span>{/if}
              </td>
              <td>{fmtDate(p.createdAt)}</td>
              <td>{fmtDate(p.lastOpenedAt)}</td>
              <td>{fmtSize(p.dbSizeBytes)}</td>
              <td class="py-1"
                ><div class="flex flex-wrap gap-1">
                  {#if !p.isSelected}
                    <button
                      type="button"
                      class={btn}
                      aria-label={`Select ${p.name}`}
                      onclick={() => run('select', p)}>Select</button
                    >
                  {/if}
                  <button
                    type="button"
                    class={btn}
                    aria-label={`Rename ${p.name}`}
                    onclick={() => run('rename', p)}>Rename</button
                  >
                  <button
                    type="button"
                    class={btn}
                    aria-label={`Duplicate ${p.name}`}
                    onclick={() => run('duplicate', p)}>Duplicate</button
                  >
                  {#if !p.isTemporary}
                    <button
                      type="button"
                      class={btn}
                      aria-label={p.isDefault
                        ? `Clear default ${p.name}`
                        : `Set ${p.name} as default`}
                      onclick={() => run(p.isDefault ? 'clearDefault' : 'setDefault', p)}
                    >
                      {p.isDefault ? 'Clear default' : 'Set default'}
                    </button>
                  {/if}
                  <button
                    type="button"
                    class="{btn} text-danger"
                    aria-label={`Delete ${p.name}`}
                    onclick={() => run('delete', p)}>Delete</button
                  >
                </div></td
              >
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}
  </Tabs.Content>

  <Tabs.Content value="backups"
    ><p class="text-muted">Backups arrive with a later task.</p></Tabs.Content
  >
</Tabs.Root>

<ProjectDialog {store} />
