<script lang="ts">
  // Collections tree (docs/02 §3.6): rules with an enable checkbox, drag to reorder or move
  // across collections, ↑/↓ buttons as the keyboard path, context menu from the command registry.
  import type { CommandContext } from '$lib/commands/registry';
  import ContextMenu from '$lib/components/ContextMenu/ContextMenu.svelte';
  import type { MatchReplaceState } from './matchreplace.svelte';

  interface Props {
    mr: MatchReplaceState;
    /** Asks before leaving unsaved edits; resolves to whether to go on. */
    guard: () => boolean;
    onimport: (file: File) => void;
    onexport: () => void;
  }

  let { mr, guard, onimport, onexport }: Props = $props();

  type Drag = { kind: 'rule'; id: string } | { kind: 'collection'; id: string };
  let drag = $state<Drag | null>(null);

  function open(id: string) {
    if (mr.draft?.id === id) return;
    if (guard()) mr.select(id);
  }

  function dropOnRule(collectionId: string, index: number) {
    if (drag?.kind === 'rule') void mr.moveRule(drag.id, collectionId, index);
    drag = null;
  }

  function dropOnCollection(collectionId: string, index: number) {
    if (drag?.kind === 'collection') void mr.moveCollection(drag.id, index);
    else if (drag?.kind === 'rule') {
      const length = mr.collections.find((c) => c.id === collectionId)?.rules.length ?? 0;
      void mr.moveRule(drag.id, collectionId, length);
    }
    drag = null;
  }

  function over(event: DragEvent) {
    if (drag) event.preventDefault();
  }

  let picker: HTMLInputElement;
</script>

<section aria-label="Collections and rules" class="flex h-full min-w-0 flex-col">
  <div class="flex flex-wrap items-center gap-1 border-b border-line px-2 py-1">
    <button
      type="button"
      class="rounded-token border border-line px-2 py-0.5 text-xs hover:bg-hover"
      onclick={() => guard() && mr.newRule()}
    >
      + New rule
    </button>
    <button
      type="button"
      class="rounded-token border border-line px-2 py-0.5 text-xs hover:bg-hover"
      onclick={() => {
        const name = window.prompt('Collection name');
        if (name?.trim()) void mr.createCollection(name);
      }}
    >
      + New collection
    </button>
    <span class="ml-auto flex gap-1">
      <button
        type="button"
        class="rounded-token border border-line px-2 py-0.5 text-xs hover:bg-hover"
        onclick={() => picker.click()}
      >
        Import
      </button>
      <button
        type="button"
        class="rounded-token border border-line px-2 py-0.5 text-xs hover:bg-hover"
        onclick={onexport}
      >
        Export
      </button>
    </span>
    <input
      bind:this={picker}
      type="file"
      accept="application/json,.json"
      aria-label="Import file"
      class="hidden"
      onchange={(event) => {
        const input = event.currentTarget;
        const file = input.files?.[0];
        input.value = ''; // so picking the same file again fires `change`
        if (file) onimport(file);
      }}
    />
  </div>

  <ul class="min-h-0 flex-1 overflow-auto py-1" aria-label="Collections">
    {#each mr.collections as collection, ci (collection.id)}
      <li>
        <div
          role="group"
          aria-label={collection.name}
          draggable="true"
          class="flex items-center gap-1 px-2 py-1 text-xs font-medium hover:bg-hover"
          ondragstart={() => (drag = { kind: 'collection', id: collection.id })}
          ondragend={() => (drag = null)}
          ondragover={over}
          ondrop={() => dropOnCollection(collection.id, ci)}
        >
          <span class="min-w-0 flex-1 truncate">▾ {collection.name}</span>
          <button
            type="button"
            aria-label="Move {collection.name} up"
            disabled={ci === 0}
            class="px-1 text-muted hover:bg-hover disabled:opacity-30"
            onclick={() => void mr.moveCollection(collection.id, ci - 1)}>↑</button
          >
          <button
            type="button"
            aria-label="Move {collection.name} down"
            disabled={ci === mr.collections.length - 1}
            class="px-1 text-muted hover:bg-hover disabled:opacity-30"
            onclick={() => void mr.moveCollection(collection.id, ci + 1)}>↓</button
          >
          <button
            type="button"
            aria-label="Rename {collection.name}"
            class="px-1 text-muted hover:bg-hover"
            onclick={() => {
              const name = window.prompt('Collection name', collection.name);
              if (name?.trim()) void mr.renameCollection(collection.id, name);
            }}>✎</button
          >
          <button
            type="button"
            aria-label="Delete {collection.name}"
            class="px-1 text-muted hover:text-danger"
            onclick={() => {
              if (window.confirm(`Delete "${collection.name}" and its rules?`))
                void mr.removeCollection(collection.id);
            }}>×</button
          >
        </div>
        <ul aria-label="{collection.name} rules">
          {#each collection.rules as rule, ri (rule.id)}
            <li
              draggable="true"
              class="flex items-center gap-1 px-2 pl-5 {mr.draft?.id === rule.id
                ? 'bg-accent-soft text-accent'
                : 'hover:bg-hover'}"
              ondragstart={() => (drag = { kind: 'rule', id: rule.id })}
              ondragend={() => (drag = null)}
              ondragover={over}
              ondrop={() => dropOnRule(collection.id, ri)}
            >
              <input
                type="checkbox"
                aria-label="Enable {rule.name}"
                checked={rule.enabled}
                onchange={(event) => void mr.setEnabled(rule.id, event.currentTarget.checked)}
              />
              <ContextMenu
                ids={['mr.rule.new', 'mr.rule.delete']}
                context={{ page: 'match-replace', mrRule: rule.id } as CommandContext}
              >
                <button
                  type="button"
                  aria-current={mr.draft?.id === rule.id ? 'true' : undefined}
                  class="min-w-0 flex-1 truncate py-1 text-left text-xs {rule.enabled
                    ? ''
                    : 'text-muted line-through'}"
                  onclick={() => open(rule.id)}
                >
                  {rule.name}
                </button>
              </ContextMenu>
              <button
                type="button"
                aria-label="Move {rule.name} up"
                disabled={ri === 0}
                class="px-1 text-xs text-muted hover:bg-hover disabled:opacity-30"
                onclick={() => void mr.moveRule(rule.id, collection.id, ri - 1)}>↑</button
              >
              <button
                type="button"
                aria-label="Move {rule.name} down"
                disabled={ri === collection.rules.length - 1}
                class="px-1 text-xs text-muted hover:bg-hover disabled:opacity-30"
                onclick={() => void mr.moveRule(rule.id, collection.id, ri + 1)}>↓</button
              >
            </li>
          {:else}
            <li class="py-1 pl-5 text-xs text-muted">No rules.</li>
          {/each}
        </ul>
      </li>
    {:else}
      <li class="px-2 py-1 text-xs text-muted">No collections yet.</li>
    {/each}
  </ul>

  {#if mr.notice}
    <p class="border-t border-line px-2 py-1 text-xs text-muted" aria-live="polite">{mr.notice}</p>
  {/if}
  {#if mr.message}
    <p role="alert" class="border-t border-line px-2 py-1 text-xs text-danger">{mr.message}</p>
  {/if}
</section>
