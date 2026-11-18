<script lang="ts">
  import { tick } from 'svelte';
  import { Tabs } from 'bits-ui';
  import CopyIcon from '@lucide/svelte/icons/copy';
  import SearchIcon from '@lucide/svelte/icons/search';
  import XIcon from '@lucide/svelte/icons/x';
  import { findMatches, hexDump, highlightSegments, prettyMessage } from './format';
  import HighlightedText from './HighlightedText.svelte';
  import MessageTable from './MessageTable.svelte';
  import { parseMessage } from './parse';
  import ShowMoreBar from './ShowMoreBar.svelte';
  import { MODE_LABELS, TEXT_MODES, type ViewerMode } from './types';

  interface Props {
    /** Full raw HTTP message: start line, headers, blank line, body. */
    raw: string;
    title?: string;
    /** Defaults to Raw/Pretty/Hex/Table, plus Preview when `previewUrl` is set. */
    modes?: ViewerMode[];
    /** Sandboxed preview source served by the proxy (`/preview/{id}`). */
    previewUrl?: string;
  }

  let { raw, title = 'Message', modes, previewUrl }: Props = $props();

  const CHUNK = 128 * 1024;
  const HEX_CHUNK = 8 * 1024;

  let mode = $state<ViewerMode>('raw');
  let wrap = $state(false);
  let limit = $state(CHUNK);
  let hexLimit = $state(HEX_CHUNK);
  let searchOpen = $state(false);
  let search = $state('');
  let current = $state(0);
  let searchInput = $state<HTMLInputElement | null>(null);
  let body = $state<HTMLDivElement | null>(null);

  const available = $derived<ViewerMode[]>(
    modes ?? (previewUrl ? [...TEXT_MODES, 'preview'] : [...TEXT_MODES]),
  );
  const parsed = $derived(parseMessage(raw));
  const prettyText = $derived(mode === 'pretty' ? prettyMessage(raw) : '');
  const hex = $derived(
    mode === 'hex' ? hexDump(parsed.body, hexLimit) : { text: '', truncated: false },
  );
  const matches = $derived(
    search.trim() ? findMatches(raw, search.trim()) : { total: 0, indices: [] as number[] },
  );
  const parts = $derived(
    mode === 'raw' && search.trim()
      ? highlightSegments(raw.slice(0, limit), matches.indices, current, search.trim().length)
      : [{ text: raw.slice(0, limit), match: false, current: false }],
  );

  function openSearch() {
    searchOpen = true;
    void tick().then(() => {
      searchInput?.focus();
      searchInput?.select();
    });
  }

  function closeSearch() {
    searchOpen = false;
    search = '';
    current = 0;
  }

  async function jump(delta: number) {
    const total = matches.indices.length;
    if (total === 0) return;
    current = (current + delta + total) % total;
    if (mode !== 'raw') mode = 'raw';
    limit = Math.max(limit, matches.indices[current]! + CHUNK);
    await tick();
    body?.querySelector('[data-current]')?.scrollIntoView?.({ block: 'center' });
  }

  function copy() {
    void navigator.clipboard?.writeText(raw);
  }

  function onkeydown(e: KeyboardEvent) {
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'f') {
      e.preventDefault();
      openSearch();
    }
  }
</script>

<!-- T-012: the region only owns the Ctrl+F shortcut, so the a11y rule does not apply. -->
<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<div
  class="flex h-full min-h-0 flex-col"
  role="region"
  aria-label={title}
  tabindex="-1"
  {onkeydown}
>
  <Tabs.Root bind:value={mode} class="flex h-full min-h-0 flex-col">
    <div
      class="flex min-h-8 shrink-0 flex-wrap items-center gap-1 border-b border-line bg-elevated px-1"
    >
      <Tabs.List class="flex items-center gap-0.5" aria-label="View mode">
        {#each available as m (m)}
          <Tabs.Trigger
            value={m}
            class="rounded-token px-2 py-1 text-xs hover:bg-hover hover:text-fg {mode === m
              ? 'bg-accent-soft text-accent'
              : 'text-muted'}"
          >
            {MODE_LABELS[m]}
          </Tabs.Trigger>
        {/each}
      </Tabs.List>

      <span class="flex-1"></span>

      {#if searchOpen}
        <div class="flex items-center gap-1">
          <input
            bind:this={searchInput}
            type="search"
            aria-label="Search message"
            placeholder="Find in message"
            class="w-44 rounded-token border border-line bg-subtle px-2 py-0.5 text-xs"
            bind:value={search}
            oninput={() => (current = 0)}
            onkeydown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault();
                void jump(e.shiftKey ? -1 : 1);
              } else if (e.key === 'Escape') {
                closeSearch();
              }
            }}
          />
          <span class="min-w-16 text-xs text-muted" aria-live="polite">
            {#if search.trim()}{matches.total
                ? `${current + 1}/${matches.total}`
                : 'No matches'}{/if}
          </span>
          <button
            type="button"
            aria-label="Previous match"
            class="rounded-token px-1 text-xs text-muted hover:bg-hover"
            disabled={matches.total === 0}
            onclick={() => void jump(-1)}>↑</button
          >
          <button
            type="button"
            aria-label="Next match"
            class="rounded-token px-1 text-xs text-muted hover:bg-hover"
            disabled={matches.total === 0}
            onclick={() => void jump(1)}>↓</button
          >
          <button
            type="button"
            aria-label="Close search"
            class="rounded-token p-1 text-muted hover:bg-hover hover:text-fg"
            onclick={closeSearch}
          >
            <XIcon size={13} aria-hidden="true" />
          </button>
        </div>
      {:else}
        <button
          type="button"
          aria-label="Search message (Ctrl+F)"
          title="Search message (Ctrl+F)"
          class="rounded-token p-1 text-muted hover:bg-hover hover:text-fg"
          onclick={openSearch}
        >
          <SearchIcon size={13} aria-hidden="true" />
        </button>
      {/if}

      <button
        type="button"
        aria-pressed={wrap}
        class="rounded-token px-1.5 py-0.5 text-xs {wrap
          ? 'text-accent'
          : 'text-muted'} hover:bg-hover"
        onclick={() => (wrap = !wrap)}>Wrap</button
      >
      <button
        type="button"
        aria-label="Copy message"
        title="Copy message"
        class="rounded-token p-1 text-muted hover:bg-hover hover:text-fg"
        onclick={copy}
      >
        <CopyIcon size={13} aria-hidden="true" />
      </button>
    </div>

    <div bind:this={body} class="min-h-0 flex-1 overflow-auto">
      <Tabs.Content value="raw" class="h-full">
        {#if raw.length > limit}
          <ShowMoreBar shown={limit} total={raw.length} onmore={() => (limit += CHUNK)} />
        {/if}
        <HighlightedText {parts} {wrap} />
      </Tabs.Content>

      <Tabs.Content value="pretty" class="h-full">
        {#if prettyText.length > limit}
          <ShowMoreBar shown={limit} total={prettyText.length} onmore={() => (limit += CHUNK)} />
        {/if}
        <pre
          class="p-2 font-mono text-xs {wrap
            ? 'break-all whitespace-pre-wrap'
            : 'whitespace-pre'}">{prettyText.slice(0, limit)}</pre>
      </Tabs.Content>

      <Tabs.Content value="hex" class="h-full">
        {#if hex.truncated}
          <ShowMoreBar
            shown={hexLimit}
            total={parsed.body.length}
            onmore={() => (hexLimit += HEX_CHUNK)}
          />
        {/if}
        <pre class="p-2 font-mono text-xs whitespace-pre">{hex.text || 'Empty body'}</pre>
      </Tabs.Content>

      <Tabs.Content value="table" class="h-full">
        <MessageTable {parsed} />
      </Tabs.Content>

      {#if available.includes('preview')}
        <Tabs.Content value="preview" class="h-full">
          {#if previewUrl}
            <iframe
              src={previewUrl}
              title="{title} preview"
              sandbox=""
              class="h-full w-full border-0 bg-white"
            ></iframe>
          {:else}
            <p class="p-2 text-muted">No preview URL for this message.</p>
          {/if}
        </Tabs.Content>
      {/if}
    </div>
  </Tabs.Root>
</div>
