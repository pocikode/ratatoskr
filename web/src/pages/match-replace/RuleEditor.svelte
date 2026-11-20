<script lang="ts">
  // Rule editor (docs/02 §3.6): section → valid operations → allowed matchers; ADD hides the matcher.
  import QueryBar from '$lib/components/QueryBar/QueryBar.svelte';
  import type { MatchReplaceState } from './matchreplace.svelte';
  import {
    MATCHER_LABELS,
    OPERATION_LABELS,
    SECTIONS,
    SOURCES,
    matchersFor,
    operationsFor,
  } from './rules';

  interface Props {
    mr: MatchReplaceState;
    onsave: () => void;
    ondelete: () => void;
  }

  let { mr, onsave, ondelete }: Props = $props();

  const draft = $derived(mr.draft);
  const matchers = $derived(draft ? matchersFor(draft.input.operation) : []);
  const field = 'rounded-token border border-line bg-elevated px-2 py-1 text-sm';
</script>

{#if !draft}
  <p class="p-3 text-xs text-muted">Select a rule, or create one.</p>
{:else}
  <form
    aria-label="Rule editor"
    class="flex h-full min-w-0 flex-col gap-3 overflow-auto p-3"
    onsubmit={(event) => {
      event.preventDefault();
      onsave();
    }}
  >
    <div class="flex flex-col gap-1">
      <label class="flex flex-col gap-1">
        <span class="text-xs text-muted">Name</span>
        <input
          class={field}
          aria-invalid={mr.errors.name ? 'true' : undefined}
          bind:value={draft.input.name}
        />
      </label>
      {@render fieldError(mr.errors.name)}
    </div>

    <div class="flex flex-col gap-1">
      <span class="text-xs text-muted">Condition</span>
      <QueryBar
        label="Condition"
        placeholder="HTTPQL, empty means always"
        error={mr.conditionError}
        bind:value={draft.input.condition}
      />
    </div>

    <fieldset class="flex flex-col gap-1">
      <legend class="text-xs text-muted">Sources</legend>
      <div class="flex flex-wrap gap-3">
        {#each SOURCES as source (source.value)}
          <label class="flex items-center gap-1 text-xs">
            <input
              type="checkbox"
              checked={draft.input.sources.includes(source.value)}
              onchange={() => mr.toggleSource(source.value)}
            />
            {source.label}
          </label>
        {/each}
      </div>
      {@render fieldError(mr.errors.sources)}
    </fieldset>

    <div class="flex flex-wrap gap-3">
      <label class="flex flex-col gap-1">
        <span class="text-xs text-muted">Section</span>
        <select
          class={field}
          value={draft.input.section}
          onchange={(event) =>
            mr.setSection(event.currentTarget.value as typeof draft.input.section)}
        >
          {#each SECTIONS as section (section.value)}
            <option value={section.value}>{section.label}</option>
          {/each}
        </select>
      </label>
      <label class="flex flex-col gap-1">
        <span class="text-xs text-muted">Operation</span>
        <select
          class={field}
          value={draft.input.operation}
          onchange={(event) =>
            mr.setOperation(event.currentTarget.value as typeof draft.input.operation)}
        >
          {#each operationsFor(draft.input.section) as op (op)}
            <option value={op}>{OPERATION_LABELS[op]}</option>
          {/each}
        </select>
      </label>
    </div>
    {@render fieldError(mr.errors.section)}

    {#if matchers.length > 0}
      <fieldset class="flex flex-col gap-1">
        <legend class="text-xs text-muted">Matcher</legend>
        <div class="flex flex-wrap items-center gap-3">
          {#each matchers as kind (kind)}
            <label class="flex items-center gap-1 text-xs">
              <input
                type="radio"
                name="matcher-kind"
                checked={draft.input.matcher.kind === kind}
                onchange={() => (draft.input.matcher = { kind, value: '' })}
              />
              {MATCHER_LABELS[kind]}
            </label>
          {/each}
        </div>
        {#if draft.input.matcher.kind !== 'FULL'}
          <input
            class="{field} font-mono"
            aria-label="Matcher value"
            aria-invalid={mr.errors.matcher ? 'true' : undefined}
            bind:value={draft.input.matcher.value}
          />
        {/if}
        {#if draft.input.matcher.kind === 'REGEX'}
          <p class="text-xs text-muted">
            RE2 syntax: no look-around, no backreferences. In the term, $1 and ${'{name}'} insert capture
            groups.
          </p>
        {/if}
        {@render fieldError(mr.errors.matcher)}
      </fieldset>
    {/if}

    <div class="flex flex-col gap-1">
      <label class="flex flex-col gap-1">
        <span class="text-xs text-muted">Term</span>
        <textarea
          class="{field} min-h-16 font-mono"
          aria-invalid={mr.errors.term ? 'true' : undefined}
          bind:value={draft.input.term}></textarea>
      </label>
      {@render fieldError(mr.errors.term)}
    </div>

    {@render fieldError(mr.errors.form)}

    <div class="mt-auto flex gap-2">
      <button
        type="submit"
        disabled={mr.busy}
        class="rounded-token border border-accent bg-accent-soft px-3 py-1 text-xs text-accent disabled:opacity-50"
      >
        Save
      </button>
      <button
        type="button"
        class="rounded-token border border-line px-3 py-1 text-xs hover:bg-hover"
        onclick={() => mr.cancel()}
      >
        Cancel
      </button>
      <button
        type="button"
        disabled={!draft.id}
        class="rounded-token border border-line px-3 py-1 text-xs text-danger hover:bg-hover disabled:opacity-30"
        onclick={ondelete}
      >
        Delete
      </button>
    </div>
  </form>
{/if}

{#snippet fieldError(message?: string)}
  {#if message}
    <p role="alert" class="text-xs text-danger">{message}</p>
  {/if}
{/snippet}
